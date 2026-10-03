package sentry

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	platformjobs "glorynavy.local/seat/internal/platform/jobs"
)

type clientUsageReconcileArgs struct{}

func (clientUsageReconcileArgs) Kind() string { return "sentry.client-usage-reconcile.v1" }

type clientUsageWorker struct {
	river.WorkerDefaults[clientUsageReconcileArgs]
	runtime *Service
}

func clientUsageExtension(s *Service, enabled bool) platformjobs.Extension {
	return platformjobs.Extension{
		Register: func(workers *river.Workers) []*river.PeriodicJob {
			river.AddWorker(workers, &clientUsageWorker{runtime: s})
			if !enabled {
				return nil
			}
			return []*river.PeriodicJob{river.NewPeriodicJob(
				river.PeriodicInterval(time.Minute),
				func() (river.JobArgs, *river.InsertOpts) {
					return clientUsageReconcileArgs{}, &river.InsertOpts{Queue: "exchange", UniqueOpts: platformjobs.ActiveUnique(), MaxAttempts: 5}
				},
				&river.PeriodicJobOpts{RunOnStart: true},
			)}
		},
		Bind: func(_ *river.Client[pgx.Tx]) {},
	}
}

func (w *clientUsageWorker) Work(ctx context.Context, _ *river.Job[clientUsageReconcileArgs]) error {
	s := w.runtime
	if s == nil || s.ClientUsageRemote == nil || s.AlertFunding == nil || s.AlertSettlement == nil || s.Pool == nil {
		return river.JobSnooze(time.Hour)
	}
	enabled, err := s.alertChargingEnabled(ctx)
	if err != nil {
		return err
	}
	if !enabled {
		return river.JobSnooze(time.Minute)
	}
	cursor, err := s.clientUsageCursor(ctx)
	if err != nil {
		return err
	}
	page, err := s.ClientUsageRemote.ListClientUsage(ctx, cursor, 100)
	if err != nil {
		return err
	}
	for _, usage := range page.Usage {
		if err := s.settleClientUsage(ctx, usage); err != nil {
			return err
		}
	}
	if len(page.Usage) > 0 {
		next := strings.TrimSpace(page.NextCursor)
		if next == "" {
			last := page.Usage[len(page.Usage)-1]
			next = last.CreatedAt + "|" + last.UsageID
		}
		if err := s.saveClientUsageCursor(ctx, next); err != nil {
			return err
		}
	}
	if page.HasMore {
		return river.JobSnooze(time.Second)
	}
	return river.JobSnooze(time.Minute)
}

func (s *Service) clientUsageCursor(ctx context.Context) (string, error) {
	var cursor string
	err := s.Pool.QueryRow(ctx, `SELECT cursor FROM sentry_client_usage_reconcile_state WHERE id=1`).Scan(&cursor)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return cursor, err
}

func (s *Service) saveClientUsageCursor(ctx context.Context, cursor string) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO sentry_client_usage_reconcile_state(id,cursor,updated_at) VALUES(1,$1,now()) ON CONFLICT(id) DO UPDATE SET cursor=EXCLUDED.cursor,updated_at=now()`, cursor)
	return err
}

func (s *Service) settleClientUsage(ctx context.Context, usage ClientUsage) error {
	if strings.TrimSpace(usage.UsageID) == "" || strings.TrimSpace(usage.AccountID) == "" || strings.TrimSpace(usage.KeyID) == "" || usage.DurationSeconds <= 0 {
		return ErrInvalid
	}
	started, err := time.Parse(time.RFC3339Nano, usage.StartedAt)
	if err != nil {
		return ErrInvalid
	}
	ended, err := time.Parse(time.RFC3339Nano, usage.EndedAt)
	if err != nil || !ended.After(started) || ended.Sub(started)%time.Second != 0 || int64(ended.Sub(started)/time.Second) != usage.DurationSeconds {
		return ErrInvalid
	}
	policy, err := s.currentAlertPolicy(ctx)
	if err != nil {
		return err
	}
	if !policyValid(policy) || usage.DurationSeconds > policy.MaxGrantSeconds {
		return ErrInvalid
	}
	grantID := stableUUID("sentry-client-grant:" + usage.UsageID)
	reserveKey := stableUUID("sentry-client-reserve:" + usage.UsageID)
	expiresAt := time.Now().UTC().Add(policy.GrantTTL)
	if candidate := ended.UTC().Add(policy.GrantTTL); candidate.After(expiresAt) {
		expiresAt = candidate
	}
	if err := s.AlertFunding.ReserveAlertTime(ctx, grantID, usage.AccountID, reserveKey, policy.PriceVersion, policy.UnitSeconds, policy.UnitPriceMinor, usage.DurationSeconds, expiresAt); err != nil {
		return err
	}
	intervalKey := stableUUID("sentry-client-interval:" + usage.UsageID)
	if err := s.AlertSettlement.ReserveAlertInterval(ctx, grantID, usage.UsageID, intervalKey, started, ended); err != nil {
		return err
	}
	return s.AlertSettlement.SettleAlertInterval(ctx, grantID, usage.UsageID, stableUUID("sentry-client-settle:"+usage.UsageID))
}

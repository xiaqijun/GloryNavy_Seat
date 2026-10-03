package sentry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	platformjobs "glorynavy.local/seat/internal/platform/jobs"
)

type monitorRewardArgs struct{}

func (monitorRewardArgs) Kind() string { return "sentry.monitor-reconcile.v1" }

type monitorRewardWorker struct {
	river.WorkerDefaults[monitorRewardArgs]
	runtime *Service
}

var errMonitorRewardUnpriced = errors.New("monitor reward price is not configured")

func monitorRewardExtension(s *Service, enabled bool) platformjobs.Extension {
	return platformjobs.Extension{
		Register: func(workers *river.Workers) []*river.PeriodicJob {
			river.AddWorker(workers, &monitorRewardWorker{runtime: s})
			if !enabled {
				return nil
			}
			return []*river.PeriodicJob{river.NewPeriodicJob(river.PeriodicInterval(time.Minute), func() (river.JobArgs, *river.InsertOpts) {
				return monitorRewardArgs{}, &river.InsertOpts{Queue: "exchange", UniqueOpts: platformjobs.ActiveUnique(), MaxAttempts: 5}
			}, &river.PeriodicJobOpts{RunOnStart: true})}
		},
		Bind: func(_ *river.Client[pgx.Tx]) {},
	}
}

func (w *monitorRewardWorker) Work(ctx context.Context, _ *river.Job[monitorRewardArgs]) error {
	s := w.runtime
	if s == nil || s.MonitorRemote == nil || s.MonitorRewardFunding == nil || s.Pool == nil {
		return river.JobSnooze(time.Hour)
	}
	cursor, err := s.monitorCursor(ctx)
	if err != nil {
		return err
	}
	page, err := s.MonitorRemote.ListMonitorContributions(ctx, cursor, 100)
	if err != nil {
		return err
	}
	for _, contribution := range page.Contributions {
		if err := s.settleMonitorContribution(ctx, contribution); errors.Is(err, errMonitorRewardUnpriced) {
			return river.JobSnooze(time.Hour)
		} else if err != nil {
			return err
		}
	}
	if len(page.Contributions) > 0 {
		next := strings.TrimSpace(page.NextCursor)
		if next == "" {
			next = page.Contributions[len(page.Contributions)-1].CreatedAt + "|" + page.Contributions[len(page.Contributions)-1].ContributionID
		}
		if err := s.saveMonitorCursor(ctx, next); err != nil {
			return err
		}
	}
	if page.HasMore {
		return river.JobSnooze(time.Second)
	}
	return river.JobSnooze(time.Minute)
}

func (s *Service) monitorCursor(ctx context.Context) (string, error) {
	var c string
	err := s.Pool.QueryRow(ctx, `SELECT cursor FROM sentry_monitor_reconcile_state WHERE id=1`).Scan(&c)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return c, err
}
func (s *Service) saveMonitorCursor(ctx context.Context, c string) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO sentry_monitor_reconcile_state(id,cursor,updated_at) VALUES(1,$1,now()) ON CONFLICT(id) DO UPDATE SET cursor=EXCLUDED.cursor,updated_at=EXCLUDED.updated_at`, c)
	return err
}

type monitorPolicyRow struct {
	Reward    int64
	Effective time.Time
	Version   int64
}

func (s *Service) settleMonitorContribution(ctx context.Context, c MonitorContribution) error {
	if strings.TrimSpace(c.ContributionID) == "" || c.DurationSeconds <= 0 || c.DurationSeconds > 86400 {
		return ErrInvalid
	}
	start, err := time.Parse(time.RFC3339Nano, c.StartedAt)
	if err != nil {
		return ErrInvalid
	}
	end, err := time.Parse(time.RFC3339Nano, c.EndedAt)
	if err != nil || !end.After(start) || end.Sub(start)%time.Second != 0 || int64(end.Sub(start)/time.Second) != c.DurationSeconds {
		return ErrInvalid
	}
	evidence, _ := json.Marshal(c.Evidence)
	if len(evidence) == 0 {
		evidence = []byte(`{}`)
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var exists bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sentry_monitor_rewards WHERE contribution_id=$1)`, c.ContributionID).Scan(&exists)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	account := strings.TrimSpace(c.AccountID)
	keyID := strings.TrimSpace(c.KeyID)
	var accountID string
	if _, err := uuidText(account); err == nil && keyID != "" {
		err = tx.QueryRow(ctx, `SELECT account_id::text FROM sentry_keys WHERE id=$1 AND account_id=$2 AND 'monitor'=ANY(permissions)`, keyID, account).Scan(&accountID)
	}
	if accountID == "" || strings.TrimSpace(c.Eligibility) != "eligible" {
		_, err = tx.Exec(ctx, `INSERT INTO sentry_monitor_rewards(contribution_id,account_id,remote_key_id,client_id,system_name,started_at,ended_at,duration_seconds,state,numerator,coins_minor,fingerprint,evidence,price_snapshot) VALUES($1,NULL,$2,$3,$4,$5,$6,$7,'excluded',0,0,$8,$9,'{}')`, c.ContributionID, keyID, c.ClientID, c.SystemName, start, end, c.DurationSeconds, monitorFingerprint(c), evidence)
		if err == nil {
			err = tx.Commit(ctx)
		}
		return err
	}
	policies, err := s.monitorPolicies(ctx, tx, end)
	if err != nil {
		return err
	}
	if len(policies) == 0 { // No configured reward price: retain evidence but do not invent a zero-priced payout.
		return errMonitorRewardUnpriced
	}
	var remainder int64
	err = tx.QueryRow(ctx, `SELECT numerator FROM sentry_monitor_remainders WHERE account_id=$1 FOR UPDATE`, accountID).Scan(&remainder)
	if errors.Is(err, pgx.ErrNoRows) {
		if _, err = tx.Exec(ctx, `INSERT INTO sentry_monitor_remainders(account_id,numerator) VALUES($1,0) ON CONFLICT DO NOTHING`, accountID); err != nil {
			return err
		}
		remainder = 0
	} else if err != nil {
		return err
	}
	total := remainder
	priceSnapshot := make([]map[string]any, 0, len(policies))
	credited := int64(0)
	for i, p := range policies {
		segStart := start
		if p.Effective.After(segStart) {
			segStart = p.Effective
		}
		segEnd := end
		if i+1 < len(policies) && policies[i+1].Effective.Before(segEnd) {
			segEnd = policies[i+1].Effective
		}
		seconds := int64(segEnd.Sub(segStart) / time.Second)
		if seconds <= 0 || p.Reward == 0 {
			continue
		}
		if seconds > math.MaxInt64/p.Reward {
			return ErrInvalid
		}
		total += seconds * p.Reward
		if total < 0 {
			return ErrInvalid
		}
		credited += total / 3600
		total %= 3600
		priceSnapshot = append(priceSnapshot, map[string]any{"version": p.Version, "effective_at": p.Effective.UTC().Format(time.RFC3339Nano), "hourly_reward_minor": p.Reward, "seconds": seconds})
	}
	if _, err = tx.Exec(ctx, `UPDATE sentry_monitor_remainders SET numerator=$2 WHERE account_id=$1`, accountID, total); err != nil {
		return err
	}
	requestKey := stableUUID(c.ContributionID)
	if credited > 0 {
		if err = s.MonitorRewardFunding.CreditMonitorRewardTx(ctx, tx, accountID, "sentry-monitor:"+c.ContributionID, requestKey, credited); err != nil {
			return err
		}
	}
	snapshot, _ := json.Marshal(priceSnapshot)
	if _, err = tx.Exec(ctx, `INSERT INTO sentry_monitor_rewards(contribution_id,account_id,remote_key_id,client_id,system_name,started_at,ended_at,duration_seconds,state,numerator,coins_minor,fingerprint,evidence,price_snapshot) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'rewarded',$9,$10,$11,$12,$13)`, c.ContributionID, accountID, keyID, c.ClientID, c.SystemName, start, end, c.DurationSeconds, total, credited, monitorFingerprint(c), evidence, snapshot); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) monitorPolicies(ctx context.Context, tx pgx.Tx, end time.Time) ([]monitorPolicyRow, error) {
	rows, err := tx.Query(ctx, `SELECT monitor_hourly_reward_minor,effective_at,version FROM sentry_time_pricing WHERE effective_at < $1 ORDER BY effective_at,version`, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []monitorPolicyRow{}
	for rows.Next() {
		var p monitorPolicyRow
		if err := rows.Scan(&p.Reward, &p.Effective, &p.Version); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
func monitorFingerprint(c MonitorContribution) string {
	return fmt.Sprintf("%s|%s|%s|%d|%s|%s", c.ContributionID, c.ClientID, c.SystemName, c.PrimaryGeneration, c.StartedAt, c.EndedAt)
}
func stableUUID(raw string) string {
	h := sha256.Sum256([]byte(raw))
	h[6] = (h[6] & 0x0f) | 0x40
	h[8] = (h[8] & 0x3f) | 0x80
	return fmt.Sprintf("%s-%s-%s-%s-%s", hex.EncodeToString(h[0:4]), hex.EncodeToString(h[4:6]), hex.EncodeToString(h[6:8]), hex.EncodeToString(h[8:10]), hex.EncodeToString(h[10:16]))
}
func uuidText(raw string) (string, error) {
	if len(raw) != 36 || raw[8] != '-' || raw[13] != '-' || raw[18] != '-' || raw[23] != '-' {
		return "", ErrInvalid
	}
	return raw, nil
}

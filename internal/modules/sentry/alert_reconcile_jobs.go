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

type alertReconcileArgs struct{}

func (alertReconcileArgs) Kind() string { return "sentry.alert-reconcile.v1" }

type alertReconcileWorker struct {
	river.WorkerDefaults[alertReconcileArgs]
	runtime *Service
}

func alertReconcileExtension(s *Service, enabled bool) platformjobs.Extension {
	return platformjobs.Extension{
		Register: func(workers *river.Workers) []*river.PeriodicJob {
			river.AddWorker(workers, &alertReconcileWorker{runtime: s})
			if !enabled {
				return nil
			}
			return []*river.PeriodicJob{river.NewPeriodicJob(
				river.PeriodicInterval(time.Minute),
				func() (river.JobArgs, *river.InsertOpts) {
					return alertReconcileArgs{}, &river.InsertOpts{Queue: "exchange", UniqueOpts: platformjobs.ActiveUnique(), MaxAttempts: 5}
				},
				&river.PeriodicJobOpts{RunOnStart: true},
			)}
		},
		Bind: func(queue *river.Client[pgx.Tx]) { s.alertQueue = queue },
	}
}

func (w *alertReconcileWorker) Work(ctx context.Context, _ *river.Job[alertReconcileArgs]) error {
	s := w.runtime
	if s == nil || !s.AlertEnabled || s.AlertRemote == nil || s.AlertSettlement == nil || s.Pool == nil {
		return river.JobSnooze(time.Hour)
	}
	cursor, err := s.alertCursor(ctx)
	if err != nil {
		return err
	}
	page, err := s.AlertRemote.ListAlertDeliveries(ctx, cursor, 100)
	if err != nil {
		return err
	}
	for _, delivery := range page.Deliveries {
		if err := ReconcileAlertDelivery(ctx, s.AlertSettlement, delivery); err != nil {
			return err
		}
	}
	if len(page.Deliveries) > 0 {
		// Advance only after the whole page is applied. A failure midway leaves
		// the page replayable; exchange request keys make that replay safe.
		cursor := strings.TrimSpace(page.NextCursor)
		if cursor == "" {
			// Keep compatibility with an older Sentry endpoint that omitted the
			// explicit cursor while still returning a non-empty page.
			cursor = pageCursor(page.Deliveries[len(page.Deliveries)-1])
		}
		if err := s.saveAlertCursor(ctx, cursor); err != nil {
			return err
		}
	}
	if page.HasMore {
		return river.JobSnooze(time.Second)
	}
	return river.JobSnooze(time.Minute)
}

func pageCursor(delivery AlertDelivery) string {
	return delivery.CreatedAt + "|" + delivery.DeliveryID
}

func (s *Service) alertCursor(ctx context.Context) (string, error) {
	var cursor string
	err := s.Pool.QueryRow(ctx, `SELECT cursor FROM sentry_alert_reconcile_state WHERE id=1`).Scan(&cursor)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return cursor, err
}

func (s *Service) saveAlertCursor(ctx context.Context, cursor string) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO sentry_alert_reconcile_state(id,cursor,updated_at) VALUES(1,$1,now()) ON CONFLICT (id) DO UPDATE SET cursor=EXCLUDED.cursor,updated_at=EXCLUDED.updated_at`, cursor)
	return err
}

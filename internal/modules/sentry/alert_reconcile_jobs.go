package sentry

import (
	"context"
	"errors"

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

func alertReconcileExtension(s *Service) platformjobs.Extension {
	return platformjobs.Extension{
		Register: func(workers *river.Workers) []*river.PeriodicJob {
			river.AddWorker(workers, &alertReconcileWorker{runtime: s})
			// Do not schedule new event/delivery reconciliation jobs. The worker
			// remains registered only to cancel jobs queued by older releases.
			return nil
		},
		Bind: func(_ *river.Client[pgx.Tx]) {},
	}
}

func (w *alertReconcileWorker) Work(ctx context.Context, _ *river.Job[alertReconcileArgs]) error {
	return river.JobCancel(errors.New("sentry alert event billing is retired; use client heartbeat usage"))
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

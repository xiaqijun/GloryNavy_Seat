package exchange

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"glorynavy.local/seat/internal/modules/exchange/internal/store"
	"glorynavy.local/seat/internal/platform/jobs"
	"time"
)

type deliveryScanArgs struct{}

func (deliveryScanArgs) Kind() string { return "exchange.delivery-scan.v1" }

type deliveryCheckArgs struct {
	OrderID int64 `json:"order_id"`
}

func (deliveryCheckArgs) Kind() string { return "exchange.delivery-check.v1" }

// Service is assigned by the host before the shared runtime starts.
type DeliveryJobs struct {
	Service *Service
	Enabled bool
	queue   *river.Client[pgx.Tx]
}
type deliveryScanWorker struct {
	river.WorkerDefaults[deliveryScanArgs]
	runtime *DeliveryJobs
}
type deliveryCheckWorker struct {
	river.WorkerDefaults[deliveryCheckArgs]
	runtime *DeliveryJobs
}

func (r *DeliveryJobs) Extension() jobs.Extension {
	return jobs.Extension{Register: func(w *river.Workers) []*river.PeriodicJob {
		river.AddWorker(w, &priceWorker{runtime: r})
		river.AddWorker(w, &deliveryScanWorker{runtime: r})
		river.AddWorker(w, &deliveryCheckWorker{runtime: r})
		if !r.Enabled {
			return nil
		}
		return []*river.PeriodicJob{river.NewPeriodicJob(river.PeriodicInterval(time.Minute), func() (river.JobArgs, *river.InsertOpts) {
			return deliveryScanArgs{}, &river.InsertOpts{Queue: "exchange", UniqueOpts: jobs.ActiveUnique()}
		}, &river.PeriodicJobOpts{RunOnStart: true})}
	}, Bind: func(q *river.Client[pgx.Tx]) { r.queue = q }}
}
func (w *deliveryScanWorker) Work(ctx context.Context, _ *river.Job[deliveryScanArgs]) error {
	r := w.runtime
	if !r.Enabled || r.Service == nil {
		return river.JobSnooze(time.Hour)
	}
	if err := r.scanPrices(ctx); err != nil {
		return err
	}
	ids, e := store.DueDeliveries(ctx, r.Service.Pool)
	if e != nil {
		return e
	}
	for _, id := range ids {
		if _, e = r.queue.Insert(ctx, deliveryCheckArgs{id}, &river.InsertOpts{Queue: "exchange", UniqueOpts: jobs.ActiveUnique(), MaxAttempts: 3}); e != nil {
			return e
		}
	}
	return nil
}
func (w *deliveryCheckWorker) Work(ctx context.Context, j *river.Job[deliveryCheckArgs]) error {
	r := w.runtime
	if !r.Enabled || r.Service == nil {
		return river.JobSnooze(time.Hour)
	}
	// Persist scheduling even on permission/data failure, avoiding head-of-line starvation.
	if e := store.ScheduleDelivery(ctx, r.Service.Pool, j.Args.OrderID, time.Now().Add(5*time.Minute)); e != nil {
		return e
	}
	return r.Service.CheckDelivery(ctx, j.Args.OrderID)
}

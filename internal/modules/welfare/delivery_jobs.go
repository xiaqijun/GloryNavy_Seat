package welfare

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
	"glorynavy.local/seat/internal/platform/jobs"
	"time"
)

type deliveryScanArgs struct{}

func (deliveryScanArgs) Kind() string { return "welfare.delivery-scan.v1" }

type deliveryCheckArgs struct {
	CaseID int64 `json:"case_id"`
}

func (deliveryCheckArgs) Kind() string { return "welfare.delivery-check.v1" }

type valuationCheckArgs struct {
	CaseID int64 `json:"case_id"`
}

func (valuationCheckArgs) Kind() string { return "welfare.valuation-check.v1" }

type settlementBatchArgs struct {
	BatchID int64 `json:"batch_id"`
}

func (settlementBatchArgs) Kind() string { return "welfare.settlement-batch.v1" }

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
type valuationCheckWorker struct {
	river.WorkerDefaults[valuationCheckArgs]
	runtime *DeliveryJobs
}
type settlementBatchWorker struct {
	river.WorkerDefaults[settlementBatchArgs]
	runtime *DeliveryJobs
}

func (r *DeliveryJobs) Extension() jobs.Extension {
	return jobs.Extension{Register: func(w *river.Workers) []*river.PeriodicJob {
		river.AddWorker(w, &deliveryScanWorker{runtime: r})
		river.AddWorker(w, &deliveryCheckWorker{runtime: r})
		river.AddWorker(w, &valuationCheckWorker{runtime: r})
		river.AddWorker(w, &settlementBatchWorker{runtime: r})
		if !r.Enabled {
			return nil
		}
		return []*river.PeriodicJob{river.NewPeriodicJob(river.PeriodicInterval(time.Minute), func() (river.JobArgs, *river.InsertOpts) {
			return deliveryScanArgs{}, &river.InsertOpts{Queue: "welfare", UniqueOpts: jobs.ActiveUnique()}
		}, &river.PeriodicJobOpts{RunOnStart: true})}
	}, Bind: func(q *river.Client[pgx.Tx]) { r.queue = q }}
}

// Insert in the same transaction as the application. The periodic scan also
// recovers cases saved before the queue was available.
func (r *DeliveryJobs) EnqueueValuationTx(ctx context.Context, tx pgx.Tx, id int64) error {
	if r.queue == nil {
		return nil
	}
	_, err := r.queue.InsertTx(ctx, tx, valuationCheckArgs{id}, &river.InsertOpts{Queue: "welfare", UniqueOpts: jobs.ActiveUnique(), MaxAttempts: 3})
	return err
}
func (w *deliveryScanWorker) Work(ctx context.Context, _ *river.Job[deliveryScanArgs]) error {
	r := w.runtime
	if !r.Enabled || r.Service == nil {
		return river.JobSnooze(time.Hour)
	}
	ids, e := store.DueDeliveries(ctx, r.Service.Pool)
	if e != nil {
		return e
	}
	for _, id := range ids {
		if _, e = r.queue.Insert(ctx, deliveryCheckArgs{id}, &river.InsertOpts{Queue: "welfare", UniqueOpts: jobs.ActiveUnique(), MaxAttempts: 3}); e != nil {
			return e
		}
	}
	valuations, e := store.PendingValuations(ctx, r.Service.Pool)
	if e != nil {
		return e
	}
	for _, id := range valuations {
		if _, e = r.queue.Insert(ctx, valuationCheckArgs{id}, &river.InsertOpts{Queue: "welfare", UniqueOpts: jobs.ActiveUnique(), MaxAttempts: 3}); e != nil {
			return e
		}
	}
	batches, e := store.DueSettlementBatches(ctx, r.Service.Pool)
	if e != nil {
		return e
	}
	for _, id := range batches {
		if _, e = r.queue.Insert(ctx, settlementBatchArgs{id}, &river.InsertOpts{Queue: "welfare", UniqueOpts: jobs.ActiveUnique(), MaxAttempts: 3}); e != nil {
			return e
		}
	}
	return nil
}

func (w *valuationCheckWorker) Work(ctx context.Context, j *river.Job[valuationCheckArgs]) error {
	r := w.runtime
	if !r.Enabled || r.Service == nil {
		return river.JobSnooze(time.Hour)
	}
	return r.Service.AutoAppraise(ctx, j.Args.CaseID)
}
func (w *deliveryCheckWorker) Work(ctx context.Context, j *river.Job[deliveryCheckArgs]) error {
	r := w.runtime
	if !r.Enabled || r.Service == nil {
		return river.JobSnooze(time.Hour)
	}
	// Persist scheduling even on permission/data failure, avoiding head-of-line starvation.
	if e := store.ScheduleDelivery(ctx, r.Service.Pool, j.Args.CaseID, time.Now().Add(5*time.Minute)); e != nil {
		return e
	}
	return r.Service.CheckDelivery(ctx, j.Args.CaseID)
}

func (w *settlementBatchWorker) Work(ctx context.Context, j *river.Job[settlementBatchArgs]) error {
	r := w.runtime
	if !r.Enabled || r.Service == nil {
		return river.JobSnooze(time.Hour)
	}
	return r.Service.ProcessSettlementBatch(ctx, j.Args.BatchID)
}

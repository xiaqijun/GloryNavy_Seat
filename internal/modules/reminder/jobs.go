package reminder

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"glorynavy.local/seat/internal/modules/reminder/internal/store"
	"glorynavy.local/seat/internal/platform/jobs"
	"time"
)

type Jobs struct {
	Service *Service
	queue   *river.Client[pgx.Tx]
}
type scanArgs struct{}

func (scanArgs) Kind() string { return "reminder.scan.v1" }

type sendArgs struct {
	Account string `json:"account"`
}

func (sendArgs) Kind() string { return "reminder.send.v1" }

type scanWorker struct {
	river.WorkerDefaults[scanArgs]
	runtime *Jobs
}
type sendWorker struct {
	river.WorkerDefaults[sendArgs]
	runtime *Jobs
}

func (r *Jobs) Extension() jobs.Extension {
	return jobs.Extension{Register: func(w *river.Workers) []*river.PeriodicJob {
		river.AddWorker(w, &scanWorker{runtime: r})
		river.AddWorker(w, &sendWorker{runtime: r})
		if !r.Service.Enabled {
			return nil
		}
		return []*river.PeriodicJob{river.NewPeriodicJob(river.PeriodicInterval(time.Minute), func() (river.JobArgs, *river.InsertOpts) {
			return scanArgs{}, &river.InsertOpts{Queue: "reminder", UniqueOpts: jobs.ActiveUnique(), MaxAttempts: 3}
		}, &river.PeriodicJobOpts{RunOnStart: true})}
	}, Bind: func(q *river.Client[pgx.Tx]) { r.queue = q }}
}
func (w *scanWorker) Work(ctx context.Context, _ *river.Job[scanArgs]) error {
	r := w.runtime
	s := r.Service
	if !s.Enabled || s.Users == nil {
		return river.JobSnooze(time.Hour)
	}
	if err := store.Maintain(ctx, s.Pool); err != nil {
		return err
	}
	c, err := store.ReadSettings(ctx, s.Pool, false)
	if err != nil || !c.Enabled {
		return err
	}
	// Missing consent is a configuration state, not a failing job for every account.
	if _, err = s.sender(ctx, c); errors.Is(err, ErrSender) {
		return nil
	} else if err != nil {
		return err
	}
	users, err := s.Users(ctx)
	if err != nil {
		return err
	}
	for _, account := range users {
		if _, err = r.queue.Insert(ctx, sendArgs{Account: account}, &river.InsertOpts{Queue: "reminder", UniqueOpts: jobs.ActiveUnique(), MaxAttempts: 3}); err != nil {
			return err
		}
	}
	return nil
}
func (w *sendWorker) Work(ctx context.Context, j *river.Job[sendArgs]) error {
	if !w.runtime.Service.Enabled {
		return river.JobSnooze(time.Hour)
	}
	err := w.runtime.Service.deliver(ctx, j.Args.Account)
	if errors.Is(err, ErrSender) {
		return nil
	}
	return err
}

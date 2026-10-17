package community

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"glorynavy.local/seat/internal/modules/community/internal/store"
	"glorynavy.local/seat/internal/platform/jobs"
	"time"
)

type approvalScanArgs struct{}

func (approvalScanArgs) Kind() string { return "community.approval-scan.v1" }

type approvalGroupArgs struct {
	Group string `json:"group"`
}

func (approvalGroupArgs) Kind() string { return "community.approval-group.v1" }

// The host injects a live, authorized, read-only approval feed before Run starts.
type ApprovalJobs struct {
	Service *Service
	Enabled bool
	Origin  string
	Pending func(context.Context, string) ([]string, error)
	queue   *river.Client[pgx.Tx]
}
type approvalScanWorker struct {
	river.WorkerDefaults[approvalScanArgs]
	runtime *ApprovalJobs
}
type approvalGroupWorker struct {
	river.WorkerDefaults[approvalGroupArgs]
	runtime *ApprovalJobs
}

// A group can contain multiple independently authorized reviewers; bound the
// complete scan without applying the shared one-minute HTTP-job default.
func (w *approvalGroupWorker) Timeout(*river.Job[approvalGroupArgs]) time.Duration {
	return 5 * time.Minute
}

func (r *ApprovalJobs) Extension() jobs.Extension {
	return jobs.Extension{Register: func(w *river.Workers) []*river.PeriodicJob {
		river.AddWorker(w, &approvalScanWorker{runtime: r})
		river.AddWorker(w, &approvalGroupWorker{runtime: r})
		if !r.Enabled {
			return nil
		}
		return []*river.PeriodicJob{river.NewPeriodicJob(river.PeriodicInterval(time.Minute), func() (river.JobArgs, *river.InsertOpts) {
			return approvalScanArgs{}, &river.InsertOpts{Queue: "community", UniqueOpts: jobs.ActiveUnique(), MaxAttempts: 3}
		}, &river.PeriodicJobOpts{RunOnStart: true})}
	}, Bind: func(q *river.Client[pgx.Tx]) { r.queue = q }}
}
func (w *approvalScanWorker) Work(ctx context.Context, _ *river.Job[approvalScanArgs]) error {
	r := w.runtime
	if !r.Enabled || r.Service == nil || r.Pending == nil {
		return river.JobSnooze(time.Hour)
	}
	if err := store.ExpireApprovalNotices(ctx, r.Service.pool); err != nil {
		return err
	}
	settings, err := r.Service.ListOneBotSettings(ctx)
	if err != nil {
		return err
	}
	if !settings.Enabled {
		return nil
	}
	for _, g := range settings.Groups {
		if g.NotifyApprovals {
			if _, err = r.queue.Insert(ctx, approvalGroupArgs{Group: g.ID}, &river.InsertOpts{Queue: "community", UniqueOpts: jobs.ActiveUnique(), MaxAttempts: 3}); err != nil {
				return err
			}
		}
	}
	return nil
}
func (w *approvalGroupWorker) Work(ctx context.Context, j *river.Job[approvalGroupArgs]) error {
	r := w.runtime
	if !r.Enabled || r.Service == nil || r.Pending == nil {
		return river.JobSnooze(time.Hour)
	}
	return r.deliver(ctx, j.Args.Group)
}

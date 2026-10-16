package loan

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"glorynavy.local/seat/internal/modules/loan/internal/store"
	"glorynavy.local/seat/internal/platform/jobs"
)

type creditEvaluationArgs struct {
	AccountID string `json:"account_id"`
}

type creditDispatchArgs struct{}

func (creditDispatchArgs) Kind() string { return "loan.credit-dispatch.v1" }

func (creditEvaluationArgs) Kind() string { return "loan.credit-evaluation.v1" }

// CreditJobs runs account-level evaluations from persisted EVE snapshots. It
// never performs an evaluation from an HTTP request.
type CreditJobs struct {
	Service *Service
	Users   func(context.Context) ([]string, error)
	Enabled bool
	queue   *river.Client[pgx.Tx]
}

type creditDispatchWorker struct {
	river.WorkerDefaults[creditDispatchArgs]
	runtime *CreditJobs
}

type creditEvaluationWorker struct {
	river.WorkerDefaults[creditEvaluationArgs]
	runtime *CreditJobs
}

func (r *CreditJobs) Extension() jobs.Extension {
	return jobs.Extension{Register: func(w *river.Workers) []*river.PeriodicJob {
		river.AddWorker(w, &creditEvaluationWorker{runtime: r})
		river.AddWorker(w, &creditDispatchWorker{runtime: r})
		if !r.Enabled {
			return nil
		}
		return []*river.PeriodicJob{river.NewPeriodicJob(river.PeriodicInterval(time.Hour), func() (river.JobArgs, *river.InsertOpts) {
			return creditDispatchArgs{}, &river.InsertOpts{Queue: "loan", UniqueOpts: jobs.ActiveUnique()}
		}, &river.PeriodicJobOpts{RunOnStart: true})}
	}, Bind: func(q *river.Client[pgx.Tx]) { r.queue = q }}
}

func (w *creditDispatchWorker) Work(ctx context.Context, _ *river.Job[creditDispatchArgs]) error {
	r := w.runtime
	if !r.Enabled || r.Service == nil || r.Users == nil {
		return river.JobSnooze(time.Hour)
	}
	users, err := r.Users(ctx)
	if err != nil {
		return err
	}
	policy, err := r.Service.CreditPolicy(ctx)
	if err != nil {
		return err
	}
	ruleVersion := creditPolicyRuleVersion(policy)
	for _, user := range users {
		e, err := store.LatestCreditEvaluation(ctx, r.Service.db(), user)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err == nil && e.PolicyVersion == ruleVersion && evaluationDue(e.EvaluatedAt, time.Now().UTC()) == false {
			continue
		}
		if _, err := r.queue.Insert(ctx, creditEvaluationArgs{AccountID: user}, &river.InsertOpts{Queue: "loan", UniqueOpts: jobs.ActiveUnique(), MaxAttempts: 3}); err != nil {
			return err
		}
	}
	return nil
}

func evaluationDue(evaluatedAt, now time.Time) bool {
	return evaluatedAt.IsZero() || evaluatedAt.After(now) || now.Sub(evaluatedAt) >= 24*time.Hour
}

func (w *creditEvaluationWorker) Timeout(*river.Job[creditEvaluationArgs]) time.Duration {
	return 5 * time.Minute
}

func (w *creditEvaluationWorker) Work(ctx context.Context, j *river.Job[creditEvaluationArgs]) error {
	r := w.runtime
	if !r.Enabled || r.Service == nil {
		return river.JobSnooze(time.Hour)
	}
	e, err := store.LatestCreditEvaluation(ctx, r.Service.db(), j.Args.AccountID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	policy, policyErr := r.Service.CreditPolicy(ctx)
	if policyErr != nil {
		return policyErr
	}
	if err == nil && e.PolicyVersion == creditPolicyRuleVersion(policy) && !evaluationDue(e.EvaluatedAt, time.Now().UTC()) {
		return nil
	}
	_, err = r.Service.EvaluateAndStore(ctx, j.Args.AccountID)
	return err
}

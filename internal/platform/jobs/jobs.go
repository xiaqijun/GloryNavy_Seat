// Package jobs owns the shared River runtime, not business scheduling policy.
package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/riverqueue/river/rivertype"
)

type Extension struct {
	Register func(*river.Workers) []*river.PeriodicJob
	Bind     func(*river.Client[pgx.Tx])
}

func New(pool *pgxpool.Pool, workers *river.Workers, periodic []*river.PeriodicJob, logger *slog.Logger) (*river.Client[pgx.Tx], error) {
	return river.NewClient(riverpgxv5.New(pool), &river.Config{
		Workers: workers, PeriodicJobs: periodic, Logger: logger, JobTimeout: 60 * time.Second,
		// River also checks each worker's timeout before rescue, preserving the
		// SDE worker's 16-minute deadline while recovering interrupted scanners.
		RescueStuckJobsAfter: 3 * time.Minute,
		Queues:               map[string]river.QueueConfig{"eve_control": {MaxWorkers: 1}, "eve_characters": {MaxWorkers: 2}, "eve_contracts": {MaxWorkers: 1}, "eve_sde": {MaxWorkers: 1}, "attendance": {MaxWorkers: 2}, "welfare": {MaxWorkers: 1}, "exchange": {MaxWorkers: 1}},
	})
}

func ActiveUnique() river.UniqueOpts {
	return river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{
		rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRunning, rivertype.JobStateRetryable, rivertype.JobStateScheduled,
	}}
}

// Migrate is called by deployment tooling and isolated tests, never API startup.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	m, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return err
	}
	_, err = m.Migrate(ctx, rivermigrate.DirectionUp, nil)
	return err
}

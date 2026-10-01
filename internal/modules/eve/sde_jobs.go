package eve

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
	platformjobs "glorynavy.local/seat/internal/platform/jobs"
)

type sdeUpdateArgs struct{}

func (sdeUpdateArgs) Kind() string { return "eve.sde-update.v1" }

type sdeUpdateWorker struct {
	river.WorkerDefaults[sdeUpdateArgs]
	s *SyncService
}

func (*sdeUpdateWorker) Timeout(*river.Job[sdeUpdateArgs]) time.Duration { return 16 * time.Minute }
func (w *sdeUpdateWorker) Work(ctx context.Context, _ *river.Job[sdeUpdateArgs]) error {
	if w.s.sde == nil {
		return nil
	}
	return w.s.sde.update(ctx)
}

func (s *StaticDataService) enqueueDue(ctx context.Context, queue *river.Client[pgx.Tx]) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	due, err := store.SDEUpdateDue(ctx, tx)
	if err != nil {
		return err
	}
	if !due {
		return nil
	}
	if _, err = queue.InsertTx(ctx, tx, sdeUpdateArgs{}, &river.InsertOpts{Queue: "eve_sde", UniqueOpts: platformjobs.ActiveUnique(), MaxAttempts: 5}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *StaticDataService) update(parent context.Context) error {
	ctx, cancel := context.WithTimeout(parent, 15*time.Minute)
	defer cancel()
	fence, err := store.ClaimSDEUpdate(ctx, s.pool)
	if errors.Is(err, pgx.ErrNoRows) {
		state, e := s.Status(ctx)
		if e != nil {
			return e
		}
		if state.Pinned {
			return nil
		}
		due := state.NextCheckAt
		if state.LeaseUntil.After(due) {
			due = state.LeaseUntil
		}
		return river.JobSnooze(max(time.Until(due), time.Second))
	}
	if err != nil {
		return err
	}
	var build int64
	finish := func(outcome, reason string) error {
		// Cancellation still releases the claim; fences protect operator changes.
		final, end := context.WithTimeout(context.Background(), 5*time.Second)
		defer end()
		return store.FinishSDEUpdate(final, s.pool, fence, build, outcome, reason, s.interval)
	}
	fail := func(reason string) error {
		if e := finish("failed", reason); e != nil {
			return e
		}
		return fmt.Errorf("SDE %s; next attempt follows persisted backoff", reason)
	}
	build, err = s.source.Latest(ctx)
	if err != nil {
		return fail("metadata_failed")
	}
	state, err := s.Status(ctx)
	if err != nil {
		return fail("database_failed")
	}
	if state.Fence != fence || state.Pinned {
		return nil
	}
	if state.ActiveBuild > build || (state.ActiveBuild == build && state.MapperVersion >= 2) {
		return finish("unchanged", "")
	}
	filename, err := s.source.Download(ctx, build)
	if err != nil {
		return fail("download_failed")
	}
	_, err = store.ImportSDENamesAutomatic(ctx, s.pool, filename, build, fence)
	if errors.Is(err, store.ErrSDEUpdateSuperseded) {
		return nil
	}
	if err != nil {
		return fail("import_failed")
	}
	return finish("updated", "")
}

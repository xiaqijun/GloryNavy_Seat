package attendance

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"glorynavy.local/seat/internal/modules/attendance/internal/store"
	"glorynavy.local/seat/internal/modules/eve"
	jobs "glorynavy.local/seat/internal/platform/jobs"
	"time"
)

type BattleGateway interface {
	BattleCredential(context.Context, int64, []byte, string) (eve.BattleProof, error)
	GuardBattle(context.Context, pgx.Tx, eve.BattleProof) error
	BattleFitting(context.Context, eve.BattleProof, int64, time.Time) (eve.BattleFitting, error)
	BattleLosses(context.Context, eve.BattleProof, int, time.Time, time.Time) (eve.BattleLossPage, error)
}
type battleArgs struct {
	TaskID int64 `json:"task_id"`
}

func (battleArgs) Kind() string { return "attendance.battle.v1" }

type battleDispatchArgs struct{}

func (battleDispatchArgs) Kind() string { return "attendance.dispatch.v1" }

type battleRuntime struct {
	service *Service
	enabled bool
	queue   *river.Client[pgx.Tx]
}
type battleWorker struct {
	river.WorkerDefaults[battleArgs]
	runtime *battleRuntime
}
type battleDispatchWorker struct {
	river.WorkerDefaults[battleDispatchArgs]
	runtime *battleRuntime
}

func (s *Service) Extension(enabled bool) jobs.Extension {
	r := &battleRuntime{service: s, enabled: enabled}
	return jobs.Extension{Register: func(w *river.Workers) []*river.PeriodicJob {
		river.AddWorker(w, &battleWorker{runtime: r})
		river.AddWorker(w, &battleDispatchWorker{runtime: r})
		river.AddWorker(w, &alliancePAPWorker{service: s, enabled: enabled})
		if !enabled {
			return nil
		}
		return []*river.PeriodicJob{river.NewPeriodicJob(river.PeriodicInterval(30*time.Second), func() (river.JobArgs, *river.InsertOpts) {
			return battleDispatchArgs{}, &river.InsertOpts{Queue: "attendance", UniqueOpts: jobs.ActiveUnique()}
		}, &river.PeriodicJobOpts{RunOnStart: true}), alliancePAPPeriodic(s, enabled)}
	}, Bind: func(q *river.Client[pgx.Tx]) { r.queue = q }}
}
func (w *battleDispatchWorker) Work(ctx context.Context, _ *river.Job[battleDispatchArgs]) error {
	if !w.runtime.enabled {
		return river.JobSnooze(time.Hour)
	}
	ids, err := store.New(w.runtime.service.Pool).DueBattleTasks(ctx)
	if err != nil {
		return err
	}
	for _, task := range ids {
		priority := 3
		if task.Kind == "fitting" {
			priority = 1
		}
		if _, err = w.runtime.queue.Insert(ctx, battleArgs{task.ID}, &river.InsertOpts{Queue: "attendance", Priority: priority, UniqueOpts: jobs.ActiveUnique(), MaxAttempts: 3}); err != nil {
			return err
		}
	}
	return nil
}
func (w *battleWorker) Work(ctx context.Context, j *river.Job[battleArgs]) error {
	if !w.runtime.enabled {
		return river.JobSnooze(time.Hour)
	}
	bounded, cancel := context.WithTimeout(ctx, 55*time.Second)
	defer cancel()
	return w.runtime.service.processBattle(bounded, j.Args.TaskID)
}

type battleProgress struct {
	Page  int       `json:"page"`
	Until time.Time `json:"until"`
	Next  time.Time `json:"next"`
}

func (s *Service) processBattle(ctx context.Context, id int64) error {
	q := store.New(s.Pool)
	t, err := q.ClaimBattleTask(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	ev, err := q.GetEvent(ctx, t.EventID)
	if err != nil {
		return err
	}
	state, reason := "ready", ""
	next := time.Now().Add(5 * time.Minute)
	progress := battleProgress{}
	if json.Unmarshal(t.Progress, &progress) != nil {
		return ErrInvalid
	}
	if progress.Page < 1 {
		progress.Page = 1
	}
	if progress.Until.IsZero() {
		progress.Until = time.Now()
		if ev.EndsAt.Valid {
			progress.Until = ev.EndsAt.Time
		}
	}
	var fit eve.BattleFitting
	var losses eve.BattleLossPage
	var proof eve.BattleProof
	var fetchErr error
	bindings, fetchErr := s.Bindings(ctx, nil, []int64{t.CharacterID})
	if fetchErr != nil {
		return fetchErr
	}
	valid := false
	for _, b := range bindings {
		if b.UserID == t.AccountID.String() && subtle.ConstantTimeCompare(b.OwnerHash, t.OwnerHash) == 1 {
			valid = true
		}
	}
	entry, entryErr := q.BattleEntry(ctx, store.BattleEntryParams{EventID: t.EventID, CharacterID: t.CharacterID})
	if entryErr != nil {
		return entryErr
	}
	if !valid || !entry.Present || entry.AccountID != t.AccountID {
		state, reason = "blocked", "binding_changed"
	} else {
		if fetchErr == nil {
			proof, fetchErr = s.Battle.BattleCredential(ctx, t.CharacterID, t.OwnerHash, t.Kind)
		}
		if fetchErr == nil {
			if t.Kind == "fitting" {
				ship, e := q.GetShip(ctx, t.SnapshotID)
				if e != nil {
					return e
				}
				fit, fetchErr = s.Battle.BattleFitting(ctx, proof, ship.ShipTypeID, ship.ObservedAt.Time)
			} else {
				losses, fetchErr = s.Battle.BattleLosses(ctx, proof, progress.Page, ev.StartsAt.Time, progress.Until)
			}
		}
		if fetchErr != nil {
			var terminal bool
			var until time.Time
			reason, until, terminal = eve.BattleFailure(fetchErr)
			state = "pending"
			if terminal {
				state = "blocked"
			} else if reason != "rate_limited" {
				t.Failures++
				next = time.Now().Add(time.Duration(1<<min(t.Failures, 6)) * time.Minute)
				if t.Failures >= 5 {
					state = "failed"
				}
			}
			if until.After(next) {
				next = until
			}
		} else if t.Kind == "losses" {
			t.Failures = 0
			if losses.Next.After(progress.Next) {
				progress.Next = losses.Next
			}
			if losses.More {
				progress.Page++
				state = "pending"
				next = time.Now().Add(time.Second)
			} else {
				next = progress.Next
				progress = battleProgress{}
				if !ev.EndsAt.Valid || time.Now().Before(ev.EndsAt.Time.Add(24*time.Hour)) {
					state = "pending"
				}
			}
		}
	}
	// Publication locks share the identity -> credential -> event -> task order.
	// Use a fresh bounded context only for publishing timeout diagnostics, never network.
	publish, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(publish)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	bound, err := s.Bindings(publish, tx, []int64{t.CharacterID})
	if err != nil {
		return err
	}
	current := false
	for _, b := range bound {
		if b.UserID == t.AccountID.String() && subtle.ConstantTimeCompare(b.OwnerHash, t.OwnerHash) == 1 {
			current = true
		}
	}
	if !current {
		state, reason = "blocked", "binding_changed"
		fetchErr = ErrConflict
	}
	if fetchErr == nil && state != "blocked" {
		if err = s.Battle.GuardBattle(publish, tx, proof); err != nil {
			state, reason = "blocked", "reauthorize"
			fetchErr = err
		}
	}
	tq := store.New(tx)
	latestEvent, err := tq.LockEvent(publish, t.EventID)
	if err != nil {
		return err
	}
	if t.Kind == "losses" && latestEvent.Version != ev.Version && fetchErr == nil && reason == "" {
		state = "pending"
		progress = battleProgress{}
		next = time.Now().Add(5 * time.Minute)
	}
	currentTask, err := tq.LockBattleTask(publish, id)
	if err != nil {
		return err
	}
	if currentTask.Fence != t.Fence || !currentTask.LeaseUntil.Time.After(time.Now()) {
		return nil
	}
	latestEntry, err := tq.BattleEntry(publish, store.BattleEntryParams{EventID: t.EventID, CharacterID: t.CharacterID})
	if err != nil {
		return err
	}
	if !latestEntry.Present || latestEntry.AccountID != t.AccountID {
		state, reason = "blocked", "binding_changed"
		fetchErr = ErrConflict
	}
	if t.Kind == "fitting" {
		data := []byte("{}")
		status := reason
		if fetchErr == nil && state == "ready" {
			data, err = json.Marshal(fit)
			if err != nil {
				return err
			}
			status = "ready"
		} else if state == "pending" {
			status = "pending"
		}
		if status == "" {
			status = "esi_unavailable"
		}
		if err = tq.SaveFitting(publish, store.SaveFittingParams{ID: t.SnapshotID, FittingState: status, Fitting: data}); err != nil {
			return err
		}
	} else if fetchErr == nil && reason == "" {
		for _, loss := range losses.Losses {
			if loss.CharacterID != t.CharacterID || loss.At.Before(latestEvent.StartsAt.Time) || latestEvent.EndsAt.Valid && loss.At.After(latestEvent.EndsAt.Time) {
				continue
			}
			data, e := json.Marshal(loss.Items)
			if e != nil {
				return e
			}
			if err = tq.SaveLoss(publish, store.SaveLossParams{EventID: t.EventID, CharacterID: t.CharacterID, KillmailID: loss.ID, OccurredAt: stamp(loss.At), ShipTypeID: loss.ShipTypeID, SolarSystemID: loss.SolarSystemID, Items: data}); err != nil {
				return err
			}
		}
	}
	data, _ := json.Marshal(progress)
	if err = tq.CompleteBattleTask(publish, store.CompleteBattleTaskParams{ID: id, State: state, Reason: reason, NextDueAt: stamp(next), Progress: data, Failures: t.Failures, Fence: t.Fence}); err != nil {
		return err
	}
	return tx.Commit(publish)
}

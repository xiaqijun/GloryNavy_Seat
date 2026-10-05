package eve

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
	platformjobs "glorynavy.local/seat/internal/platform/jobs"
)

type syncArgs struct {
	TargetID   int64 `json:"target_id"`
	Generation int64 `json:"generation"`
}
type profileArgs syncArgs

func (profileArgs) Kind() string { return "eve.character-profile.v1" }

type authorizationArgs syncArgs

func (authorizationArgs) Kind() string { return "eve.character-authorization.v1" }

type structureArgs syncArgs

func (structureArgs) Kind() string { return "eve.corporation-structures.v1" }

type dispatchArgs struct{}

func (dispatchArgs) Kind() string { return "eve.sync-dispatch.v1" }

// SyncService owns EVE resource scheduling. Worker payloads contain IDs only.
type SyncService struct {
	pool            *pgxpool.Pool
	auth            *AuthorizationService
	queue           *river.Client[pgx.Tx]
	logger          *slog.Logger
	available       atomic.Bool
	validBinding    func(context.Context, int64, []byte) (bool, error)
	sde             *StaticDataService
	onlineEnabled   bool
	fittingsEnabled bool
	skillsEnabled   bool
	lossesEnabled   bool
	walletEnabled   bool
}

func NewSync(pool *pgxpool.Pool, auth *AuthorizationService, logger *slog.Logger, validBinding func(context.Context, int64, []byte) (bool, error), extensions ...platformjobs.Extension) (*SyncService, error) {
	s := &SyncService{pool: pool, auth: auth, logger: logger, validBinding: validBinding}
	workers := river.NewWorkers()
	river.AddWorker(workers, &dispatchWorker{s: s})
	river.AddWorker(workers, &profileWorker{s: s})
	river.AddWorker(workers, &authorizationWorker{s: s})
	river.AddWorker(workers, &structureWorker{s: s})
	river.AddWorker(workers, &onlineWorker{s: s})
	river.AddWorker(workers, &fittingWorker{s: s})
	river.AddWorker(workers, &lossWorker{s: s})
	river.AddWorker(workers, &walletWorker{s: s})
	river.AddWorker(workers, &contractsWorker{s: s})
	river.AddWorker(workers, &contractDetailWorker{s: s})
	river.AddWorker(workers, &sdeUpdateWorker{s: s})
	periodic := []*river.PeriodicJob{river.NewPeriodicJob(river.PeriodicInterval(30*time.Second), func() (river.JobArgs, *river.InsertOpts) {
		return dispatchArgs{}, &river.InsertOpts{Queue: "eve_control", UniqueOpts: platformjobs.ActiveUnique()}
	}, &river.PeriodicJobOpts{RunOnStart: true})}
	for _, x := range extensions {
		periodic = append(periodic, x.Register(workers)...)
	}
	client, err := platformjobs.New(pool, workers, periodic, logger)
	if err != nil {
		return nil, err
	}
	s.queue = client
	for _, x := range extensions {
		x.Bind(client)
	}
	if auth != nil {
		auth.sync = s
	}
	return s, nil
}

// SetStaticData enables public SDE scheduling before the runtime starts.
func (s *SyncService) SetStaticData(service *StaticDataService) { s.sde = service }

// SetUserAgent configures the operator contact before the worker starts.
func (s *SyncService) SetUserAgent(value string) {
	if s.auth != nil {
		s.auth.esi.shared.SetUserAgent(value)
	}
}

func (s *SyncService) Run(ctx context.Context) {
	if s.auth == nil && s.sde == nil {
		return
	}
	for {
		if ctx.Err() != nil {
			return
		}
		if err := s.queue.Start(ctx); err != nil {
			s.logger.Error("ESI queue could not start; check database and River migrations; retrying")
			select {
			case <-ctx.Done():
				return
			case <-time.After(15 * time.Second):
			}
			continue
		}
		break
	}
	s.available.Store(s.auth != nil)
	defer s.available.Store(false)
	<-ctx.Done()
	s.available.Store(false)
	stop, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := s.queue.Stop(stop); err != nil {
		force, end := context.WithTimeout(context.Background(), 5*time.Second)
		defer end()
		_ = s.queue.StopAndCancel(force)
	}
}

func (s *SyncService) enqueue(ctx context.Context, tx pgx.Tx, t store.EveSyncTarget) (string, error) {
	if t.State == "blocked" {
		return "blocked", nil
	}
	if t.ActiveJobID.Valid {
		job, err := s.queue.JobGetTx(ctx, tx, t.ActiveJobID.Int64)
		if err != nil && !errors.Is(err, river.ErrNotFound) {
			return "", err
		}
		if err == nil && job.State != rivertype.JobStateCompleted && job.State != rivertype.JobStateCancelled && job.State != rivertype.JobStateDiscarded {
			return "already_pending", nil
		}
		if err := store.New(tx).ResetSyncTarget(ctx, t.ID); err != nil {
			return "", err
		}
	}
	var args river.JobArgs
	if isWallet(t.Resource) {
		if !s.walletEnabled {
			return "disabled", nil
		}
		args = walletArgs{t.ID, t.Generation, t.Resource}
	} else {
		switch t.Resource {
		case "profile":
			args = profileArgs{t.ID, t.Generation}
		case "online":
			if !s.onlineEnabled {
				return "disabled", nil
			}
			args = onlineArgs{t.ID, t.Generation}
		case "authorization":
			args = authorizationArgs{t.ID, t.Generation}
		case "corporation_structures":
			args = structureArgs{t.ID, t.Generation}
		case "killmails":
			if !s.lossesEnabled {
				return "disabled", nil
			}
			args = lossArgs{t.ID, t.Generation}
		case "fittings", "skills", "skillqueue":
			if !s.resourceEnabled(t.Resource) {
				return "disabled", nil
			}
			args = fittingArgs{t.ID, t.Generation, t.Resource}
		case "character_contracts", "corporation_contracts":
			args = contractsArgs{t.ID, t.Generation}
		default:
			return "blocked", errors.New("unregistered ESI resource")
		}
	}
	opts := &river.InsertOpts{Queue: "eve_characters", UniqueOpts: platformjobs.ActiveUnique(), MaxAttempts: 5, ScheduledAt: t.NextDueAt.Time}
	if isContracts(t.Resource) {
		opts.Queue = "eve_contracts"
	}
	res, err := s.queue.InsertTx(ctx, tx, args, opts)
	if err != nil {
		return "", err
	}
	if err = store.New(tx).SetSyncJob(ctx, store.SetSyncJobParams{ID: t.ID, ActiveJobID: pgtype.Int8{Int64: res.Job.ID, Valid: true}}); err != nil {
		return "", err
	}
	if t.NextDueAt.Time.After(time.Now()) {
		return "deferred", nil
	}
	return "queued", nil
}

func (s *SyncService) scheduleCharacter(ctx context.Context, tx pgx.Tx, id int64) error {
	rows, err := store.New(tx).ListCharacterSync(ctx, id)
	if err != nil {
		return err
	}
	for _, t := range rows {
		if _, err = s.enqueue(ctx, tx, t); err != nil {
			return err
		}
	}
	return nil
}

func (s *SyncService) dispatch(ctx context.Context) error {
	if s.sde != nil {
		if err := s.sde.enqueueDue(ctx, s.queue); err != nil {
			s.logger.Warn("SDE dispatch deferred")
			if s.auth == nil {
				return err
			}
		}
	}
	if s.auth == nil {
		return nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	q := store.New(tx)
	if s.walletEnabled {
		if err := store.SeedWalletTargets(ctx, tx); err != nil {
			return err
		}
	}
	if s.onlineEnabled {
		if err := q.SeedOnlineTargets(ctx); err != nil {
			return err
		}
	}
	if s.lossesEnabled {
		if err := store.SeedLossTargets(ctx, tx); err != nil {
			return err
		}
	}
	if s.fittingsEnabled || s.skillsEnabled {
		if err := q.SeedFittingTargets(ctx, store.SeedFittingTargetsParams{FittingsEnabled: s.fittingsEnabled, SkillsEnabled: s.skillsEnabled}); err != nil {
			return err
		}
	}
	rows, err := q.LockDueTargets(ctx, store.LockDueTargetsParams{OnlineEnabled: s.onlineEnabled, FittingsEnabled: s.fittingsEnabled, SkillsEnabled: s.skillsEnabled, LossesEnabled: s.lossesEnabled, WalletEnabled: s.walletEnabled})
	if err != nil {
		return err
	}
	for _, t := range rows {
		if _, err = s.enqueue(ctx, tx, t); err != nil {
			return err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	if err = qCleanup(ctx, s.pool); err != nil {
		s.logger.Warn("ESI maintenance deferred")
	}
	return nil
}
func qCleanup(ctx context.Context, pool *pgxpool.Pool) error {
	q := store.New(pool)
	if err := q.CleanupOnlineSamples(ctx); err != nil {
		return err
	}
	if err := q.CleanupESIBuckets(ctx); err != nil {
		return err
	}
	if err := q.CleanupESICharges(ctx); err != nil {
		return err
	}
	if err := q.CleanupTokenEvents(ctx); err != nil {
		return err
	}
	if err := q.CleanupSyncCache(ctx); err != nil {
		return err
	}
	return q.CleanupSyncRuns(ctx)
}

type dispatchWorker struct {
	river.WorkerDefaults[dispatchArgs]
	s *SyncService
}

func (w *dispatchWorker) Work(ctx context.Context, _ *river.Job[dispatchArgs]) error {
	return w.s.dispatch(ctx)
}

type profileWorker struct {
	river.WorkerDefaults[profileArgs]
	s *SyncService
}

func (w *profileWorker) Work(ctx context.Context, j *river.Job[profileArgs]) error {
	return w.s.work(ctx, syncArgs(j.Args), j.ID, "profile")
}

type authorizationWorker struct {
	river.WorkerDefaults[authorizationArgs]
	s *SyncService
}

func (w *authorizationWorker) Work(ctx context.Context, j *river.Job[authorizationArgs]) error {
	return w.s.work(ctx, syncArgs(j.Args), j.ID, "authorization")
}

type structureWorker struct {
	river.WorkerDefaults[structureArgs]
	s *SyncService
}

func (w *structureWorker) Work(ctx context.Context, j *river.Job[structureArgs]) error {
	return w.s.work(ctx, syncArgs(j.Args), j.ID, "corporation_structures")
}

func (s *SyncService) work(parent context.Context, args syncArgs, jobID int64, resource string) error {
	if s.auth == nil {
		return river.JobSnooze(time.Hour)
	}
	ctx, cancel := context.WithTimeout(parent, 55*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	q := store.New(tx)
	t, err := q.GetSyncTarget(ctx, args.TargetID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if t.Generation != args.Generation || t.Resource != resource || t.CompletedJobID.Int64 == jobID || t.ActiveJobID.Int64 != jobID {
		return nil
	}
	if t.NextDueAt.Time.After(time.Now()) {
		return river.JobSnooze(time.Until(t.NextDueAt.Time))
	}
	t, err = q.ClaimSync(ctx, store.ClaimSyncParams{ID: t.ID, Generation: args.Generation, ActiveJobID: pgtype.Int8{Int64: jobID, Valid: true}})
	if errors.Is(err, pgx.ErrNoRows) {
		return river.JobSnooze(30 * time.Second)
	}
	if err != nil {
		return err
	}
	if err = q.AbandonSyncRuns(ctx, t.ID); err != nil {
		return err
	}
	if err = q.BeginSyncRun(ctx, store.BeginSyncRunParams{TargetID: t.ID, JobID: jobID, Fence: t.Fence}); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}

	credential, err := store.New(s.pool).GetCredential(ctx, t.CharacterID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if credential.GrantGeneration != args.Generation {
		return nil
	}
	valid := true
	if s.validBinding != nil {
		valid, err = s.validBinding(ctx, t.CharacterID, credential.OwnerHash)
		if err != nil {
			return err
		}
	}
	var result syncResult
	if !valid {
		err = syncFault{Reason: "identity_changed", Status: 0, Temporary: false}
	}
	if err == nil {
		result, err = s.collect(ctx, t, credential)
	}
	return s.finish(ctx, t, credential, jobID, result, err)
}

type syncResult struct {
	wallet        *walletBatch
	losses        *lossBatch
	fitting       *fittingObservation
	online        *onlineObservation
	contracts     *contractPage
	profile       *characterProfile
	snapshot      *esiSnapshot
	structures    *structureObservation
	next, content time.Time
}
type characterProfile struct {
	Name        string `json:"name"`
	Corporation int64  `json:"corporation_id"`
	Alliance    int64  `json:"alliance_id"`
}

func (s *SyncService) collect(ctx context.Context, t store.EveSyncTarget, c store.EveCredential) (syncResult, error) {
	var result syncResult
	if isWallet(t.Resource) {
		return s.collectWallet(ctx, t, c)
	}
	if t.Resource == "killmails" {
		return s.collectLosses(ctx, t, c)
	}
	if t.Resource == "online" {
		return s.collectOnline(ctx, t, c)
	}
	if t.Resource == "fittings" || t.Resource == "skills" || t.Resource == "skillqueue" {
		return s.collectFittingResource(ctx, t, c)
	}
	if isContracts(t.Resource) {
		return s.collectContracts(ctx, t, c)
	}
	if t.Resource == "profile" {
		var p characterProfile
		response, err := s.auth.esi.Request(ctx, ESIRequest{Method: "GET", Path: characterPath(c.CharacterID)}, &p)
		if err != nil {
			return result, err
		}
		if p.Name == "" || p.Corporation <= 0 {
			return result, syncFault{Reason: "invalid_response", Status: 200, Temporary: false}
		}
		result.profile = &p
		result.next = response.ExpiresAt
		result.content = response.ContentUpdatedAt
		return result, nil
	}
	if t.Resource == "corporation_structures" {
		return s.collectStructures(ctx, t, c)
	}
	if c.RolesNotBefore.Time.After(time.Now()) {
		result.next = c.RolesNotBefore.Time
		return result, syncFault{Reason: "corporation_changed", Status: 0, Temporary: true}
	}
	snapshot, err := s.auth.esi.snapshot(ctx, c.CharacterID, c.GrantGeneration)
	result.snapshot = &snapshot
	result.next = snapshot.Next
	result.content = snapshot.Content
	return result, err
}

func (s *SyncService) finish(ctx context.Context, t store.EveSyncTarget, c store.EveCredential, jobID int64, result syncResult, fetchErr error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	q := store.New(tx)
	current, err := q.GetCredentialForUpdate(ctx, c.CharacterID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	locked, err := q.LockSyncTarget(ctx, t.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if current.GrantGeneration != c.GrantGeneration || locked.Fence != t.Fence || !locked.LeaseUntil.Time.After(time.Now()) {
		return nil
	}
	if current.State == "reauthorize" && t.Resource != "profile" {
		fetchErr = ErrReauthorize
	}
	if fetchErr == nil && result.contracts != nil && result.contracts.kind == "corporation" {
		if err := contractSource(ctx, q, current, result.contracts.owner); err != nil {
			fetchErr = err
		}
	}
	if fetchErr == nil && result.wallet != nil && result.wallet.Kind == "corporation" {
		owner, e := walletCorporation(ctx, tx, current, t.Resource)
		if e != nil {
			fetchErr = e
		} else if owner != result.wallet.Owner {
			fetchErr = syncFault{Reason: "corporation_changed", Temporary: true}
		}
	}
	next := result.next
	state, reason, outcome := "idle", "", "success"
	failures := int32(0)
	code := 0
	old, err := q.GetAuthorization(ctx, t.CharacterID)
	if err != nil {
		return err
	}
	// Affiliation changes invalidate old facts even if the subsequent role request failed.
	corporation := int64(0)
	if result.snapshot != nil {
		corporation = result.snapshot.CorporationID
	}
	if result.profile != nil {
		corporation = result.profile.Corporation
	}
	moved := corporation > 0 && old.CorporationID.Valid && corporation != old.CorporationID.Int64
	if moved {
		if err = q.InvalidateSnapshot(ctx, t.CharacterID); err != nil {
			return err
		}
		barrier := next
		if result.snapshot != nil {
			barrier = result.snapshot.Barrier
		}
		if err = q.SetRoleBarrier(ctx, store.SetRoleBarrierParams{CharacterID: t.CharacterID, UntilAt: timestamp(barrier)}); err != nil {
			return err
		}
		if t.Resource == "authorization" {
			next = barrier
			result.next = barrier
		}
		if t.Resource == "authorization" {
			fetchErr = syncFault{Reason: "corporation_changed", Status: 0, Temporary: true}
		}
	}
	if t.Resource == "authorization" && current.RolesNotBefore.Time.After(time.Now()) {
		fetchErr = syncFault{Reason: "corporation_changed", Status: 0, Temporary: true}
		result.next = current.RolesNotBefore.Time
	}
	success := fetchErr == nil
	var snooze time.Duration
	if fetchErr != nil {
		outcome = "failed"
		failures = t.Failures + 1
		state = "deferred"
		reason = "upstream_unavailable"
		next = time.Now().Add(time.Duration(1<<min(failures, 5))*15*time.Second + time.Duration(rand.IntN(15))*time.Second)
		var fault syncFault
		var retry retryError
		switch {
		case errors.Is(fetchErr, ErrReauthorize):
			state = "blocked"
			reason = "reauthorize"
		case errors.As(fetchErr, &retry):
			reason = "rate_limited"
			next = retry.Until
			failures = t.Failures
		case errors.As(fetchErr, &fault):
			reason = fault.Reason
			code = fault.Status
			if !fault.Temporary {
				state = "blocked"
			}
		}
		if reason == "corporation_changed" {
			failures = t.Failures
			if result.next.After(next) {
				next = result.next
			}
		}
		if reason == "pagination_changed" {
			if isWallet(t.Resource) {
				if err = store.DeleteWalletCursor(ctx, tx, t.ID); err != nil {
					return err
				}
			}
			if t.Resource == "killmails" {
				if err = store.DeleteLossCursor(ctx, tx, t.ID); err != nil {
					return err
				}
			}
			if err = q.DeleteContractCursor(ctx, t.ID); err != nil {
				return err
			}
		}
		if reason == "shared_source" || reason == "authorization_pending" {
			failures = t.Failures
			next = time.Now().Add(time.Minute)
		}
		if isWallet(t.Resource) && reason == "missing_role" {
			failures = t.Failures
			next = time.Now().Add(5 * time.Minute)
		}
		if state == "blocked" {
			if t.Resource == "authorization" {
				if err = q.InvalidateSnapshot(ctx, t.CharacterID); err != nil {
					return err
				}
			}
			if reason == "reauthorize" {
				if err = q.RevokeCredential(ctx, t.CharacterID); err != nil {
					return err
				}
				if _, err = q.AdvanceGeneration(ctx, t.CharacterID); err != nil {
					return err
				}
				if err = q.BlockCharacterSync(ctx, store.BlockCharacterSyncParams{CharacterID: t.CharacterID, Reason: "reauthorize"}); err != nil {
					return err
				}
				if err = q.DeletePrivateCache(ctx, t.CharacterID); err != nil {
					return err
				}
				if err = q.InvalidateSnapshot(ctx, t.CharacterID); err != nil {
					return err
				}
			}
		} else if failures >= 5 {
			state = "failed"
			next = time.Now().Add(time.Hour)
		} else {
			snooze = max(time.Second, time.Until(next))
		}
	} else {
		if result.wallet != nil {
			if err = s.saveWalletBatch(ctx, tx, t, c, result.wallet); err != nil {
				return err
			}
			if !result.wallet.Done {
				success = false
				state = "deferred"
				reason = "pagination"
				outcome = "progress"
				next = time.Now().Add(time.Second)
				snooze = time.Second
			}
		}
		if result.losses != nil {
			if err = s.saveLossBatch(ctx, tx, t, c, result.losses); err != nil {
				return err
			}
			if !result.losses.Done {
				success = false
				state = "deferred"
				reason = "pagination"
				outcome = "progress"
				next = time.Now().Add(time.Second)
				snooze = time.Second
			}
		}
		if result.contracts != nil {
			if err = s.saveContractPage(ctx, tx, t, c, result.contracts); err != nil {
				return err
			}
			if result.contracts.page < result.contracts.pages {
				success = false
				state = "deferred"
				reason = "pagination"
				outcome = "progress"
				next = time.Now().Add(time.Second)
				snooze = time.Second
			}
		}
		if result.fitting != nil {
			if err = q.SaveFittingSnapshot(ctx, store.SaveFittingSnapshotParams{CharacterID: c.CharacterID, Resource: t.Resource, Generation: c.GrantGeneration, ObservedAt: timestamp(result.fitting.at), Payload: result.fitting.payload}); err != nil {
				return err
			}
		}
		if result.profile != nil {
			p := result.profile
			if err = q.SaveCharacterProfile(ctx, store.SaveCharacterProfileParams{CharacterID: t.CharacterID, Name: p.Name, CorporationID: p.Corporation, AllianceID: p.Alliance, ValidUntil: timestamp(next)}); err != nil {
				return err
			}
			if err = q.SetSyncName(ctx, store.SetSyncNameParams{CharacterID: t.CharacterID, DisplayName: p.Name}); err != nil {
				return err
			}
		}
		if result.snapshot != nil {
			p := result.snapshot
			clean := func(v []string) []string {
				if v == nil {
					return []string{}
				}
				return v
			}
			if err = q.SaveSnapshot(ctx, store.SaveSnapshotParams{CharacterID: t.CharacterID, OwnerHash: c.OwnerHash, CorporationID: p.CorporationID, CorporationName: p.Corporation.Name, AllianceID: p.Corporation.Alliance, CeoID: p.Corporation.CEO, Roles: clean(p.Roles.Roles), RolesAtHq: clean(p.Roles.HQ), RolesAtBase: clean(p.Roles.Base), RolesAtOther: clean(p.Roles.Other), SyncedAt: timestamp(time.Now()), ValidUntil: timestamp(next.Add(5 * time.Minute))}); err != nil {
				return err
			}
		}
		if result.structures != nil {
			p := result.structures
			if err = q.SaveStructureSnapshot(ctx, store.SaveStructureSnapshotParams{CharacterID: c.CharacterID, Generation: c.GrantGeneration, OwnerHash: c.OwnerHash, CorporationID: p.corporationID, CorporationName: p.corporationName, ObservedAt: timestamp(p.observedAt), ValidUntil: timestamp(p.validUntil), Payload: p.payload}); err != nil {
				return err
			}
		}
	}
	if t.Resource == "online" && (!success || result.online != nil) {
		observed := time.Now()
		value := pgtype.Bool{}
		if success && result.online != nil {
			observed = result.online.at
			value = pgtype.Bool{Bool: result.online.value, Valid: true}
		}
		corpID := int64(0)
		if old.ValidUntil.Valid && old.ValidUntil.Time.After(time.Now()) && old.CorporationID.Valid {
			corpID = old.CorporationID.Int64
		}
		if err = q.SaveOnlineSample(ctx, store.SaveOnlineSampleParams{CorporationID: corpID, CharacterID: c.CharacterID, Generation: c.GrantGeneration, ObservedAt: timestamp(observed), Online: value}); err != nil {
			return err
		}
	}
	if t.Resource == "authorization" && reason != "reauthorize" {
		authState := "ready"
		if !success {
			authState = "retry"
		}
		if reason == "corporation_changed" || state == "blocked" {
			authState = "pending"
		}
		if err = q.UpdateCredential(ctx, store.UpdateCredentialParams{CharacterID: c.CharacterID, Sealed: current.Sealed, State: authState, NextSyncAt: timestamp(next), Scopes: current.Scopes}); err != nil {
			return err
		}
	}
	// Waiting for a shared source, authorization cache or budget is not a failed fetch.
	if state == "deferred" && (reason == "rate_limited" || reason == "shared_source" || reason == "authorization_pending" || reason == "corporation_changed" || (isWallet(t.Resource) && reason == "missing_role")) {
		outcome = "deferred"
	}
	if err = q.FinishSyncRun(ctx, store.FinishSyncRunParams{TargetID: t.ID, Fence: t.Fence, Outcome: outcome, Reason: reason, HttpStatus: int32(code)}); err != nil {
		return err
	}
	if snooze > 0 {
		if _, err = q.DeferSyncTarget(ctx, store.DeferSyncTargetParams{ID: t.ID, Fence: t.Fence, Reason: reason, NextDueAt: timestamp(next), Failures: failures}); err != nil {
			return err
		}
	} else {
		content := pgtype.Timestamptz{}
		if !result.content.IsZero() {
			content = timestamp(result.content)
		}
		_, err = q.FinishSyncTarget(ctx, store.FinishSyncTargetParams{ID: t.ID, Fence: t.Fence, Generation: t.Generation, JobID: pgtype.Int8{Int64: jobID, Valid: true}, State: state, Reason: reason, NextDueAt: timestamp(next.Add(time.Duration(rand.IntN(20)+1) * time.Second)), Failures: failures, Success: success, ValidUntil: timestamp(next), ContentUpdatedAt: content})
		if err != nil {
			return err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	s.logger.Info("ESI resource processed", "target_id", t.ID, "resource", t.Resource, "outcome", outcome, "reason", reason)
	if snooze > 0 {
		return river.JobSnooze(snooze)
	}
	return nil
}

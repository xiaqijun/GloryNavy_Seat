package identity

import (
	"context"
	"crypto/subtle"
	"errors"
	"slices"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"glorynavy.local/seat/internal/modules/identity/internal/store"
	"glorynavy.local/seat/internal/platform/jobs"
)

var ErrCleanupConfig = errors.New("invalid cleanup configuration")
var ErrCleanupForbidden = errors.New("cleanup administrator required")
var ErrCleanupConflict = errors.New("cleanup configuration changed")

type CorporationEvidence struct {
	ID            int64
	Owner         []byte
	CorporationID int64
	Name          string
	Reliable      bool
}
type CorporationCleanup struct {
	Pool             *pgxpool.Pool
	Administrator    func(context.Context, string) (bool, error)
	Evidence         func(context.Context, pgx.Tx, []int64) ([]CorporationEvidence, error)
	Remove           CharacterCleanup
	CorporationNames func(context.Context) ([]CleanupCorporation, error)
}
type CleanupSettings struct {
	store.CleanupPolicy
	CorporationIDs     []string             `json:"corporation_ids"`
	CorporationOptions []CleanupCorporation `json:"corporation_options"`
}
type CleanupInput struct {
	Enabled        bool     `json:"enabled"`
	CorporationIDs []string `json:"corporation_ids"`
	Version        int64    `json:"version,string"`
}
type CleanupPreviewItem struct {
	ID              string `json:"character_id"`
	Name            string `json:"name"`
	CorporationID   string `json:"corporation_id"`
	CorporationName string `json:"corporation_name"`
}
type CleanupPreview struct {
	Remove  int                  `json:"remove_count"`
	Keep    int                  `json:"keep_count"`
	Unknown int                  `json:"unknown_count"`
	Items   []CleanupPreviewItem `json:"items"`
}

func cleanupIDs(raw []string) ([]int64, error) {
	if len(raw) > 50 {
		return nil, ErrCleanupConfig
	}
	out := []int64{}
	for _, v := range raw {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 || strconv.FormatInt(n, 10) != v {
			return nil, ErrCleanupConfig
		}
		out = append(out, n)
	}
	slices.Sort(out)
	return slices.Compact(out), nil
}
func (s *CorporationCleanup) authorized(ctx context.Context, actor string) error {
	if s.Administrator == nil {
		return ErrCleanupForbidden
	}
	ok, err := s.Administrator(ctx, actor)
	if err != nil {
		return err
	}
	if !ok {
		return ErrCleanupForbidden
	}
	return nil
}
func (s *CorporationCleanup) Settings(ctx context.Context, actor string) (CleanupSettings, error) {
	if err := s.authorized(ctx, actor); err != nil {
		return CleanupSettings{}, err
	}
	p, err := store.ReadCleanupPolicy(ctx, s.Pool, false)
	out := CleanupSettings{CleanupPolicy: p, CorporationIDs: []string{}, CorporationOptions: []CleanupCorporation{}}
	if err != nil {
		return out, err
	}
	if s.CorporationNames != nil {
		out.CorporationOptions, err = s.CorporationNames(ctx)
		if err != nil {
			return out, err
		}
	}
	for _, id := range p.Corporations {
		out.CorporationIDs = append(out.CorporationIDs, strconv.FormatInt(id, 10))
	}
	return out, err
}
func (s *CorporationCleanup) Save(ctx context.Context, actor string, in CleanupInput) (CleanupSettings, error) {
	if err := s.authorized(ctx, actor); err != nil {
		return CleanupSettings{}, err
	}
	ids, err := cleanupIDs(in.CorporationIDs)
	if err != nil || in.Version <= 0 || (in.Enabled && len(ids) == 0) {
		return CleanupSettings{}, ErrCleanupConfig
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return CleanupSettings{}, err
	}
	defer tx.Rollback(context.Background())
	if err = store.SaveCleanupPolicy(ctx, tx, actor, in.Enabled, ids, in.Version); errors.Is(err, pgx.ErrNoRows) {
		return CleanupSettings{}, ErrCleanupConflict
	} else if err != nil {
		return CleanupSettings{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return CleanupSettings{}, err
	}
	return s.Settings(ctx, actor)
}
func cleanupDecision(c store.CleanupCharacter, e CorporationEvidence, allowed []int64) string {
	if c.Status != "active" || !e.Reliable || e.CorporationID <= 0 || subtle.ConstantTimeCompare(c.Owner, e.Owner) != 1 {
		return "unknown"
	}
	if slices.Contains(allowed, e.CorporationID) {
		return "keep"
	}
	return "remove"
}
func (s *CorporationCleanup) evidence(ctx context.Context, tx pgx.Tx, chars []store.CleanupCharacter) (map[int64]CorporationEvidence, error) {
	if s.Evidence == nil {
		return nil, errors.New("cleanup membership unavailable")
	}
	ids := make([]int64, 0, len(chars))
	for _, c := range chars {
		ids = append(ids, c.ID)
	}
	rows, err := s.Evidence(ctx, tx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]CorporationEvidence, len(rows))
	for _, r := range rows {
		out[r.ID] = r
	}
	return out, nil
}
func (s *CorporationCleanup) Preview(ctx context.Context, actor string, raw []string) (CleanupPreview, error) {
	out := CleanupPreview{Items: []CleanupPreviewItem{}}
	if err := s.authorized(ctx, actor); err != nil {
		return out, err
	}
	ids, err := cleanupIDs(raw)
	if err != nil || len(ids) == 0 {
		return out, ErrCleanupConfig
	}
	chars, err := store.CleanupCharacters(ctx, s.Pool, "")
	if err != nil {
		return out, err
	}
	facts, err := s.evidence(ctx, nil, chars)
	if err != nil {
		return out, err
	}
	for _, c := range chars {
		e := facts[c.ID]
		switch cleanupDecision(c, e, ids) {
		case "keep":
			out.Keep++
		case "unknown":
			out.Unknown++
		case "remove":
			out.Remove++
			if len(out.Items) < 100 {
				out.Items = append(out.Items, CleanupPreviewItem{strconv.FormatInt(c.ID, 10), c.Name, strconv.FormatInt(e.CorporationID, 10), e.Name})
			}
		}
	}
	return out, nil
}

// A transaction handles one account, preserving the identity lock order and
// atomically advancing a durable cursor. Interrupted runs resume that cursor.
func (s *CorporationCleanup) runAccount(ctx context.Context) (bool, error) {
	if s.Remove == nil || s.Evidence == nil {
		return false, errors.New("cleanup services unavailable")
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(context.Background())
	p, err := store.ReadCleanupPolicy(ctx, tx, true)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !p.Enabled || len(p.Corporations) == 0 || p.NextRun == nil || p.NextRun.After(time.Now()) {
		return false, nil
	}
	if p.Started == nil {
		if err = store.StartCleanup(ctx, tx); err != nil {
			return false, err
		}
		p.Removed = 0
		p.Skipped = 0
		p.After = nil
	}
	user, err := store.NextCleanupUser(ctx, tx, p.After)
	if errors.Is(err, pgx.ErrNoRows) {
		if err = store.FinishCleanup(ctx, tx, p); err != nil {
			return false, err
		}
		return false, tx.Commit(ctx)
	}
	if err != nil {
		return false, err
	}
	chars, err := store.CleanupCharacters(ctx, tx, user)
	if err != nil {
		return false, err
	}
	for _, c := range chars {
		if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, c.ID); err != nil {
			return false, err
		}
	}
	// Reuse identity-owned store functions, including the account row lock.
	q := store.New(tx)
	var uid pgtype.UUID
	if err = uid.Scan(user); err != nil {
		return false, err
	}
	if _, err = q.LockUser(ctx, uid); err != nil {
		return false, err
	}
	current, err := store.CleanupCharacters(ctx, tx, user)
	if err != nil {
		return false, err
	}
	// A concurrently added/moved character needs its own correctly ordered lock.
	if !sameCleanupRoster(chars, current) {
		return true, ErrCleanupConflict
	}
	facts, err := s.evidence(ctx, tx, current)
	if err != nil {
		return false, err
	}
	removed, skipped := 0, 0
	for _, c := range current {
		e := facts[c.ID]
		switch cleanupDecision(c, e, p.Corporations) {
		case "unknown":
			skipped++
		case "remove":
			if err = s.Remove(ctx, tx, c.ID, user); err != nil {
				return false, err
			}
			if err = store.RemoveCleanupCharacter(ctx, tx, c, e.CorporationID); err != nil {
				return false, err
			}
			removed++
		}
	}
	if err = store.FinishCleanupAccount(ctx, tx, user, removed, skipped); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}
func sameCleanupRoster(a, b []store.CleanupCharacter) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID || a[i].User != b[i].User || a[i].Status != b[i].Status || subtle.ConstantTimeCompare(a[i].Owner, b[i].Owner) != 1 {
			return false
		}
	}
	return true
}
func (s *CorporationCleanup) RunDue(ctx context.Context) error {
	for i := 0; i < 25; i++ {
		more, err := s.runAccount(ctx)
		if err != nil {
			return err
		}
		if !more {
			return nil
		}
	}
	return nil
}

type corporationCleanupArgs struct{}

func (corporationCleanupArgs) Kind() string { return "identity.corporation-cleanup.v1" }

type corporationCleanupWorker struct {
	river.WorkerDefaults[corporationCleanupArgs]
	service *CorporationCleanup
}

func (w *corporationCleanupWorker) Work(ctx context.Context, _ *river.Job[corporationCleanupArgs]) error {
	return w.service.RunDue(ctx)
}
func (s *CorporationCleanup) Extension() jobs.Extension {
	return jobs.Extension{Bind: func(*river.Client[pgx.Tx]) {}, Register: func(w *river.Workers) []*river.PeriodicJob {
		river.AddWorker(w, &corporationCleanupWorker{service: s})
		return []*river.PeriodicJob{river.NewPeriodicJob(river.PeriodicInterval(time.Minute), func() (river.JobArgs, *river.InsertOpts) {
			return corporationCleanupArgs{}, &river.InsertOpts{Queue: "eve_control", UniqueOpts: jobs.ActiveUnique(), MaxAttempts: 5}
		}, &river.PeriodicJobOpts{RunOnStart: true})}
	}}
}

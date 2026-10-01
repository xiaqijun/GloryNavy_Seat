package fittings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/fittings/internal/store"
	"glorynavy.local/seat/internal/platform/locale"
	"strings"
	"time"
)

type LibraryCorporation struct {
	ID              int64  `json:"id,string"`
	Name            string `json:"name"`
	CanCreateSkills bool   `json:"can_create_skills"`
}
type LibraryCharacter struct {
	ID      int64  `json:"id,string"`
	Name    string `json:"name"`
	CanSave bool   `json:"can_save"`
}
type LibraryEdit struct {
	CorporationID int64  `json:"corporation_id,string"`
	Version       int64  `json:"version,string"`
	RequestKey    string `json:"request_key"`
	EFT           string `json:"eft"`
	Description   string `json:"description"`
}
type LibraryEntry struct {
	ID            int64     `json:"id,string"`
	CorporationID int64     `json:"corporation_id,string"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	Version       int64     `json:"version,string"`
	UpdatedAt     time.Time `json:"updated_at"`
	Fit           Fit       `json:"fit"`
	ShipName      string    `json:"ship_name"`
	Group         string    `json:"group"`
	EFT           string    `json:"eft"`
}

func (s *Service) visibleLibrary(ctx context.Context, user string, corp int64) error {
	if s.LibraryCorporations == nil {
		return pgx.ErrNoRows
	}
	rows, e := s.LibraryCorporations(ctx, user)
	if e != nil {
		return e
	}
	for _, r := range rows {
		if r.ID == corp {
			return nil
		}
	}
	return pgx.ErrNoRows
}
func (s *Service) libraryEntry(ctx context.Context, v store.Library, batch ...map[int64]eve.StaticTypeName) (LibraryEntry, error) {
	var f Fit
	if e := json.Unmarshal(v.Fit, &f); e != nil {
		return LibraryEntry{}, e
	}
	var names map[int64]eve.StaticTypeName
	if len(batch) > 0 {
		names = batch[0]
	} else {
		var e error
		names, e = s.Names.TypeNames(ctx, []int64{f.ShipTypeID})
		if e != nil {
			return LibraryEntry{}, e
		}
	}
	ref := references[f.ShipTypeID]
	name := locale.Choose(ctx, ref.Name, ref.English)
	if n, ok := names[f.ShipTypeID]; ok {
		name = n.Name
	}
	if name == "" {
		name = fmt.Sprintf(locale.Choose(ctx, "舰船 #%d", "Ship #%d"), f.ShipTypeID)
	}
	return LibraryEntry{v.ID, v.CorporationID, v.Name, v.Description, v.Version, v.UpdatedAt, f, name, locale.Choose(ctx, ref.Group, ref.GroupEnglish), exportEFT(f)}, nil
}
func (s *Service) LibraryList(ctx context.Context, user string, corp int64) ([]LibraryEntry, error) {
	if e := s.visibleLibrary(ctx, user, corp); e != nil {
		return nil, e
	}
	rows, e := store.LibraryList(ctx, s.Pool, corp)
	if e != nil {
		return nil, e
	}
	out := []LibraryEntry{}
	ids := []int64{}
	seen := map[int64]bool{}
	for _, r := range rows {
		var f Fit
		if e := json.Unmarshal(r.Fit, &f); e != nil {
			return nil, e
		}
		if !seen[f.ShipTypeID] {
			ids = append(ids, f.ShipTypeID)
			seen[f.ShipTypeID] = true
		}
	}
	names, e := s.Names.TypeNames(ctx, ids)
	if e != nil {
		return nil, e
	}
	for _, r := range rows {
		v, e := s.libraryEntry(ctx, r, names)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, nil
}
func (s *Service) LibraryRead(ctx context.Context, user string, id int64) (LibraryEntry, error) {
	v, e := store.LibraryRead(ctx, s.Pool, id)
	if e != nil {
		return LibraryEntry{}, e
	}
	if e = s.visibleLibrary(ctx, user, v.CorporationID); e != nil {
		return LibraryEntry{}, e
	}
	return s.libraryEntry(ctx, v)
}
func (s *Service) LibraryImport(ctx context.Context, user string, id int64, c LibraryEdit, remove bool) (LibraryEntry, error) {
	admin, e := s.Administrator(ctx, user)
	if e != nil {
		return LibraryEntry{}, e
	}
	if !admin {
		return LibraryEntry{}, pgx.ErrNoRows
	}
	if e = s.visibleLibrary(ctx, user, c.CorporationID); e != nil {
		return LibraryEntry{}, e
	}
	if _, e = uuid(user); e != nil {
		return LibraryEntry{}, e
	}
	if id == 0 {
		if _, e = uuid(c.RequestKey); e != nil {
			return LibraryEntry{}, e
		}
	} else if c.Version < 1 {
		return LibraryEntry{}, ErrInvalid
	}
	var f Fit
	if !remove {
		f, e = parseEFT(c.EFT)
		if e != nil {
			return LibraryEntry{}, e
		}
		c.Description = strings.TrimSpace(c.Description)
		if _, e = gamePayload(f, c.Description); e != nil {
			return LibraryEntry{}, e
		}
	}
	raw, _ := json.Marshal(f)
	v, e := store.LibrarySave(ctx, s.Pool, user, c.RequestKey, store.Library{ID: id, CorporationID: c.CorporationID, Name: f.Name, Description: c.Description, Fit: raw, Version: c.Version}, remove)
	if errors.Is(e, pgx.ErrNoRows) {
		e = ErrConflict
	}
	if e != nil {
		return LibraryEntry{}, e
	}
	return s.libraryEntry(ctx, v)
}
func (s *Service) SaveToGame(ctx context.Context, user string, id, version, char int64, key string) (store.GameSave, error) {
	if _, e := uuid(key); e != nil {
		return store.GameSave{}, e
	}
	v, e := s.LibraryRead(ctx, user, id)
	if e != nil {
		return store.GameSave{}, e
	}
	if v.Version != version {
		return store.GameSave{}, ErrConflict
	}
	rows, e := s.Own(ctx, user)
	if e != nil {
		return store.GameSave{}, e
	}
	owned := false
	for _, r := range rows {
		if r.ID == char {
			owned = true
		}
	}
	if !owned {
		return store.GameSave{}, pgx.ErrNoRows
	}
	payload, e := gamePayload(v.Fit, v.Description)
	if e != nil {
		return store.GameSave{}, e
	}
	if s.GameSave == nil {
		return store.GameSave{}, invalidImport("游戏保存服务暂不可用")
	}
	job, send, e := store.ReserveSend(ctx, s.Pool, user, key, char, id, version)
	if errors.Is(e, pgx.ErrNoRows) {
		e = ErrConflict
	}
	if e != nil || !send {
		return job, e
	}
	result := s.GameSave(ctx, user, char, payload)
	finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if e = store.FinishSend(finish, s.Pool, job.ID, result.State, result.Reason, result.ID); e != nil {
		return store.GameSave{ID: job.ID, State: "unknown", Reason: "confirmation_required"}, nil
	}
	job.State = result.State
	job.Reason = result.Reason
	if result.ID > 0 {
		job.FittingID = &result.ID
	}
	return job, nil
}

// RequiredSkills uses the same static prerequisites as the fitting-to-skill-plan workflow.
func RequiredSkills(f Fit) ([]RequiredSkill, error) { return prerequisites(f) }
func (s *Service) GuardLibraryVersion(ctx context.Context, tx pgx.Tx, corp, id, version int64) error {
	return store.GuardLibraryVersion(ctx, tx, corp, id, version)
}

// Package fittings owns editable site fits. ESI snapshots stay in the eve module.
package fittings

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/fittings/internal/store"
)

var ErrInvalid = errors.New("invalid fitting")
var ErrConflict = errors.New("fitting changed")

type Character struct {
	ID   int64  `json:"id,string"`
	Name string `json:"name"`
}
type Service struct {
	Pool             *pgxpool.Pool
	Own              func(context.Context, string) ([]Character, error)
	Administrator    func(context.Context, string) (bool, error)
	CanReadCharacter func(context.Context, string, int64) (bool, error)
	Snapshot         func(context.Context, int64, string) (json.RawMessage, *time.Time, error)
	Names            interface {
		TypeNames(context.Context, []int64) (map[int64]eve.StaticTypeName, error)
	}
	Search              func(context.Context, string) ([]eve.StaticTypeName, error)
	LibraryCorporations func(context.Context, string) ([]LibraryCorporation, error)
	LibraryCharacters   func(context.Context, string) ([]LibraryCharacter, error)
	GameSave            func(context.Context, string, int64, eve.GameFitting) eve.FittingWriteResult
}
type Item struct {
	TypeID   int64  `json:"type_id,string"`
	Slot     string `json:"slot"`
	Index    int    `json:"index"`
	Quantity int64  `json:"quantity"`
	State    string `json:"state"`
	ChargeID int64  `json:"charge_id,string,omitempty"`
}
type Fit struct {
	Name        string `json:"name"`
	ShipTypeID  int64  `json:"ship_type_id,string"`
	ModeID      int64  `json:"mode_id,string,omitempty"`
	SkillMode   string `json:"skill_mode"`
	CharacterID int64  `json:"character_id,string,omitempty"`
	Items       []Item `json:"items"`
}
type Draft struct {
	ID        int64     `json:"id,string"`
	Version   int64     `json:"version,string"`
	Name      string    `json:"name"`
	UpdatedAt time.Time `json:"updated_at"`
	Fit       *Fit      `json:"fit,omitempty"`
	CanEdit   bool      `json:"can_edit"`
}
type Edit struct {
	Version    int64  `json:"version,string"`
	RequestKey string `json:"request_key"`
	Fit        Fit    `json:"fit"`
}

func uuid(v string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if id.Scan(v) != nil || !id.Valid || id.String() == "00000000-0000-0000-0000-000000000000" {
		return id, ErrInvalid
	}
	return id, nil
}
func validate(f Fit) error {
	if len([]rune(strings.TrimSpace(f.Name))) < 1 || len([]rune(f.Name)) > 80 || f.ShipTypeID <= 0 || f.ShipTypeID > math.MaxInt32 || f.ModeID < 0 || f.ModeID > math.MaxInt32 || len(f.Items) > 512 || f.Items == nil || !slices.Contains([]string{"all5", "all0", "character"}, f.SkillMode) || (f.SkillMode == "character" && f.CharacterID <= 0) {
		return ErrInvalid
	}
	slots := map[string]int{"high": 8, "medium": 8, "low": 8, "rig": 3, "subsystem": 4, "service": 8, "fighter_tube": 5, "fighter_bay": 512, "drone_bay": 512, "cargo": 512}
	seen := map[string]bool{}
	for _, i := range f.Items {
		limit, ok := slots[i.Slot]
		if !ok || i.Index < 0 || i.Index >= limit || i.TypeID <= 0 || i.TypeID > math.MaxInt32 || i.ChargeID < 0 || i.ChargeID > math.MaxInt32 || i.Quantity < 1 || i.Quantity > 1000000 || !slices.Contains([]string{"offline", "online", "active", "overload"}, i.State) {
			return ErrInvalid
		}
		if i.Slot != "cargo" && i.Slot != "drone_bay" && i.Slot != "fighter_bay" {
			k := i.Slot + strconv.Itoa(i.Index)
			if seen[k] || i.Slot != "fighter_tube" && i.Quantity != 1 {
				return ErrInvalid
			}
			seen[k] = true
		}
	}
	return nil
}
func (s *Service) Subject(ctx context.Context, user, member string) (string, error) {
	if member == "" || member == user {
		return user, nil
	}
	if _, err := uuid(member); err != nil {
		return "", err
	}
	ok, err := s.Administrator(ctx, user)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", pgx.ErrNoRows
	}
	chars, err := s.Own(ctx, member)
	if err != nil {
		return "", err
	}
	if len(chars) == 0 {
		return "", pgx.ErrNoRows
	}
	return member, nil
}
func (s *Service) List(ctx context.Context, user, member string, before int64) ([]Draft, string, error) {
	subject, err := s.Subject(ctx, user, member)
	if err != nil {
		return nil, "", err
	}
	actor, err := uuid(subject)
	if err != nil {
		return nil, "", err
	}
	if before == 0 {
		before = math.MaxInt64
	}
	rows, err := store.New(s.Pool).ListDrafts(ctx, store.ListDraftsParams{AccountID: actor, BeforeID: before})
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) > 30 {
		rows = rows[:30]
		next = strconv.FormatInt(rows[29].ID, 10)
	}
	out := []Draft{}
	for _, r := range rows {
		out = append(out, Draft{ID: r.ID, Name: r.Name, Version: r.Version, UpdatedAt: r.UpdatedAt.Time, CanEdit: subject == user})
	}
	return out, next, nil
}
func (s *Service) Read(ctx context.Context, user string, id int64) (Draft, error) {
	r, err := store.New(s.Pool).GetDraft(ctx, id)
	if err != nil {
		return Draft{}, err
	}
	if _, err = s.Subject(ctx, user, r.AccountID.String()); err != nil {
		return Draft{}, err
	}
	var f Fit
	if err = json.Unmarshal(r.Fit, &f); err != nil {
		return Draft{}, err
	}
	return Draft{r.ID, r.Version, r.Name, r.UpdatedAt.Time, &f, r.AccountID.String() == user}, nil
}
func (s *Service) Save(ctx context.Context, user string, id int64, c Edit) (Draft, error) {
	if err := validate(c.Fit); err != nil {
		return Draft{}, err
	}
	actor, err := uuid(user)
	if err != nil {
		return Draft{}, err
	}
	if c.Fit.SkillMode == "character" {
		ok, err := s.CanReadCharacter(ctx, user, c.Fit.CharacterID)
		if err != nil {
			return Draft{}, err
		}
		if !ok {
			return Draft{}, pgx.ErrNoRows
		}
	}
	data, _ := json.Marshal(c.Fit)
	q := store.New(s.Pool)
	var r store.FittingsDraft
	if id == 0 {
		key, e := uuid(c.RequestKey)
		if e != nil {
			return Draft{}, e
		}
		r, err = q.CreateDraft(ctx, store.CreateDraftParams{AccountID: actor, Name: c.Fit.Name, Fit: data, RequestKey: key})
		if err == nil {
			var saved Fit
			if json.Unmarshal(r.Fit, &saved) != nil {
				return Draft{}, ErrConflict
			}
			canonical, _ := json.Marshal(saved)
			if string(canonical) != string(data) {
				return Draft{}, ErrConflict
			}
		}
	} else {
		if c.Version < 1 {
			return Draft{}, ErrInvalid
		}
		r, err = q.UpdateDraft(ctx, store.UpdateDraftParams{ID: id, AccountID: actor, Name: c.Fit.Name, Fit: data, Version: c.Version})
		if errors.Is(err, pgx.ErrNoRows) {
			return Draft{}, ErrConflict
		}
	}
	if err != nil {
		return Draft{}, err
	}
	return Draft{r.ID, r.Version, r.Name, r.UpdatedAt.Time, &c.Fit, true}, nil
}
func (s *Service) Delete(ctx context.Context, user string, id, version int64) error {
	actor, err := uuid(user)
	if err != nil {
		return err
	}
	if version < 1 {
		return ErrInvalid
	}
	n, err := store.New(s.Pool).DeleteDraft(ctx, store.DeleteDraftParams{ID: id, AccountID: actor, Version: version})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrConflict
	}
	return nil
}

package skills

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/skills/internal/store"
	"glorynavy.local/seat/internal/platform/locale"
	"sort"
	"strconv"
	"strings"
	"time"
)

var ErrInvalid = errors.New("invalid skills request")
var ErrConflict = errors.New("skills plan conflict")

type Character struct {
	ID            int64  `json:"id,string"`
	Name          string `json:"name"`
	CorporationID int64  `json:"corporation_id,string"`
}
type Corporation struct {
	ID        int64  `json:"id,string"`
	Name      string `json:"name"`
	CanManage bool   `json:"can_manage"`
}
type Service struct {
	Pool         *pgxpool.Pool
	Characters   func(context.Context, string, string) ([]Character, error)
	CanRead      func(context.Context, string, int64) (bool, error)
	Corporations func(context.Context, string) ([]Corporation, error)
	Manage       func(context.Context, string, int64) (bool, error)
	Members      func(context.Context, string, int64) ([]Character, error)
	Snapshot     func(context.Context, int64, string) (eve.SkillResource, error)
	Names        interface {
		TypeNames(context.Context, []int64) (map[int64]eve.StaticTypeName, error)
	}
}

func (s *Service) Types(ctx context.Context) ([]SkillType, error) {
	ids := []int64{}
	for id := range catalog {
		ids = append(ids, id)
	}
	names, err := s.Names.TypeNames(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := []SkillType{}
	for _, id := range ids {
		v := catalog[id]
		v.Name = locale.Choose(ctx, v.Name, v.English)
		v.Group = locale.Choose(ctx, v.Group, v.GroupEnglish)
		if n, ok := names[id]; ok && n.Name != "" {
			v.Name = n.Name
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
func (s *Service) snapshot(ctx context.Context, id int64) (Snapshot, error) {
	out, err := s.observations(ctx, id)
	if err != nil {
		return out, err
	}
	return s.nameSnapshot(ctx, out)
}
func (s *Service) observations(ctx context.Context, id int64) (Snapshot, error) {
	sk, err := s.Snapshot(ctx, id, "skills")
	if err != nil {
		return Snapshot{}, err
	}
	q, err := s.Snapshot(ctx, id, "skillqueue")
	if err != nil {
		return Snapshot{}, err
	}
	out, err := combine(sk, q, time.Now())
	if err != nil {
		return out, err
	}
	return out, nil
}
func (s *Service) nameSnapshot(ctx context.Context, out Snapshot) (Snapshot, error) {
	ids := []int64{}
	for _, v := range out.Skills {
		ids = append(ids, v.ID)
	}
	for _, v := range out.Queue {
		ids = append(ids, v.ID)
	}
	names, err := s.Names.TypeNames(ctx, ids)
	if err != nil {
		return out, err
	}
	name := func(id int64) string {
		if n, ok := names[id]; ok && n.Name != "" {
			return n.Name
		}
		if t, ok := catalog[id]; ok {
			return locale.Choose(ctx, t.Name, t.English)
		}
		return fmt.Sprintf(locale.Choose(ctx, "技能 #%d", "Skill #%d"), id)
	}
	for i := range out.Skills {
		v := &out.Skills[i]
		v.Name = name(v.ID)
		v.Group = locale.Choose(ctx, catalog[v.ID].Group, catalog[v.ID].GroupEnglish)
		if v.Group == "" {
			v.Group = locale.Choose(ctx, "其他", "Other")
		}
	}
	for i := range out.Queue {
		out.Queue[i].Name = name(out.Queue[i].ID)
	}
	return out, nil
}
func (s *Service) Read(ctx context.Context, user string, id int64) (Snapshot, error) {
	ok, err := s.CanRead(ctx, user, id)
	if err != nil {
		return Snapshot{}, err
	}
	if !ok {
		return Snapshot{}, pgx.ErrNoRows
	}
	return s.snapshot(ctx, id)
}
func (s *Service) allowed(ctx context.Context, user string, corp int64, write bool) error {
	if write {
		ok, err := s.Manage(ctx, user, corp)
		if err != nil {
			return err
		}
		if !ok {
			return pgx.ErrNoRows
		}
		return nil
	}
	rows, err := s.Corporations(ctx, user)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if r.ID == corp {
			return nil
		}
	}
	return pgx.ErrNoRows
}
func (s *Service) Plans(ctx context.Context, user string, corp int64) ([]store.Plan, error) {
	if err := s.allowed(ctx, user, corp, false); err != nil {
		return nil, err
	}
	return store.List(ctx, s.Pool, corp)
}

type Edit struct {
	CorporationID int64         `json:"corporation_id,string"`
	Name          string        `json:"name"`
	Requirements  []Requirement `json:"requirements"`
	Version       int64         `json:"version,string"`
	RequestKey    string        `json:"request_key"`
}

func (s *Service) Save(ctx context.Context, user string, id int64, c Edit, remove bool) (store.Plan, error) {
	if id > 0 {
		old, err := store.Read(ctx, s.Pool, id)
		if err != nil {
			return store.Plan{}, err
		}
		if c.CorporationID != old.CorporationID {
			return store.Plan{}, ErrInvalid
		}
	}
	if err := s.allowed(ctx, user, c.CorporationID, true); err != nil {
		return store.Plan{}, err
	}
	c.Name = strings.TrimSpace(c.Name)
	if !remove {
		if len([]rune(c.Name)) < 1 || len([]rune(c.Name)) > 80 || len(c.Requirements) < 1 || len(c.Requirements) > 200 {
			return store.Plan{}, ErrInvalid
		}
		seen := map[int64]bool{}
		for _, r := range c.Requirements {
			if _, ok := catalog[r.ID]; !ok || seen[r.ID] || r.Level < 1 || r.Level > 5 {
				return store.Plan{}, ErrInvalid
			}
			seen[r.ID] = true
		}
		sort.Slice(c.Requirements, func(i, j int) bool { return c.Requirements[i].ID < c.Requirements[j].ID })
	}
	if id == 0 && !validUUID(c.RequestKey) {
		return store.Plan{}, ErrInvalid
	}
	if id > 0 && c.Version < 1 {
		return store.Plan{}, ErrInvalid
	}
	raw, _ := json.Marshal(c.Requirements)
	p, err := store.Change(ctx, s.Pool, user, c.RequestKey, store.Plan{ID: id, CorporationID: c.CorporationID, Name: c.Name, Requirements: raw, Version: c.Version}, remove)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrConflict
	}
	return p, err
}
func validUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

type Result struct {
	RemainingSP *int64     `json:"remaining_sp"`
	Character   Character  `json:"character"`
	State       string     `json:"state"`
	Met         int        `json:"met"`
	Total       int        `json:"total"`
	Checks      []Check    `json:"checks"`
	ObservedAt  *time.Time `json:"observed_at"`
}

type Report struct {
	Items      []Result `json:"items"`
	NextCursor string   `json:"next_cursor"`
}

func (s *Service) CheckPlan(ctx context.Context, user string, id int64, member string, all bool, after int64) (*Report, error) {
	p, err := store.Read(ctx, s.Pool, id)
	if err != nil {
		return nil, err
	}
	var chars []Character
	if all {
		if err = s.allowed(ctx, user, p.CorporationID, true); err != nil {
			return nil, err
		}
		chars, err = s.Members(ctx, user, p.CorporationID)
	} else {
		chars, err = s.Characters(ctx, user, member)
	}
	if err != nil {
		return nil, err
	}
	var req []Requirement
	if err = json.Unmarshal(p.Requirements, &req); err != nil {
		return nil, err
	}
	// A member check is scoped to the subject's current corporation, including admin read mode.
	eligible := []Character{}
	for _, c := range chars {
		if c.CorporationID == p.CorporationID {
			eligible = append(eligible, c)
		}
	}
	if !all && len(eligible) == 0 {
		return nil, pgx.ErrNoRows
	}
	sort.Slice(eligible, func(i, j int) bool { return eligible[i].ID < eligible[j].ID })
	page := []Character{}
	for _, c := range eligible {
		if c.ID > after {
			page = append(page, c)
		}
	}
	next := ""
	if len(page) > 50 {
		page = page[:50]
		next = strconv.FormatInt(page[49].ID, 10)
	}
	ids := []int64{}
	for _, r := range req {
		ids = append(ids, r.ID)
	}
	names, err := s.Names.TypeNames(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := []Result{}
	for _, c := range page {
		snap, e := s.observations(ctx, c.ID)
		if e != nil {
			return nil, e
		}
		checks := evaluate(req, snap)
		r := Result{Character: c, State: "met", Total: len(checks), Checks: checks, ObservedAt: snap.SkillsMeta.ObservedAt}
		r.RemainingSP = remainingTotal(checks)
		for i := range r.Checks {
			v := &r.Checks[i]
			v.Name = locale.Choose(ctx, catalog[v.ID].Name, catalog[v.ID].English)
			if n, ok := names[v.ID]; ok {
				v.Name = n.Name
			}
			switch v.State {
			case "met":
				r.Met++
			case "unknown":
				r.State = "unknown"
			case "missing":
				if r.State != "unknown" {
					r.State = "missing"
				}
			}
		}
		out = append(out, r)
	}
	return &Report{Items: out, NextCursor: next}, nil
}
func number(v string) (int64, error) {
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 || strconv.FormatInt(n, 10) != v {
		return 0, ErrInvalid
	}
	return n, nil
}

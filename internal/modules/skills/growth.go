package skills

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/skills/internal/store"
	"sort"
)

// CheckRequirements checks one owned character, including characters beyond report pagination.
// Explicit plan requirements augment the current fitting's prerequisites by maximum level.
func (s *Service) CheckRequirements(ctx context.Context, user string, char, corp, plan int64, base []Requirement) (Result, int64, error) {
	chars, err := s.Characters(ctx, user, "")
	if err != nil {
		return Result{}, 0, err
	}
	var subject Character
	for _, c := range chars {
		if c.ID == char && c.CorporationID == corp {
			subject = c
		}
	}
	if subject.ID == 0 {
		return Result{}, 0, pgx.ErrNoRows
	}
	version := int64(0)
	if plan > 0 {
		p, e := store.Read(ctx, s.Pool, plan)
		if e != nil {
			return Result{}, 0, e
		}
		if p.CorporationID != corp {
			return Result{}, 0, pgx.ErrNoRows
		}
		var req []Requirement
		if e = json.Unmarshal(p.Requirements, &req); e != nil {
			return Result{}, 0, e
		}
		base = append(append([]Requirement{}, base...), req...)
		version = p.Version
	}
	levels := map[int64]int{}
	for _, r := range base {
		if r.Level < 1 || r.Level > 5 || catalog[r.ID].ID == "" {
			return Result{}, 0, ErrInvalid
		}
		levels[r.ID] = max(levels[r.ID], r.Level)
	}
	req := []Requirement{}
	for id, l := range levels {
		req = append(req, Requirement{ID: id, Level: l})
	}
	sort.Slice(req, func(i, j int) bool { return req[i].ID < req[j].ID })
	snap, err := s.observations(ctx, char)
	if err != nil {
		return Result{}, 0, err
	}
	checks := evaluate(req, snap)
	result := Result{Character: subject, State: "met", Total: len(checks), Checks: checks, ObservedAt: snap.SkillsMeta.ObservedAt, RemainingSP: remainingTotal(checks)}
	if len(checks) == 0 {
		result.State = "unknown"
	}
	for _, c := range checks {
		switch c.State {
		case "met":
			result.Met++
		case "unknown":
			result.State = "unknown"
		case "missing":
			if result.State != "unknown" {
				result.State = "missing"
			}
		}
	}
	return result, version, nil
}

// GuardPlanVersion keeps a checked plan unchanged until the calling transaction commits.
func (s *Service) GuardPlanVersion(ctx context.Context, tx pgx.Tx, corp, id, version int64) error {
	return store.GuardVersion(ctx, tx, corp, id, version)
}

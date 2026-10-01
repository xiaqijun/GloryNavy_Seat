package eve

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
	"time"
)

const SkillQueueReadScope = "esi-skills.read_skillqueue.v1"

type SkillQueueEntry struct {
	ID           int64      `json:"skill_id"`
	Position     int        `json:"queue_position"`
	Level        int        `json:"finished_level"`
	Start        *time.Time `json:"start_date,omitempty"`
	Finish       *time.Time `json:"finish_date,omitempty"`
	StartSP      *int64     `json:"training_start_sp,omitempty"`
	LevelStartSP *int64     `json:"level_start_sp,omitempty"`
	EndSP        *int64     `json:"level_end_sp,omitempty"`
}

func validSkillQueue(raw []byte) bool {
	var rows []SkillQueueEntry
	if json.Unmarshal(raw, &rows) != nil || rows == nil || len(rows) > 1000 {
		return false
	}
	seen := map[int]bool{}
	for _, r := range rows {
		if r.ID <= 0 || r.Position < 0 || seen[r.Position] || r.Level < 1 || r.Level > 5 {
			return false
		}
		seen[r.Position] = true
		if r.Start != nil && r.Finish != nil && !r.Finish.After(*r.Start) {
			return false
		}
		for _, v := range []*int64{r.StartSP, r.LevelStartSP, r.EndSP} {
			if v != nil && *v < 0 {
				return false
			}
		}
	}
	return true
}

// SkillResource is a cached observation. The host must authorize the current binding first.
type SkillResource struct {
	Payload    json.RawMessage `json:"-"`
	Status     string          `json:"status"`
	Reason     string          `json:"reason"`
	ObservedAt *time.Time      `json:"observed_at"`
	ValidUntil *time.Time      `json:"valid_until"`
}

func (s *AuthorizationService) SkillSnapshot(ctx context.Context, id int64, resource string) (SkillResource, error) {
	out := SkillResource{Status: "pending"}
	if resource != "skills" && resource != "skillqueue" {
		return out, pgx.ErrNoRows
	}
	if s == nil || s.pool == nil {
		return out, nil
	}
	q := store.New(s.pool)
	targets, err := q.ListCharacterSync(ctx, id)
	if err != nil {
		return out, err
	}
	for _, t := range targets {
		if t.Resource == resource {
			out.Reason = t.Reason
			out.ValidUntil = optionalTime(t.ValidUntil)
			if t.State == "blocked" {
				out.Status = "blocked"
				return out, nil
			}
		}
	}
	snap, err := q.ReadFittingSnapshot(ctx, store.ReadFittingSnapshotParams{CharacterID: id, Resource: resource})
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	out.Payload = snap.Payload
	out.ObservedAt = optionalTime(snap.ObservedAt)
	out.Status = "stale"
	if out.ValidUntil != nil && out.ValidUntil.After(time.Now()) {
		out.Status = "ready"
	}
	return out, nil
}

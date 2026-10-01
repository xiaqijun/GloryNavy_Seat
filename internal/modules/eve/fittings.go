package eve

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
)

const FittingsReadScope = "esi-fittings.read_fittings.v1"
const SkillsReadScope = "esi-skills.read_skills.v1"

type fittingArgs struct {
	TargetID   int64  `json:"target_id"`
	Generation int64  `json:"generation"`
	Resource   string `json:"resource"`
}

func (fittingArgs) Kind() string { return "eve.fitting-resource.v1" }

type fittingWorker struct {
	river.WorkerDefaults[fittingArgs]
	s *SyncService
}

func (w *fittingWorker) Work(ctx context.Context, j *river.Job[fittingArgs]) error {
	if !w.s.resourceEnabled(j.Args.Resource) {
		return river.JobSnooze(time.Hour)
	}
	if j.Args.Resource != "fittings" && j.Args.Resource != "skills" && j.Args.Resource != "skillqueue" {
		return fmt.Errorf("invalid fitting resource")
	}
	return w.s.work(ctx, syncArgs{j.Args.TargetID, j.Args.Generation}, j.ID, j.Args.Resource)
}
func (s *SyncService) SetFittingsEnabled(v bool) { s.fittingsEnabled = v }
func (s *SyncService) SetSkillsEnabled(v bool)   { s.skillsEnabled = v }
func (s *SyncService) resourceEnabled(resource string) bool {
	switch resource {
	case "fittings":
		return s.fittingsEnabled
	case "skills":
		return s.fittingsEnabled || s.skillsEnabled
	case "skillqueue":
		return s.skillsEnabled
	}
	return false
}

type fittingObservation struct {
	payload []byte
	at      time.Time
}
type SavedFittingItem struct {
	Flag     string `json:"flag"`
	Quantity int64  `json:"quantity"`
	TypeID   int64  `json:"type_id"`
}
type SavedFitting struct {
	ID          int64              `json:"fitting_id"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	ShipTypeID  int64              `json:"ship_type_id"`
	Items       []SavedFittingItem `json:"items"`
}
type TrainedSkills struct {
	Skills []struct {
		ID      int64 `json:"skill_id"`
		Active  int   `json:"active_skill_level"`
		Trained int   `json:"trained_skill_level"`
	} `json:"skills"`
}

func (s *SyncService) collectFittingResource(ctx context.Context, t store.EveSyncTarget, c store.EveCredential) (syncResult, error) {
	var out syncResult
	var raw json.RawMessage
	scope := FittingsReadScope
	if t.Resource == "skills" {
		scope = SkillsReadScope
	}
	if t.Resource == "skillqueue" {
		scope = SkillQueueReadScope
	}
	r, err := s.auth.esi.Request(ctx, ESIRequest{Method: "GET", Path: fmt.Sprintf("/characters/%d/%s/", c.CharacterID, t.Resource), CharacterID: c.CharacterID, Generation: c.GrantGeneration, Scopes: []string{scope}}, &raw)
	if err != nil {
		return out, err
	}
	invalid := func() (syncResult, error) {
		return out, syncFault{Reason: "invalid_response", Status: 200, Temporary: true}
	}
	if t.Resource == "fittings" {
		var fits []SavedFitting
		if json.Unmarshal(raw, &fits) != nil || fits == nil || len(fits) > 1000 {
			return invalid()
		}
		seen := map[int64]bool{}
		for _, f := range fits {
			if f.ID <= 0 || f.ShipTypeID <= 0 || f.Name == "" || len(f.Items) > 512 || seen[f.ID] {
				return invalid()
			}
			seen[f.ID] = true
			for _, i := range f.Items {
				if i.TypeID <= 0 || i.Quantity <= 0 || i.Flag == "" {
					return invalid()
				}
			}
		}
	} else if t.Resource == "skillqueue" {
		if !validSkillQueue(raw) {
			return invalid()
		}
	} else {
		var skills TrainedSkills
		if json.Unmarshal(raw, &skills) != nil || skills.Skills == nil || len(skills.Skills) > 2000 {
			return invalid()
		}
		seen := map[int64]bool{}
		for _, k := range skills.Skills {
			if k.ID <= 0 || k.Active < 0 || k.Active > 5 || k.Trained < 0 || k.Trained > 5 || seen[k.ID] {
				return invalid()
			}
			seen[k.ID] = true
		}
	}
	if r.ValidatedAt.IsZero() {
		return invalid()
	}
	out.fitting = &fittingObservation{raw, r.ValidatedAt}
	out.next = r.ExpiresAt
	out.content = r.ContentUpdatedAt
	if out.next.Before(time.Now().Add(time.Second)) {
		out.next = time.Now().Add(time.Minute)
	}
	return out, nil
}

// Caller checks current active identity binding and subject ownership/admin access.
func (s *AuthorizationService) FittingSnapshot(ctx context.Context, id int64, resource string) (json.RawMessage, *time.Time, error) {
	if resource != "fittings" && resource != "skills" {
		return nil, nil, pgx.ErrNoRows
	}
	r, err := store.New(s.pool).ReadFittingSnapshot(ctx, store.ReadFittingSnapshotParams{CharacterID: id, Resource: resource})
	if err != nil {
		return nil, nil, err
	}
	return r.Payload, &r.ObservedAt.Time, nil
}

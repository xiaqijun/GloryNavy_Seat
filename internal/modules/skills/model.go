package skills

import (
	_ "embed"
	"encoding/json"
	"glorynavy.local/seat/internal/modules/eve"
	"math"
	"sort"
	"strconv"
	"time"
)

//go:embed catalog.json
var catalogJSON []byte

type SkillType struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	English      string  `json:"english"`
	GroupID      string  `json:"group_id"`
	Group        string  `json:"group"`
	GroupEnglish string  `json:"group_english"`
	Rank         float64 `json:"rank"`
}

var catalogBuild string
var catalog = func() map[int64]SkillType {
	var v struct {
		Build  int64       `json:"build"`
		Skills []SkillType `json:"skills"`
	}
	if err := json.Unmarshal(catalogJSON, &v); err != nil {
		panic(err)
	}
	catalogBuild = strconv.FormatInt(v.Build, 10)
	out := map[int64]SkillType{}
	for _, s := range v.Skills {
		id, _ := strconv.ParseInt(s.ID, 10, 64)
		out[id] = s
	}
	return out
}()

type Requirement struct {
	ID    int64 `json:"skill_id,string"`
	Level int   `json:"level"`
}
type Skill struct {
	ID           int64  `json:"id,string"`
	Name         string `json:"name"`
	Group        string `json:"group"`
	Trained      int    `json:"trained"`
	Active       *int   `json:"active"`
	Points       int64  `json:"points"`
	QueueApplied bool   `json:"queue_applied"`
}
type QueueItem struct {
	ID       int64      `json:"id,string"`
	Name     string     `json:"name"`
	Position int        `json:"position"`
	Level    int        `json:"level"`
	Start    *time.Time `json:"start"`
	Finish   *time.Time `json:"finish"`
	State    string     `json:"state"`
}
type Snapshot struct {
	SkillsMeta    eve.SkillResource `json:"skills_meta"`
	QueueMeta     eve.SkillResource `json:"queue_meta"`
	Skills        []Skill           `json:"skills"`
	Queue         []QueueItem       `json:"queue"`
	TotalSP       *int64            `json:"total_sp"`
	UnallocatedSP *int64            `json:"unallocated_sp"`
	CalculatedAt  time.Time         `json:"calculated_at"`
	uncertain     map[int64]bool
}

// Completed entries are applied only through the queue's observation time, not the wall clock.
// A stale queue cannot prove that a user has not paused/changed a future training item.
func combine(sk, queue eve.SkillResource, now time.Time) (Snapshot, error) {
	out := Snapshot{SkillsMeta: sk, QueueMeta: queue, Skills: []Skill{}, Queue: []QueueItem{}, CalculatedAt: now}
	out.uncertain = map[int64]bool{}
	var raw struct {
		Skills []struct {
			ID      int64 `json:"skill_id"`
			Trained int   `json:"trained_skill_level"`
			Active  int   `json:"active_skill_level"`
			Points  int64 `json:"skillpoints_in_skill"`
		} `json:"skills"`
		Total       *int64 `json:"total_sp"`
		Unallocated *int64 `json:"unallocated_sp"`
	}
	byID := map[int64]Skill{}
	if len(sk.Payload) > 0 {
		if err := json.Unmarshal(sk.Payload, &raw); err != nil {
			return out, err
		}
		out.TotalSP = raw.Total
		out.UnallocatedSP = raw.Unallocated
		for _, s := range raw.Skills {
			active := s.Active
			byID[s.ID] = Skill{ID: s.ID, Trained: s.Trained, Active: &active, Points: s.Points}
		}
	}
	if len(queue.Payload) > 0 {
		var rows []eve.SkillQueueEntry
		if err := json.Unmarshal(queue.Payload, &rows); err != nil {
			return out, err
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].Position < rows[j].Position })
		for _, q := range rows {
			state := "paused"
			if q.Start != nil && q.Finish != nil {
				state = "scheduled"
				if !q.Start.After(now) {
					state = "training"
				}
				if !q.Finish.After(now) {
					state = "awaiting_sync"
				}
			}
			if q.Finish != nil && queue.ObservedAt != nil && !q.Finish.After(*queue.ObservedAt) {
				state = "completed"
				if sk.ObservedAt != nil && queue.ObservedAt.Before(*sk.ObservedAt) && q.Level > byID[q.ID].Trained {
					out.uncertain[q.ID] = true
				}
				if len(sk.Payload) > 0 && (sk.ObservedAt == nil || !queue.ObservedAt.Before(*sk.ObservedAt)) {
					v := byID[q.ID]
					v.ID = q.ID
					if q.Level > v.Trained {
						v.Trained = q.Level
						v.QueueApplied = true
						if q.EndSP != nil && *q.EndSP > v.Points {
							if out.TotalSP != nil {
								*out.TotalSP += *q.EndSP - v.Points
							}
							v.Points = *q.EndSP
						}
						byID[q.ID] = v
					}
				}
			}
			out.Queue = append(out.Queue, QueueItem{ID: q.ID, Position: q.Position, Level: q.Level, Start: q.Start, Finish: q.Finish, State: state})
		}
	}
	for _, v := range byID {
		out.Skills = append(out.Skills, v)
	}
	sort.Slice(out.Skills, func(i, j int) bool { return out.Skills[i].ID < out.Skills[j].ID })
	return out, nil
}

type Check struct {
	ID          int64  `json:"skill_id,string"`
	Name        string `json:"name"`
	Required    int    `json:"required"`
	Trained     *int   `json:"trained"`
	State       string `json:"state"`
	RemainingSP *int64 `json:"remaining_sp"`
}

// Whole points needed to reach a level: round only after applying the SDE rank.
func levelPoints(id int64, level int) (int64, bool) {
	rank := catalog[id].Rank
	if rank <= 0 || rank > 1000 || math.IsNaN(rank) || math.IsInf(rank, 0) || level < 0 || level > 5 {
		return 0, false
	}
	if level == 0 {
		return 0, true
	}
	return int64(math.Ceil(250 * rank * math.Exp2(2.5*float64(level-1)))), true
}

func remainingPoints(id int64, required int, skill Skill) *int64 {
	if skill.Trained >= required {
		zero := int64(0)
		return &zero
	}
	target, ok := levelPoints(id, required)
	minimum, known := levelPoints(id, skill.Trained)
	if !ok || !known || skill.Points < 0 {
		return nil
	}
	// Confirmed completed queues may omit end SP; their trained level still proves
	// the minimum points. Partial training comes from observed SP, never a clock estimate.
	current := max(minimum, skill.Points)
	if current >= target {
		return nil
	} // contradictory level/SP observation
	remaining := target - current
	return &remaining
}

func remainingTotal(checks []Check) *int64 {
	total := int64(0)
	for _, c := range checks {
		if c.RemainingSP == nil {
			return nil
		}
		total += *c.RemainingSP
	}
	return &total
}

func evaluate(reqs []Requirement, s Snapshot) []Check {
	// Expiry schedules refresh; it does not erase an authorized observation.
	// Both resources must exist, and conflicting observations remain unknown.
	readable := func(r eve.SkillResource) bool {
		return (r.Status == "ready" || r.Status == "stale") && r.ObservedAt != nil && len(r.Payload) > 0
	}
	canCheck := readable(s.SkillsMeta) && readable(s.QueueMeta)
	byID := map[int64]Skill{}
	for _, k := range s.Skills {
		byID[k.ID] = k
	}
	out := []Check{}
	for _, r := range reqs {
		c := Check{ID: r.ID, Required: r.Level, State: "unknown"}
		if canCheck && !s.uncertain[r.ID] {
			n := byID[r.ID].Trained
			c.Trained = &n
			c.State = "missing"
			c.RemainingSP = remainingPoints(r.ID, r.Level, byID[r.ID])
			if n >= r.Level {
				c.State = "met"
			}
		}
		out = append(out, c)
	}
	return out
}

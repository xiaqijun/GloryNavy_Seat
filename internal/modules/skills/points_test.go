package skills

import (
	"encoding/json"
	"glorynavy.local/seat/internal/modules/eve"
	"testing"
	"time"
)

func TestSkillPointThresholds(t *testing.T) {
	for _, c := range []struct {
		id    int64
		level int
		want  int64
	}{{3300, 1, 250}, {3300, 2, 1415}, {3300, 3, 8000}, {3300, 4, 45255}, {3300, 5, 256000}, {20494, 4, 90510}, {3335, 2, 7072}, {16069, 2, 2829}, {40572, 4, 543059}} {
		got, ok := levelPoints(c.id, c.level)
		if !ok || got != c.want {
			t.Fatalf("skill %d level %d: %d, want %d", c.id, c.level, got, c.want)
		}
	}
	for id := range catalog {
		if _, ok := levelPoints(id, 5); !ok {
			t.Fatalf("SDE skill rank missing %d", id)
		}
	}
}

func TestSkillPointGapUsesObservedPartialTraining(t *testing.T) {
	at := time.Now()
	meta := eve.SkillResource{Status: "stale", ObservedAt: &at, Payload: json.RawMessage(`[]`)}
	s := Snapshot{SkillsMeta: meta, QueueMeta: meta, Skills: []Skill{{ID: 3300, Trained: 3, Points: 20000}}}
	checks := evaluate([]Requirement{{3300, 4}, {3301, 1}}, s)
	if checks[0].RemainingSP == nil || *checks[0].RemainingSP != 25255 || checks[1].RemainingSP == nil || *checks[1].RemainingSP != 250 {
		t.Fatal(checks)
	}
	if total := remainingTotal(checks); total == nil || *total != 25505 {
		t.Fatal("bad total", total)
	}
	free := int64(1000000)
	s.UnallocatedSP = &free
	if got := evaluate([]Requirement{{3300, 4}}, s)[0].RemainingSP; got == nil || *got != 25255 {
		t.Fatal("unallocated SP must not be silently spent")
	}
	s.Skills[0].Trained = 5
	if got := evaluate([]Requirement{{3300, 4}}, s)[0].RemainingSP; got == nil || *got != 0 {
		t.Fatal("met requirement must have zero deficit")
	}
	s.Skills[0] = Skill{ID: 3300, Trained: 3, Points: 45255}
	if got := evaluate([]Requirement{{3300, 4}}, s)[0].RemainingSP; got != nil {
		t.Fatal("conflicting level and SP must not imply zero deficit")
	}
	s.uncertain = map[int64]bool{3300: true}
	checks = evaluate([]Requirement{{3300, 4}, {3301, 1}}, s)
	if checks[0].RemainingSP != nil || remainingTotal(checks) != nil || checks[1].RemainingSP == nil {
		t.Fatal("partial unknown must not present partial sum as full total")
	}
	s.uncertain = nil
	s.QueueMeta.Status = "blocked"
	if evaluate([]Requirement{{3301, 1}}, s)[0].RemainingSP != nil {
		t.Fatal("blocked data must remain unknown")
	}
	s.QueueMeta.Status = "ready"
	if evaluate([]Requirement{{99999999, 1}}, s)[0].RemainingSP != nil {
		t.Fatal("unknown multiplier must remain unknown")
	}
}

func TestSkillPointGapAfterConfirmedQueueWithoutEndSP(t *testing.T) {
	at := time.Date(2026, 9, 17, 1, 0, 0, 0, time.UTC)
	sk := eve.SkillResource{Status: "stale", ObservedAt: &at, Payload: json.RawMessage(`{"skills":[{"skill_id":3300,"trained_skill_level":3,"active_skill_level":3,"skillpoints_in_skill":8000}]}`)}
	q := eve.SkillResource{Status: "stale", ObservedAt: &at, Payload: json.RawMessage(`[{"skill_id":3300,"queue_position":0,"finished_level":4,"finish_date":"2026-09-17T00:00:00Z"},{"skill_id":3300,"queue_position":1,"finished_level":5,"finish_date":"2026-09-17T02:00:00Z","level_end_sp":256000}]`)}
	s, err := combine(sk, q, at.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	checks := evaluate([]Requirement{{3300, 5}}, s)
	if checks[0].RemainingSP == nil || *checks[0].RemainingSP != 210745 {
		t.Fatal("completed level floor/future queue reconciliation incorrect", checks)
	}
}

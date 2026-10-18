package skills

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"glorynavy.local/seat/internal/modules/eve"
)

func TestOfflineCompletedQueueIndependentOfFetchOrder(t *testing.T) {
	at := time.Date(2026, 10, 18, 5, 0, 0, 0, time.UTC)
	for _, order := range []time.Duration{-time.Minute, 0, time.Minute} {
		for _, status := range []string{"ready", "stale"} {
			queueAt := at.Add(order)
			sk := eve.SkillResource{Status: status, ObservedAt: &at, Payload: json.RawMessage(`{"skills":[{"skill_id":3300,"trained_skill_level":3,"active_skill_level":2,"skillpoints_in_skill":8000}],"total_sp":8000}`)}
			q := eve.SkillResource{Status: status, ObservedAt: &queueAt, Payload: json.RawMessage(`[{"skill_id":3300,"queue_position":0,"finished_level":4,"finish_date":"2026-10-18T04:00:00Z","level_end_sp":45255},{"skill_id":3300,"queue_position":1,"finished_level":5,"finish_date":"2026-10-18T06:00:00Z","level_end_sp":256000}]`)}
			s, err := combine(sk, q, at.Add(48*time.Hour))
			if err != nil {
				t.Fatal(err)
			}
			checks := evaluate([]Requirement{{3300, 4}, {3300, 5}}, s)
			if checks[0].State != "met" || checks[1].State != "missing" || *checks[1].Trained != 4 || *checks[1].RemainingSP != 210745 {
				t.Fatal("completion depends on fetch order or wall clock", order, status, checks)
			}
			if *s.Skills[0].Active != 2 || *s.TotalSP != 45255 || !s.Skills[0].QueueApplied {
				t.Fatal("active levels or SP changed incorrectly", s)
			}
			// Growth eligibility uses the same observed completion, including when
			// the character's skills endpoint was refreshed after its queue.
			svc := Service{Characters: func(context.Context, string, string) ([]Character, error) {
				return []Character{{ID: 1, CorporationID: 10}}, nil
			}, Snapshot: func(_ context.Context, _ int64, resource string) (eve.SkillResource, error) {
				if resource == "skills" {
					return sk, nil
				}
				return q, nil
			}}
			result, _, err := svc.CheckRequirements(context.Background(), "owner", 1, 10, 0, []Requirement{{3300, 4}})
			if err != nil || result.State != "met" || result.RemainingSP == nil || *result.RemainingSP != 0 {
				t.Fatal("growth check disagrees", result, err)
			}
		}
	}
}

func TestCompletedQueueNeverDowngradesOrUsesUnreadableObservations(t *testing.T) {
	at := time.Now()
	sk := eve.SkillResource{Status: "ready", ObservedAt: &at, Payload: json.RawMessage(`{"skills":[{"skill_id":3300,"trained_skill_level":5,"active_skill_level":3,"skillpoints_in_skill":256000}],"total_sp":256000}`)}
	q := eve.SkillResource{Status: "ready", ObservedAt: &at, Payload: json.RawMessage(`[{"skill_id":3300,"queue_position":0,"finished_level":4,"finish_date":"2026-09-01T00:00:00Z","level_end_sp":45255}]`)}
	s, err := combine(sk, q, at)
	if err != nil || s.Skills[0].Trained != 5 || s.Skills[0].Points != 256000 || s.Skills[0].QueueApplied || *s.TotalSP != 256000 {
		t.Fatal("queue downgraded skills or double counted SP", s, err)
	}
	sk.Payload = json.RawMessage(`{"skills":[{"skill_id":3300,"trained_skill_level":3,"active_skill_level":3,"skillpoints_in_skill":8000}],"total_sp":8000}`)
	for _, resource := range []string{"skills", "skillqueue"} {
		for _, state := range []string{"blocked", "pending", "missing_time", "missing_payload"} {
			a, b := sk, q
			meta := &a
			if resource == "skillqueue" {
				meta = &b
			}
			switch state {
			case "missing_time":
				meta.ObservedAt = nil
			case "missing_payload":
				meta.Payload = nil
			default:
				meta.Status = state
			}
			s, err := combine(a, b, at)
			if err != nil || evaluate([]Requirement{{3300, 4}}, s)[0].State != "unknown" {
				t.Fatal(resource, state, s, err)
			}
			for _, skill := range s.Skills {
				if skill.QueueApplied {
					t.Fatal("unreadable resource promoted a skill", resource, state)
				}
			}
		}
	}
}

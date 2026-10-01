package skills

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/testutil"
	"testing"
	"time"
)

func TestSkillsQueueReconciliationAndFreshness(t *testing.T) {
	at := time.Date(2026, 9, 15, 1, 0, 0, 0, time.UTC)
	sk := eve.SkillResource{Status: "ready", ObservedAt: &at, Payload: json.RawMessage(`{"skills":[{"skill_id":3300,"trained_skill_level":3,"active_skill_level":2,"skillpoints_in_skill":8000}],"total_sp":8000}`)}
	q := eve.SkillResource{Status: "ready", ObservedAt: &at, Payload: json.RawMessage(`[{"skill_id":3300,"queue_position":0,"finished_level":4,"finish_date":"2026-09-15T00:00:00Z","level_end_sp":45000},{"skill_id":3300,"queue_position":1,"finished_level":5,"finish_date":"2026-09-15T01:10:00Z","level_end_sp":256000}]`)}
	s, e := combine(sk, q, at.Add(time.Hour))
	if e != nil {
		t.Fatal(e)
	}
	if s.Skills[0].Trained != 4 || *s.Skills[0].Active != 2 || *s.TotalSP != 45000 || s.UnallocatedSP != nil {
		t.Fatalf("wrong merge: %+v", s)
	}
	if s.Queue[1].State != "paused" {
		t.Fatalf("missing start is not active: %+v", s.Queue[1])
	}
	older := at.Add(-time.Minute)
	q.ObservedAt = &older
	mixed, e := combine(sk, q, at)
	if e != nil {
		t.Fatal(e)
	}
	if evaluate([]Requirement{{3300, 4}}, mixed)[0].State != "unknown" {
		t.Fatal("conflicting observation order must not falsely fail")
	}
	q.ObservedAt = &at
	checks := evaluate([]Requirement{{3300, 4}, {3301, 1}}, s)
	if checks[0].State != "met" || checks[1].State != "missing" {
		t.Fatal(checks)
	}
	for _, states := range [][2]string{{"ready", "stale"}, {"stale", "ready"}, {"stale", "stale"}} {
		s.SkillsMeta.Status, s.QueueMeta.Status = states[0], states[1]
		checks = evaluate([]Requirement{{3300, 4}, {3301, 1}, {3300, 5}}, s)
		if checks[0].State != "met" || checks[1].State != "missing" || checks[2].State != "missing" || *checks[2].Trained != 4 {
			t.Fatal("historical observations should check known levels without advancing future training", states, checks)
		}
	}
	mixed.SkillsMeta.Status, mixed.QueueMeta.Status = "stale", "stale"
	if evaluate([]Requirement{{3300, 4}}, mixed)[0].State != "unknown" {
		t.Fatal("historical conflicting observations must remain unknown")
	}
	for _, resource := range []string{"skills", "queue"} {
		for _, invalid := range []string{"pending", "blocked", "unexpected", "no payload", "no observation"} {
			copy := s
			meta := &copy.SkillsMeta
			if resource == "queue" {
				meta = &copy.QueueMeta
			}
			switch invalid {
			case "no payload":
				meta.Payload = nil
			case "no observation":
				meta.ObservedAt = nil
			default:
				meta.Status = invalid
			}
			for _, c := range evaluate([]Requirement{{3300, 4}, {3301, 1}}, copy) {
				if c.State != "unknown" || c.Trained != nil {
					t.Fatal("unreadable observation must remain unknown", resource, invalid, c)
				}
			}
		}
	}
	sk.Payload = nil
	s, e = combine(sk, q, at)
	if e != nil || len(s.Skills) != 0 || s.TotalSP != nil {
		t.Fatal("queue is not a substitute for complete skill list")
	}
}

type namesStub struct{}

func (namesStub) TypeNames(context.Context, []int64) (map[int64]eve.StaticTypeName, error) {
	return map[int64]eve.StaticTypeName{}, nil
}
func TestSkillsPlanPermissionsVersionAuditAndBoundaries(t *testing.T) {
	pool := testutil.Database(t)
	ctx := context.Background()
	user := "11111111-1111-4111-8111-111111111111"
	manage := true
	s := &Service{Pool: pool, Names: namesStub{}, Manage: func(_ context.Context, _ string, corp int64) (bool, error) { return manage && corp == 900, nil }, Corporations: func(context.Context, string) ([]Corporation, error) { return []Corporation{{ID: 900}}, nil }, Characters: func(context.Context, string, string) ([]Character, error) {
		return []Character{{ID: 101, CorporationID: 901}}, nil
	}}
	c := Edit{CorporationID: 900, Name: "基础", Requirements: []Requirement{{3300, 4}}, RequestKey: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"}
	p, e := s.Save(ctx, user, 0, c, false)
	if e != nil {
		t.Fatal(e)
	}
	again, e := s.Save(ctx, user, 0, c, false)
	if e != nil || again.ID != p.ID {
		t.Fatal("create replay", e)
	}
	s.Members = func(context.Context, string, int64) ([]Character, error) {
		out := []Character{}
		for i := int64(1); i <= 51; i++ {
			out = append(out, Character{ID: i, CorporationID: 900})
		}
		return out, nil
	}
	s.Snapshot = func(context.Context, int64, string) (eve.SkillResource, error) {
		return eve.SkillResource{Status: "pending"}, nil
	}
	page, e := s.CheckPlan(ctx, user, p.ID, "", true, 0)
	if e != nil || len(page.Items) != 50 || page.NextCursor != "50" {
		t.Fatal("first page", page, e)
	}
	page, e = s.CheckPlan(ctx, user, p.ID, "", true, 50)
	if e != nil || len(page.Items) != 1 || page.NextCursor != "" {
		t.Fatal("last page", page, e)
	}
	if page.Items[0].State != "unknown" {
		t.Fatal("first sync should remain unknown", page)
	}
	if page.Items[0].RemainingSP != nil {
		t.Fatal("unknown report must not claim zero SP")
	}
	observed := time.Now().Add(-24 * time.Hour).Truncate(time.Microsecond)
	level := 4
	s.Snapshot = func(_ context.Context, _ int64, resource string) (eve.SkillResource, error) {
		payload := json.RawMessage(`[]`)
		if resource == "skills" {
			payload, _ = json.Marshal(map[string]any{"skills": []map[string]any{{"skill_id": 3300, "trained_skill_level": level, "active_skill_level": level, "skillpoints_in_skill": 45000}}})
		}
		return eve.SkillResource{Status: "stale", ObservedAt: &observed, Payload: payload}, nil
	}
	page, e = s.CheckPlan(ctx, user, p.ID, "", true, 50)
	if e != nil || page.Items[0].State != "met" || page.Items[0].Met != 1 || !page.Items[0].ObservedAt.Equal(observed) {
		t.Fatal("report must use historical observations and preserve their timestamp", page, e)
	}
	level = 3
	page, e = s.CheckPlan(ctx, user, p.ID, "", true, 50)
	if e != nil || page.Items[0].State != "missing" || page.Items[0].Met != 0 {
		t.Fatal("report must recompute from replacement observations", page, e)
	}
	if page.Items[0].RemainingSP == nil || *page.Items[0].RemainingSP != 255 {
		t.Fatal("report did not aggregate observed SP deficit", page)
	}
	if _, e = s.Plans(ctx, user, 901); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatal("foreign corp", e)
	}
	if _, e = s.CheckPlan(ctx, user, p.ID, "", false, 0); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatal("foreign character must not check plan", e)
	}
	manage = false
	c.Version = p.Version
	c.Name = "changed"
	if _, e = s.Save(ctx, user, p.ID, c, false); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatal("revoked manager", e)
	}
	manage = true
	c.Requirements = []Requirement{{587, 5}}
	if _, e = s.Save(ctx, user, p.ID, c, false); !errors.Is(e, ErrInvalid) {
		t.Fatal("ship is not a skill", e)
	}
	c.Requirements = []Requirement{{3300, 4}, {3300, 5}}
	if _, e = s.Save(ctx, user, p.ID, c, false); !errors.Is(e, ErrInvalid) {
		t.Fatal("duplicate", e)
	}
	c.Requirements = []Requirement{{3300, 5}}
	p, e = s.Save(ctx, user, p.ID, c, false)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Save(ctx, user, p.ID, c, false); !errors.Is(e, ErrConflict) {
		t.Fatal("stale version", e)
	}
	c.Version = p.Version
	if _, e = s.Save(ctx, user, p.ID, c, true); e != nil {
		t.Fatal(e)
	}
	var count int
	if e = pool.QueryRow(ctx, "SELECT count(*) FROM skills_plan_audit").Scan(&count); e != nil || count != 3 {
		t.Fatal("audit should be create/update/delete once", count, e)
	}
}

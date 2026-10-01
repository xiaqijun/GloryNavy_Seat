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

func TestGrowthRequirementsUseCurrentPlanAndHistoricalSkills(t *testing.T) {
	ctx := context.Background()
	pool := testutil.Database(t)
	at := time.Now().Add(-time.Hour)
	s := &Service{Pool: pool, Characters: func(context.Context, string, string) ([]Character, error) {
		out := []Character{}
		for i := int64(1); i <= 70; i++ {
			out = append(out, Character{ID: i, CorporationID: 900})
		}
		return out, nil
	}}
	unavailable := false
	s.Snapshot = func(_ context.Context, id int64, resource string) (eve.SkillResource, error) {
		if unavailable {
			return eve.SkillResource{Status: "blocked"}, nil
		}
		payload := json.RawMessage(`[]`)
		if resource == "skills" {
			payload = json.RawMessage(`{"skills":[{"skill_id":3300,"trained_skill_level":4,"active_skill_level":4,"skillpoints_in_skill":45255}],"total_sp":45255}`)
		}
		return eve.SkillResource{Status: "stale", ObservedAt: &at, Payload: payload}, nil
	}
	base := []Requirement{{ID: 3300, Level: 3}}
	r, _, err := s.CheckRequirements(ctx, "user", 70, 900, 0, base)
	if err != nil || r.State != "met" {
		t.Fatal(r, err)
	}
	if _, _, err = s.CheckRequirements(ctx, "user", 70, 901, 0, base); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("foreign corp allowed", err)
	}
	var id int64
	// Store a plan through its normal service, then edit its requirement.
	s.Manage = func(context.Context, string, int64) (bool, error) { return true, nil }
	user := "00000000-0000-4000-8000-000000000011"
	if _, err = pool.Exec(ctx, "INSERT INTO identity_users(id)VALUES($1)", user); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, "INSERT INTO skills_plans(corporation_id,name,requirements,created_by,request_key)VALUES(900,'Extra','[{\"skill_id\":\"3300\",\"level\":5}]',$1,'00000000-0000-4000-8000-000000000012')RETURNING id", user).Scan(&id); err != nil {
		t.Fatal(err)
	}
	r, version, err := s.CheckRequirements(ctx, "user", 70, 900, id, base)
	if err != nil || r.State != "missing" || r.RemainingSP == nil || *r.RemainingSP <= 0 {
		t.Fatal(r, err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err = s.GuardPlanVersion(ctx, tx, 900, id, version+1); err == nil {
		t.Fatal("version conflict ignored")
	}
	tx.Rollback(ctx)
	if _, err = pool.Exec(ctx, "UPDATE skills_plans SET requirements='[{\"skill_id\":\"3300\",\"level\":2}]',version=version+1 WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
	r, _, err = s.CheckRequirements(ctx, "user", 70, 900, id, []Requirement{{ID: 3300, Level: 5}})
	if err != nil || r.State != "missing" {
		t.Fatal("plan weakened hull requirement", r, err)
	}
	unavailable = true
	r, _, err = s.CheckRequirements(ctx, "user", 70, 900, 0, base)
	if err != nil || r.State != "unknown" {
		t.Fatal(r, err)
	}
}

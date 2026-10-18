package access

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/testutil"
)

func TestRoleScopeOptionsFollowCurrentManagementAndCorporationScope(t *testing.T) {
	pool := testutil.Database(t)
	ctx := context.Background()
	users := identity.New(pool)
	token, err := users.SignIn(ctx, 123, "Pilot", "owner", "")
	if err != nil {
		t.Fatal(err)
	}
	session, err := users.Session(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	facts := []Fact{{CharacterID: 123, State: "ready", Corporation: Corporation{ID: 10}, SyncedAt: time.Now(), ValidUntil: time.Now().Add(time.Hour)}}
	s := New(pool, func(context.Context, string) ([]Fact, error) { return facts, nil }, func(_ context.Context, id int64) (Corporation, error) {
		switch id {
		case 10, 20:
			return Corporation{ID: id, Name: "Configured", AllianceID: 100}, nil
		case 30:
			return Corporation{ID: id, Name: "Third", AllianceID: 300}, nil
		default:
			return Corporation{}, pgx.ErrNoRows
		}
	})
	configured := []int64{10, 10, 20, 30, 999}
	s.ConfiguredCorporations = func(context.Context) ([]int64, error) { return configured, nil }
	var named []int64
	s.ScopeNames = func(_ context.Context, ids []int64) (map[string]string, error) {
		named = ids
		return map[string]string{"100": "Alliance One", "300": "Alliance Three"}, nil
	}
	if _, err = s.ScopeOptions(ctx, session.UserID); !errors.Is(err, errScopeOptionsDenied) {
		t.Fatal("ordinary account", err)
	}
	role := Role{ID: "00000000-0000-4000-8000-000000000001", Name: "Manager", Grants: []Grant{{Permission: "access.manage"}}}
	if _, err = s.SaveRole(ctx, session.UserID, role); err != nil {
		t.Fatal(err)
	}
	if err = s.Assign(ctx, session.UserID, session.UserID, role.ID, true); err != nil {
		t.Fatal(err)
	}
	out, err := s.ScopeOptions(ctx, session.UserID)
	if err != nil || !reflect.DeepEqual(out.Corporations, []ScopeOption{{10, "Configured"}}) || !reflect.DeepEqual(out.Alliances, []ScopeOption{{100, "Alliance One"}}) {
		t.Fatalf("delegated scope: %+v %v", out, err)
	}
	if err = s.SetAdministrator(ctx, session.UserID, true); err != nil {
		t.Fatal(err)
	}
	out, err = s.ScopeOptions(ctx, session.UserID)
	if err != nil || len(out.Corporations) != 3 || len(out.Alliances) != 2 || !reflect.DeepEqual(named, []int64{100, 300}) {
		t.Fatalf("administrator configured scope/dedup: %+v %v %v", out, named, err)
	}
	configured = []int64{20}
	out, err = s.ScopeOptions(ctx, session.UserID)
	if err != nil || len(out.Corporations) != 1 || out.Corporations[0].ID != 20 {
		t.Fatalf("configuration refresh: %+v %v", out, err)
	}
	if err = s.SetAdministrator(ctx, session.UserID, false); err != nil {
		t.Fatal(err)
	}
	out, err = s.ScopeOptions(ctx, session.UserID)
	if err != nil || len(out.Corporations) != 0 || len(out.Alliances) != 0 {
		t.Fatalf("cross-corporation filter: %+v %v", out, err)
	}
	configured = []int64{10}
	facts[0].ValidUntil = time.Now().Add(-time.Minute)
	out, err = s.ScopeOptions(ctx, session.UserID)
	if err != nil || len(out.Corporations) != 0 {
		t.Fatalf("stale membership: %+v %v", out, err)
	}
	if err = s.Assign(ctx, session.UserID, session.UserID, role.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ScopeOptions(ctx, session.UserID); !errors.Is(err, errScopeOptionsDenied) {
		t.Fatal("revoked management", err)
	}
}

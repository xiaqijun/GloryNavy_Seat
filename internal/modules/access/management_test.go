package access

import (
	"context"
	"errors"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/testutil"
	"math"
	"sync"
	"testing"
)

func TestRoleVersionsPreventOverwriteAndAuditOnlyCommittedChanges(t *testing.T) {
	pool := testutil.Database(t)
	ctx := context.Background()
	s := New(pool, nil, nil)
	role := Role{ID: "01994763-4111-7000-8000-111111111111", Name: "财务", Grants: []Grant{{Permission: "corporation.journal", Corporations: EntityIDs{10}}}}
	created, err := s.SaveRole(ctx, "operator", role)
	if err != nil || created.Version != 1 {
		t.Fatalf("create: %+v %v", created, err)
	}
	if _, err = s.SaveRole(ctx, "operator", role); !errors.Is(err, ErrConflict) {
		t.Fatalf("recreate: %v", err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, name := range []string{"财务一", "财务二"} {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			copy := created
			copy.Name = name
			_, e := s.SaveRole(ctx, "operator", copy)
			results <- e
		}(name)
	}
	wg.Wait()
	close(results)
	success, conflicts := 0, 0
	for e := range results {
		if e == nil {
			success++
		} else if errors.Is(e, ErrConflict) {
			conflicts++
		} else {
			t.Fatal(e)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatalf("concurrent results %d %d", success, conflicts)
	}
	if err = s.DeleteRole(ctx, "operator", role.ID, 1); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale delete: %v", err)
	}
	users := identity.New(pool)
	token, err := users.SignIn(ctx, 123, "Pilot", "owner", "")
	if err != nil {
		t.Fatal(err)
	}
	session, err := users.Session(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Assign(ctx, "operator", session.UserID, role.ID, true); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.Can(ctx, session.UserID, "corporation.journal", Corporation{ID: 10}); err != nil || !ok {
		t.Fatal("assignment missing", err)
	}
	if err = s.DeleteRole(ctx, "operator", role.ID, 2); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.Can(ctx, session.UserID, "corporation.journal", Corporation{ID: 10}); err != nil || ok {
		t.Fatal("deleted grant survives", err)
	}
	audit, next, err := s.Audit(ctx, math.MaxInt64)
	if err != nil || len(audit) != 4 || next != "" {
		t.Fatalf("audit committed changes: %d %q %v", len(audit), next, err)
	}
	if audit[0].Action != "role.deleted" {
		t.Fatal("audit order")
	}
	management := Role{ID: role.ID, Name: "权限管理员", Grants: []Grant{{Permission: "access.manage"}}}
	if _, err = s.SaveRole(ctx, "operator", management); err != nil {
		t.Fatal(err)
	}
	if err = s.Assign(ctx, "operator", session.UserID, role.ID, true); err != nil {
		t.Fatal(err)
	}
	a, err := s.Account(ctx, session.UserID)
	if err != nil || !a.CanManage || a.Administrator {
		t.Fatal("delegated management capability", err)
	}
	if err = s.Assign(ctx, "operator", session.UserID, role.ID, false); err != nil {
		t.Fatal(err)
	}
	a, err = s.Account(ctx, session.UserID)
	if err != nil || a.CanManage {
		t.Fatal("revocation retained capability", err)
	}
}

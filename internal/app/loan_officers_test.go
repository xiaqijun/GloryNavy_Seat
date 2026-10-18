package app

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"testing"
	"time"

	"glorynavy.local/seat/internal/modules/access"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/modules/loan"
)

func TestLoanOfficerDirectoryAndOwnership(t *testing.T) {
	f := newCleanupFixture(t)
	ctx := context.Background()
	token, officer := f.sign(t, 101)
	f.facts(t, 101, 10)
	_, admin := f.sign(t, 201)
	f.facts(t, 201, 10)
	if _, err := f.users.Complete(ctx, 102, "Officer Alt", "owner", token, identity.LoginIntent{Kind: "link", UserID: officer.UserID}, nil); err != nil {
		t.Fatal(err)
	}
	f.facts(t, 102, 10)
	acl := access.New(f.pool, func(context.Context, string) ([]access.Fact, error) {
		return []access.Fact{{CharacterID: 101, State: "ready", Corporation: access.Corporation{ID: 10}, Roles: []string{"Director"}, SyncedAt: time.Now().Add(-time.Minute), ValidUntil: time.Now().Add(time.Hour)}}, nil
	}, nil)
	acl.ConfiguredCorporations = func(context.Context) ([]int64, error) { return []int64{10}, nil }
	if err := acl.SetAdministrator(ctx, admin.UserID, true); err != nil {
		t.Fatal(err)
	}
	s := &loan.Service{Pool: f.pool}
	wireLoanOfficers(s, f.users, acl, eve.ReadAuthorization(f.pool))
	check := func(want int) {
		t.Helper()
		rows, err := s.LoanOfficers(ctx, 10)
		if err != nil || len(rows) != want {
			t.Fatalf("directory: %+v %v want %d", rows, err, want)
		}
	}
	check(0) // Neither administrator nor Director implicitly becomes an officer.
	role, err := acl.SaveRole(ctx, "test", access.Role{ID: "01994763-4111-7000-8000-333333333333", Name: "Loan officer", Grants: []access.Grant{{Permission: "corporation.loan", Corporations: access.EntityIDs{10}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err = acl.Assign(ctx, "test", officer.UserID, role.ID, true); err != nil {
		t.Fatal(err)
	}
	check(2)
	rows, _ := s.LoanOfficers(ctx, 10)
	if rows[0].ID != 101 || !rows[0].IsMain || rows[1].IsMain {
		t.Fatal(rows)
	}
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.LockOfficerCharacters(ctx, tx, []int64{102}); err != nil {
		t.Fatal(err)
	}
	if err = s.GuardLoanOfficer(ctx, tx, 10, 102); err != nil {
		t.Fatal("alt rejected", err)
	}
	// A role revocation cannot race the contract publication transaction.
	blocked, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	err = acl.Assign(blocked, "test", officer.UserID, role.ID, false)
	cancel()
	if err == nil {
		t.Fatal("revoked permission while a contract holds it")
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if err = f.users.ChangeCharacter(ctx, token, 102, false, nil); err != nil {
		t.Fatal(err)
	}
	rows, _ = s.LoanOfficers(ctx, 10)
	if rows[0].ID != 102 || !rows[0].IsMain || rows[1].IsMain {
		t.Fatal("main change", rows)
	}
	// The previous main remains a selectable alt until explicitly unbound.
	if err = s.GuardLoanOfficer(ctx, nil, 10, 101); err != nil {
		t.Fatal(err)
	}
	if err = f.users.ChangeCharacter(ctx, token, 101, true, func(context.Context, pgx.Tx, int64, string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	check(1)
	if err = s.GuardLoanOfficer(ctx, nil, 10, 101); !errors.Is(err, loan.ErrOfficer) {
		t.Fatal("unbound accepted", err)
	}
	if err = acl.Assign(ctx, "test", officer.UserID, role.ID, false); err != nil {
		t.Fatal(err)
	}
	check(0)
	if err = s.GuardLoanOfficer(ctx, nil, 10, 102); !errors.Is(err, loan.ErrOfficer) {
		t.Fatal("revoked accepted", err)
	}
	if err = acl.Assign(ctx, "test", officer.UserID, role.ID, true); err != nil {
		t.Fatal(err)
	}
	role.Grants[0].Corporations = access.EntityIDs{20}
	if _, err = acl.SaveRole(ctx, "test", role); err != nil {
		t.Fatal(err)
	}
	check(0)
}

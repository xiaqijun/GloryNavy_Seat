package attendance

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"sync"
	"testing"
)

func TestPAPRequirementDefaultPermissionsAndVersion(t *testing.T) {
	s, _ := attendanceFixture(t)
	ctx := context.Background()
	s.Administrator = func(_ context.Context, user string) (bool, error) { return user == manager, nil }
	r, err := s.PAPRequirement(ctx, member)
	if err != nil || r.MonthlyPoints != 3 || r.CanManage || r.Source != "alliance" {
		t.Fatal(r, err)
	}
	c := PAPRequirementChange{MonthlyPoints: 5, Version: r.Version}
	if err = s.SetPAPRequirement(ctx, member, c); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("nonadmin write", err)
	}
	if err = s.SetPAPRequirement(ctx, manager, PAPRequirementChange{MonthlyPoints: 0, Version: 1}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	out := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); out <- s.SetPAPRequirement(ctx, manager, c) }()
	}
	wg.Wait()
	close(out)
	saved, conflicts := 0, 0
	for e := range out {
		if e == nil {
			saved++
		} else if errors.Is(e, ErrConflict) {
			conflicts++
		} else {
			t.Fatal(e)
		}
	}
	if saved != 1 || conflicts != 1 {
		t.Fatal(saved, conflicts)
	}
	r, err = s.PAPRequirement(ctx, manager)
	if err != nil || r.MonthlyPoints != 5 || r.Version != 2 || !r.CanManage {
		t.Fatal(r, err)
	}
	var n int
	if err = s.Pool.QueryRow(ctx, `SELECT count(*) FROM attendance_pap_requirement_audit WHERE previous_points=3 AND monthly_points=5 AND actor_id=$1::uuid`, manager).Scan(&n); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	calls := 0
	s.Administrator = func(context.Context, string) (bool, error) { calls++; return calls == 1, nil }
	if err = s.SetPAPRequirement(ctx, manager, PAPRequirementChange{MonthlyPoints: 8, Version: 2}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("revoked admin", err)
	}
}

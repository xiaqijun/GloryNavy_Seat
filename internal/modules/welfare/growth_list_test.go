package welfare

import (
	"context"
	"testing"
)

func TestGrowthListScope(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	for _, row := range []struct {
		owner, kind string
		corp        int64
	}{
		{userID, "growth_gila", 10}, {userID, "growth_ishtar", 10},
		{userID, "solo", 10}, {otherID, "growth_gila", 10}, {userID, "growth_gila", 20},
	} {
		if _, err := s.Pool.Exec(ctx, `INSERT INTO welfare_cases(account_id,corporation_id,kind,state,detail,claim_keys) VALUES($1,$2,$3,'submitted','{}','{}')`, row.owner, row.corp, row.kind); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := s.List(ctx, userID, 10, false, "growth", 0)
	if err != nil || len(rows) != 2 {
		t.Fatalf("growth scope: %v %v", rows, err)
	}
	older, err := s.List(ctx, userID, 10, false, "growth", rows[0].ID)
	if err != nil || len(older) != 1 || older[0].ID != rows[1].ID {
		t.Fatalf("cursor: %v %v", older, err)
	}
	rows, err = s.List(ctx, adminID, 10, true, "growth", 0)
	if err != nil || len(rows) != 3 {
		t.Fatalf("managed growth: %v %v", rows, err)
	}
	rows, err = s.List(ctx, userID, 10, false, "growth_gila", 0)
	if err != nil || len(rows) != 1 {
		t.Fatalf("exact kind: %v %v", rows, err)
	}
}

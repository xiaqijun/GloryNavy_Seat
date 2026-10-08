package welfare

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestCompensationPermissionOnlyExpandsLossManagement(t *testing.T) {
	s := &Service{
		Scope: func(context.Context, string, int64, bool) (bool, error) { return false, nil },
		CompensationScope: func(context.Context, string, int64) (bool, error) {
			return true, nil
		},
	}
	if err := s.allowedCase(context.Background(), "actor", 10, "srp", true); err != nil {
		t.Fatalf("compensation officer should manage loss cases: %v", err)
	}
	if err := s.allowedCase(context.Background(), "actor", 10, "growth_gila", true); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("compensation officer must not manage growth cases: %v", err)
	}
	if err := s.allowedCase(context.Background(), "actor", 10, "srp", false); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("compensation grant must not replace ordinary member scope: %v", err)
	}
}

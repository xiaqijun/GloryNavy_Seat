package attendance

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestAllianceConversionRequiresSiteAdministrator(t *testing.T) {
	s, _ := attendanceFixture(t)
	s.Administrator = func(context.Context, string) (bool, error) { return false, nil }
	if _, err := s.ConvertAlliancePAP(context.Background(), member, nil); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("non-administrator reached alliance conversion: %v", err)
	}
}

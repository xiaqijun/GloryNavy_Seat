package attendance

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestFleetNoticePermissionsVersionsAndReplay(t *testing.T) {
	s, _ := attendanceFixture(t)
	ctx := context.Background()
	e := createTestEvent(t, s)
	input := FleetNoticeInput{SolarSystem: "Jita", Doctrine: "护卫舰", Revision: 0, RequestKey: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"}
	if _, err := s.SaveFleetNotice(ctx, member, e.ID, input); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("unauthorized save: %v", err)
	}
	first, err := s.SaveFleetNotice(ctx, manager, e.ID, input)
	if err != nil || first.Revision != 1 {
		t.Fatal(first, err)
	}
	again, err := s.SaveFleetNotice(ctx, manager, e.ID, input)
	if err != nil || again.Revision != 1 {
		t.Fatal("replay", again, err)
	}
	input.RequestKey = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	input.SolarSystem = "Amarr"
	if _, err = s.SaveFleetNotice(ctx, manager, e.ID, input); !errors.Is(err, ErrConflict) {
		t.Fatal("stale revision", err)
	}
	input.Revision = 1
	updated, err := s.SaveFleetNotice(ctx, manager, e.ID, input)
	if err != nil || updated.Revision != 2 || updated.SolarSystem != "Amarr" {
		t.Fatal(updated, err)
	}
	current, err := s.FleetNotice(ctx, manager, e.ID)
	if err != nil || current == nil || current.Revision != 2 {
		t.Fatal(current, err)
	}
	if _, err = s.FleetNotice(ctx, member, e.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("unauthorized read", err)
	}
}

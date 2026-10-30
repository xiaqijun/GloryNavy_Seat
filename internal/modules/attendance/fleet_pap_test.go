package attendance

import (
	"context"
	"errors"
	"testing"

	"glorynavy.local/seat/internal/modules/eve"
)

func TestFleetPAPIssueAndLateJoin(t *testing.T) {
	s, fleet := attendanceFixture(t)
	ctx := context.Background()
	e := createTestEvent(t, s)
	fleet.fleet.Members = []eve.FleetMember{{ID: 1, Name: "Lead", CorporationID: 10}, {ID: 2, Name: "Alt", CorporationID: 10}, {ID: 3, Name: "External", CorporationID: 20}}
	c := Change{Version: e.Version, RequestKey: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", SourceID: 1, PAPPoints: 1}
	first, err := s.Change(ctx, manager, e.ID, "capture_pap", c)
	if err != nil || first.PAPAwarded != 2 || !first.Event.PAPIssued || first.Event.State != "open" {
		t.Fatal(first, err)
	}
	replay, err := s.Change(ctx, manager, e.ID, "capture_pap", c)
	if err != nil || replay.PAPAwarded != 2 {
		t.Fatal("idempotent replay", replay, err)
	}
	fleet.fleet.Members = []eve.FleetMember{{ID: 1, Name: "Lead", CorporationID: 10}, {ID: 4, Name: "Pilot", CorporationID: 10}}
	c.Version = first.Event.Version
	c.RequestKey = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	second, err := s.Change(ctx, manager, e.ID, "capture_pap", c)
	if err != nil || second.PAPAwarded != 1 {
		t.Fatal("late member", second, err)
	}
	var awards, ledger int
	if err = s.Pool.QueryRow(ctx, `SELECT count(*) FROM attendance_pap_awards WHERE event_id=$1 AND points=1`, e.ID).Scan(&awards); err != nil {
		t.Fatal(err)
	}
	if err = s.Pool.QueryRow(ctx, `SELECT count(*) FROM attendance_pap_ledger WHERE event_id=$1`, e.ID).Scan(&ledger); err != nil {
		t.Fatal(err)
	}
	if awards != 3 || ledger != 3 {
		t.Fatalf("awards=%d ledger=%d", awards, ledger)
	}
	c.Version, c.RequestKey, c.PAPPoints = second.Event.Version, "dddddddd-dddd-4ddd-8ddd-dddddddddddd", 2
	if _, err = s.Change(ctx, manager, e.ID, "capture_pap", c); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed score: %v", err)
	}
}

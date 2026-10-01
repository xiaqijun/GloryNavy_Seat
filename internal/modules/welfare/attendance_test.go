package welfare

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"testing"
)

func TestWelfareSRPRequiresConfirmedAttendance(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	c := Command{Action: "apply", CorporationID: 10, Kind: "srp", RequestKey: key(500), Detail: Detail{CharacterID: 123, ShipTypeID: 17715, KillmailID: 900, EventID: 999, OccurredAt: "2026-09-01T01:00:00Z", Description: "军团损失", Evidence: "KM"}}
	s.GuardAttendanceLoss = func(context.Context, pgx.Tx, string, int64, int64, int64) (int64, error) { return 0, pgx.ErrNoRows }
	if _, err := s.Execute(ctx, adminID, c); !errors.Is(err, ErrAttendanceRequired) {
		t.Fatalf("manual/forged event bypass: %v", err)
	}
	s.GuardAttendanceLoss = func(_ context.Context, _ pgx.Tx, owner string, corp, char, km int64) (int64, error) {
		if owner != adminID || corp != 10 || char != 123 || km != 900 {
			t.Fatal("wrong guard arguments")
		}
		return 1234, nil
	}
	b, err := s.Execute(ctx, adminID, c)
	if err != nil {
		t.Fatal(err)
	}
	var v Case
	_ = json.Unmarshal(b, &v)
	var d Detail
	_ = json.Unmarshal(v.Detail, &d)
	if d.EventID != 1234 {
		t.Fatal("trusted client event", d.EventID)
	}
	// Use an independent reviewer and revoke the linkage before approval.
	s.Scope = func(context.Context, string, int64, bool) (bool, error) { return true, nil }
	s.Members = func(context.Context, string, int64) ([]Character, error) {
		return []Character{{AccountID: adminID}}, nil
	}
	s.GuardAttendanceLoss = func(context.Context, pgx.Tx, string, int64, int64, int64) (int64, error) { return 0, pgx.ErrNoRows }
	_, err = s.Execute(ctx, userID, Command{Action: "approve", ID: v.ID, Version: 1, RequestKey: key(501), Note: "核对", Detail: Detail{BaseMinor: 100}})
	if !errors.Is(err, ErrAttendanceRequired) {
		t.Fatalf("revoked attendance approved: %v", err)
	}
	c.Kind = "solo"
	c.RequestKey = key(502)
	if _, err = s.Execute(ctx, adminID, c); err != nil {
		t.Fatalf("PVP incorrectly gated: %v", err)
	}
	s.AttendanceLosses = nil
	rows, err := s.annotateLosses(ctx, otherID, 10, 123, []Loss{{ID: 999, CharacterID: 123}})
	if err != nil || len(rows) != 1 || len(rows[0].Reimbursement.AvailableKinds) != 1 || rows[0].Reimbursement.AvailableKinds[0] != "solo" {
		t.Fatalf("unlinked listing: %+v %v", rows, err)
	}
}

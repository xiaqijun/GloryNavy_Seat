package attendance

import (
	"context"
	"testing"
)

func TestFleetDeliveryAccountMerge(t *testing.T) {
	s, _ := attendanceFixture(t)
	ctx := context.Background()
	e := createTestEvent(t, s)
	_, err := s.SaveFleetNotice(ctx, manager, e.ID, FleetNoticeInput{SolarSystem: "Jita", Doctrine: "护卫舰", RequestKey: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"})
	if err != nil {
		t.Fatal(err)
	}
	s.ReportBindings = func(context.Context, string, string, int64) ([]Binding, error) {
		return []Binding{{ID: 4, UserID: member}}, nil
	}
	s.FleetTarget = func(context.Context, string, int64) (string, error) { return "v1", nil }
	s.FleetSend = func(context.Context, string, int64, string, string, string, string) FleetDeliveryResult {
		return FleetDeliveryResult{State: "sent"}
	}
	_, err = s.FleetDispatch(ctx, manager, e.ID, FleetDispatchInput{Revision: 1, RequestKey: "cccccccc-cccc-4ccc-8ccc-cccccccccccc", Channels: []string{"email"}})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	before, err := s.MergeAccountTx(ctx, tx, member, manager, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE attendance_fleet_deliveries SET state='sending' WHERE account_id=$1`, member); err != nil {
		t.Fatal(err)
	}
	after, err := s.MergeAccountTx(ctx, tx, member, manager, false)
	if err != nil || string(before) == string(after) {
		t.Fatal("delivery state must change merge preview", err)
	}
	if _, err = s.MergeAccountTx(ctx, tx, member, manager, true); err != nil {
		t.Fatal(err)
	}
	var owner, original, state, reason string
	if err = tx.QueryRow(ctx, `SELECT account_id::text,original_account_id::text,state,reason FROM attendance_fleet_deliveries`).Scan(&owner, &original, &state, &reason); err != nil || owner != manager || original != member || state != "unknown" || reason != "account_merged" {
		t.Fatal(owner, original, state, reason, err)
	}
}

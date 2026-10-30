package attendance

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/attendance/internal/store"
)

func TestFleetDispatchChannelsDeduplicateAndDoNotRetryUnknown(t *testing.T) {
	s, _ := attendanceFixture(t)
	ctx := context.Background()
	e := createTestEvent(t, s)
	_, err := s.SaveFleetNotice(ctx, manager, e.ID, FleetNoticeInput{SolarSystem: "Jita", Doctrine: "轻型舰", RequestKey: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"})
	if err != nil {
		t.Fatal(err)
	}
	s.ReportBindings = func(context.Context, string, string, int64) ([]Binding, error) {
		return []Binding{{ID: 1, UserID: manager}, {ID: 2, UserID: manager}, {ID: 4, UserID: member}}, nil
	}
	s.FleetTarget = func(context.Context, string, int64) (string, error) { return "v1", nil }
	calls := map[string]int{}
	s.FleetSend = func(_ context.Context, ch string, _ int64, version, account, subject, body string) FleetDeliveryResult {
		if version != "v1" || subject != "Fleet" || body == "" {
			t.Fatal("unexpected payload")
		}
		calls[ch]++
		if ch == "qq" {
			return FleetDeliveryResult{State: "unknown", Reason: "confirmation_required"}
		}
		if ch == "email" && account == member {
			return FleetDeliveryResult{State: "skipped", Reason: "email_unavailable"}
		}
		return FleetDeliveryResult{State: "sent"}
	}
	in := FleetDispatchInput{Revision: 1, RequestKey: "cccccccc-cccc-4ccc-8ccc-cccccccccccc", Channels: []string{"email", "qq", "eve_mail"}}
	first, err := s.FleetDispatch(ctx, manager, e.ID, in)
	if err != nil || first.ID <= 0 {
		t.Fatal(first, err)
	}
	s.FleetTarget = func(context.Context, string, int64) (string, error) { return "", errors.New("offline") }
	replay, err := s.FleetDispatch(ctx, manager, e.ID, in)
	if err != nil || replay.ID != first.ID {
		t.Fatal("replay", replay, err)
	}
	for i := 0; i < 4; i++ {
		d, err := store.ClaimFleetDelivery(ctx, s.Pool)
		if err != nil {
			t.Fatal(err)
		}
		if err = s.processFleetDelivery(ctx, d); err != nil {
			t.Fatal(err)
		}
	}
	if calls["qq"] != 1 || calls["eve_mail"] != 1 || calls["email"] != 2 {
		t.Fatal(calls)
	}
	status, err := s.LatestFleetDispatch(ctx, manager, e.ID)
	if err != nil || status == nil {
		t.Fatal(status, err)
	}
	seen := map[string]int64{}
	for _, c := range status.Counts {
		seen[c.Channel+":"+c.State] = c.Count
	}
	if seen["qq:unknown"] != 1 || seen["eve_mail:sent"] != 1 || seen["email:sent"] != 1 || seen["email:skipped"] != 1 {
		t.Fatal(seen)
	}
	if err = store.MaintainFleetDeliveries(ctx, s.Pool); err != nil {
		t.Fatal(err)
	}
	if _, err = store.ClaimFleetDelivery(ctx, s.Pool); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("unknown retried", err)
	}
}

func TestFleetDispatchRechecksManagerBeforeSending(t *testing.T) {
	s, _ := attendanceFixture(t)
	ctx := context.Background()
	e := createTestEvent(t, s)
	_, err := s.SaveFleetNotice(ctx, manager, e.ID, FleetNoticeInput{SolarSystem: "Jita", Doctrine: "护卫舰", RequestKey: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"})
	if err != nil {
		t.Fatal(err)
	}
	s.FleetTarget = func(context.Context, string, int64) (string, error) { return "v1", nil }
	called := false
	s.FleetSend = func(context.Context, string, int64, string, string, string, string) FleetDeliveryResult {
		called = true
		return FleetDeliveryResult{State: "sent"}
	}
	_, err = s.FleetDispatch(ctx, manager, e.ID, FleetDispatchInput{Revision: 1, RequestKey: "cccccccc-cccc-4ccc-8ccc-cccccccccccc", Channels: []string{"qq"}})
	if err != nil {
		t.Fatal(err)
	}
	d, err := store.ClaimFleetDelivery(ctx, s.Pool)
	if err != nil {
		t.Fatal(err)
	}
	s.Manage = func(context.Context, string, int64) (bool, error) { return false, nil }
	if err = s.processFleetDelivery(ctx, d); err != nil || called {
		t.Fatal("permission not enforced", err, called)
	}
}

func TestFleetDeliveryRecoveryDoesNotResendUncertainRequest(t *testing.T) {
	s, _ := attendanceFixture(t)
	ctx := context.Background()
	e := createTestEvent(t, s)
	_, err := s.SaveFleetNotice(ctx, manager, e.ID, FleetNoticeInput{SolarSystem: "Jita", Doctrine: "护卫舰", RequestKey: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"})
	if err != nil {
		t.Fatal(err)
	}
	s.FleetTarget = func(context.Context, string, int64) (string, error) { return "v1", nil }
	s.FleetSend = func(context.Context, string, int64, string, string, string, string) FleetDeliveryResult {
		return FleetDeliveryResult{State: "sent"}
	}
	_, err = s.FleetDispatch(ctx, manager, e.ID, FleetDispatchInput{Revision: 1, RequestKey: "cccccccc-cccc-4ccc-8ccc-cccccccccccc", Channels: []string{"qq", "eve_mail"}})
	if err != nil {
		t.Fatal(err)
	}
	one, err := store.ClaimFleetDelivery(ctx, s.Pool)
	if err != nil {
		t.Fatal(err)
	}
	two, err := store.ClaimFleetDelivery(ctx, s.Pool)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.TransitionFleetDelivery(ctx, s.Pool, one.ID, "preparing", one.UpdatedAt, "sending", ""); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Pool.Exec(ctx, `UPDATE attendance_fleet_deliveries SET updated_at=now()-interval '4 minutes' WHERE id=ANY($1::bigint[])`, []int64{one.ID, two.ID}); err != nil {
		t.Fatal(err)
	}
	if err = store.MaintainFleetDeliveries(ctx, s.Pool); err != nil {
		t.Fatal(err)
	}
	status, err := s.LatestFleetDispatch(ctx, manager, e.ID)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]int64{}
	for _, c := range status.Counts {
		seen[c.Channel+":"+c.State] = c.Count
	}
	if seen[one.Channel+":unknown"] != 1 || seen[two.Channel+":pending"] != 1 {
		t.Fatal(seen)
	}
	if _, err = store.TransitionFleetDelivery(ctx, s.Pool, two.ID, "preparing", two.UpdatedAt, "sending", ""); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("stale worker not fenced", err)
	}
}

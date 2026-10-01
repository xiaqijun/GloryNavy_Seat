package welfare

import (
	"context"
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
	"testing"
	"time"
)

func TestWelfareLossStatusScopeAndLifecycle(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	_, err := s.Execute(ctx, adminID, Command{Action: "configure", CorporationID: 10, Kind: "srp", RequestKey: key(200), Config: Config{Enabled: true, EffectiveAt: "2020-01-01T00:00:00Z"}})
	if err != nil {
		t.Fatal(err)
	}
	loss := Loss{ID: 900, CharacterID: 123, CorporationID: 10, At: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}
	check := func(actor, want string) {
		t.Helper()
		rows, e := s.annotateLosses(ctx, actor, 10, 123, []Loss{loss})
		if e != nil || len(rows) != 1 {
			t.Fatalf("read: %v %+v", e, rows)
		}
		if rows[0].Reimbursement.Status != want {
			t.Fatalf("want %s, got %+v", want, rows[0].Reimbursement)
		}
		if want == "available" && (len(rows[0].Reimbursement.AvailableKinds) != 2 || rows[0].Reimbursement.AvailableKinds[0] != "srp" || rows[0].Reimbursement.AvailableKinds[1] != "solo") {
			t.Fatal("unexpected application kinds", rows[0].Reimbursement)
		}
	}
	check(userID, "available")
	loss.CorporationID = 20
	check(userID, "available")
	loss.CorporationID = 10
	loss.At = time.Date(2019, 9, 1, 0, 0, 0, 0, time.UTC)
	check(userID, "available")
	loss.At = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	saved, e := store.Save(ctx, s.Pool, Case{AccountID: userID, CorporationID: 10, Kind: "srp", State: "submitted", Detail: raw(Detail{CharacterID: 123, KillmailID: 900}), Keys: []string{}})
	if e != nil {
		t.Fatal(e)
	}
	for _, state := range []string{"submitted", "information", "external", "approved", "executing", "completed", "rejected", "cancelled"} {
		if _, e = s.Pool.Exec(ctx, `UPDATE welfare_cases SET state=$1 WHERE id=$2`, state, saved.ID); e != nil {
			t.Fatal(e)
		}
		want := "processing"
		if state == "completed" {
			want = "completed"
		}
		if state == "rejected" || state == "cancelled" {
			want = "available"
		}
		check(userID, want)
		check(adminID, want)
		check(otherID, "available")
	}
	// A newer withdrawn/rejected case must not mask an older completed reimbursement.
	if _, e = s.Pool.Exec(ctx, `UPDATE welfare_cases SET state='completed' WHERE id=$1`, saved.ID); e != nil {
		t.Fatal(e)
	}
	_, e = store.Save(ctx, s.Pool, Case{AccountID: userID, CorporationID: 10, Kind: "solo", State: "submitted", Detail: raw(Detail{CharacterID: 123, KillmailID: 900}), Keys: []string{}})
	if e != nil {
		t.Fatal(e)
	}
	check(userID, "completed")
}

func TestWelfareLossNoPolicyGate(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	loss := Loss{ID: 999, CharacterID: 123, CorporationID: 10, At: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}
	check := func(want string) {
		t.Helper()
		rows, err := s.annotateLosses(ctx, userID, 10, 123, []Loss{loss})
		if err != nil || len(rows) != 1 {
			t.Fatalf("read %v", err)
		}
		if rows[0].Reimbursement.Reason != want {
			t.Fatalf("want %s, got %+v", want, rows[0].Reimbursement)
		}
	}
	check("")
	for i, tt := range []struct {
		cfg  Config
		want string
	}{
		{Config{}, ""},
		{Config{Enabled: true, EffectiveAt: "2099-01-01T00:00:00Z"}, ""},
		{Config{Enabled: true, EffectiveAt: "2026-09-02T00:00:00Z"}, ""},
		{Config{Enabled: true, EffectiveAt: "2020-01-01T00:00:00Z"}, ""},
	} {
		if _, err := s.Execute(ctx, adminID, Command{Action: "configure", CorporationID: 10, Kind: "solo", RequestKey: key(300 + i), Version: int64(i), Config: func() Config { c := tt.cfg; c.DayZone = "UTC"; return c }()}); err != nil {
			t.Fatal(err)
		}
		check(tt.want)
	}
	loss.CorporationID = 20
	check("")
	loss.CorporationID = 10
	loss.At = time.Now().Add(time.Hour)
	check("future_loss")
}

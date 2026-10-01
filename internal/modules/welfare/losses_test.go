package welfare

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"testing"
	"time"
)

func TestWelfareManualLossRequiresCurrentAdministrator(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	for i, kind := range []string{"srp", "solo"} {
		_, err := s.Execute(ctx, adminID, Command{Action: "configure", CorporationID: 10, Kind: kind, RequestKey: key(100 + i), Config: Config{Enabled: true, EffectiveAt: "2020-01-01T00:00:00Z", DayZone: "UTC"}})
		if err != nil {
			t.Fatal(err)
		}
		c := Command{Action: "apply", CorporationID: 10, Kind: kind, RequestKey: key(110 + i), Detail: Detail{CharacterID: 123, ShipTypeID: 17715, KillmailID: int64(100 + i), OccurredAt: "2026-09-01T01:00:00Z", Description: "历史损失录入", Evidence: "人工核对记录", Alliance: "unknown"}}
		// A delegated manager still cannot create manual evidence.
		s.Scope = func(context.Context, string, int64, bool) (bool, error) { return true, nil }
		if _, err = s.Execute(ctx, userID, c); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("%s non-admin manual entry: %v", kind, err)
		}
		c.Detail.SyncedLoss = true
		if _, err = s.Execute(ctx, userID, c); err == nil {
			t.Fatal("forged synchronized evidence accepted")
		}
		c.Detail.SyncedLoss = false
		if _, err = s.Execute(ctx, adminID, c); err != nil {
			t.Fatalf("%s admin manual entry: %v", kind, err)
		}
		// Privileges can change between initial validation and publication.
		checks := 0
		s.Administrator = func(context.Context, string) (bool, error) { checks++; return checks == 1, nil }
		c.RequestKey = key(120 + i)
		if _, err = s.Execute(ctx, adminID, c); !errors.Is(err, pgx.ErrNoRows) || checks != 2 {
			t.Fatalf("late revocation: %v, checks %d", err, checks)
		}
		s.Administrator = func(_ context.Context, actor string) (bool, error) { return actor == adminID, nil }
	}
	var count int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM welfare_cases`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("partial publication: %d %v", count, err)
	}
}

func TestWelfareLossUsesServerSnapshot(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	_, e := s.Execute(ctx, adminID, Command{Action: "configure", CorporationID: 10, Kind: "srp", RequestKey: key(50), Config: Config{Enabled: true, EffectiveAt: "2020-01-01T00:00:00Z", DayZone: "UTC"}})
	if e != nil {
		t.Fatal(e)
	}
	loss := Loss{ID: 99, CharacterID: 123, CorporationID: 10, ShipTypeID: 17715, SolarSystemID: 30000142, SolarSystemName: "吉他", At: time.Date(2026, 9, 1, 1, 0, 0, 0, time.UTC), ObservedAt: time.Now()}
	s.Losses = func(_ context.Context, actor string, corp, char, before, id int64) ([]Loss, error) {
		if actor != userID || corp != 10 || char != 123 || id != 99 {
			return nil, pgx.ErrNoRows
		}
		return []Loss{loss}, nil
	}
	guards := 0
	s.GuardLoss = func(_ context.Context, _ pgx.Tx, actor string, char, id int64) error { guards++; return nil }
	command := Command{Action: "apply", CorporationID: 10, Kind: "srp", RequestKey: key(51), Detail: Detail{CharacterID: 123, ShipTypeID: 999, KillmailID: 99, OccurredAt: "2030-01-01T00:00:00Z", SyncedLoss: true, Description: "集结损失", Evidence: "ESI"}}
	b, e := s.Execute(ctx, userID, command)
	if e != nil {
		t.Fatal(e)
	}
	var saved Case
	if e = json.Unmarshal(b, &saved); e != nil {
		t.Fatal(e)
	}
	var d Detail
	if e = json.Unmarshal(saved.Detail, &d); e != nil {
		t.Fatal(e)
	}
	if d.ShipTypeID != 17715 || d.OccurredAt != loss.At.Format(time.RFC3339) || d.LossEvidence == nil || d.LossEvidence.SolarSystemName != "吉他" || guards != 1 {
		t.Fatal("browser fields replaced verified evidence", d, guards)
	}
	loss.CharacterID = 999
	command.RequestKey = key(52)
	if _, e = s.Execute(ctx, userID, command); e == nil {
		t.Fatal("wrong character loss accepted")
	}
	loss.CharacterID = 123
	s.GuardLoss = func(context.Context, pgx.Tx, string, int64, int64) error { return pgx.ErrNoRows }
	command.RequestKey = key(53)
	if _, e = s.Execute(ctx, userID, command); e == nil {
		t.Fatal("revoked evidence published")
	}
	var count int
	if e = s.Pool.QueryRow(ctx, `SELECT count(*) FROM welfare_cases`).Scan(&count); e != nil || count != 1 {
		t.Fatal("partial/late publication", count, e)
	}
}

package attendance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestEventSourcesPersistPerAccountAndProtectReplays(t *testing.T) {
	s, _ := attendanceFixture(t)
	s.AllianceLockAccounts = func(context.Context, pgx.Tx, []string) error { return nil }
	s.Manage = func(_ context.Context, user string, corp int64) (bool, error) {
		return (user == manager || user == member) && corp == 10, nil
	}
	ctx := context.Background()
	create := Create{CorporationID: 10, Title: "Fleet source", StartsAt: time.Now().Add(-time.Hour).UTC().Truncate(time.Microsecond), RequestKey: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", SourceID: 2}
	event, err := s.Create(ctx, manager, create)
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.EventSource(ctx, manager, event.ID)
	if err != nil || first.SourceID != "2" || first.Revision != "1" || first.Invalid {
		t.Fatalf("created selection: %+v %v", first, err)
	}
	updated, err := s.SetEventSource(ctx, manager, event.ID, SourceChange{SourceID: 1, Revision: 1})
	if err != nil || updated.SourceID != "1" || updated.Revision != "2" {
		t.Fatalf("updated selection: %+v %v", updated, err)
	}
	if _, err = s.SetEventSource(ctx, manager, event.ID, SourceChange{SourceID: 2, Revision: 1}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale revision: %v", err)
	}
	if _, err = s.Create(ctx, manager, create); err != nil {
		t.Fatalf("idempotent create: %v", err)
	}
	again, err := s.EventSource(ctx, manager, event.ID)
	if err != nil || again.SourceID != "1" || again.Revision != "2" {
		t.Fatalf("replay changed selection: %+v %v", again, err)
	}
	create.SourceID = 1
	if _, err = s.Create(ctx, manager, create); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed creation source: %v", err)
	}
	create.SourceID = 0
	if _, err = s.Create(ctx, manager, create); !errors.Is(err, ErrConflict) {
		t.Fatalf("omitted creation source: %v", err)
	}
	if _, err = s.SetEventSource(ctx, manager, event.ID, SourceChange{SourceID: 4, Revision: 2}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("other account source: %v", err)
	}
	other, err := s.EventSource(ctx, member, event.ID)
	if err != nil || other.SourceID != "" || other.Revision != "0" {
		t.Fatalf("other account initial: %+v %v", other, err)
	}
	other, err = s.SetEventSource(ctx, member, event.ID, SourceChange{SourceID: 4, Revision: 0})
	if err != nil || other.SourceID != "4" {
		t.Fatalf("other account selection: %+v %v", other, err)
	}
	again, err = s.EventSource(ctx, manager, event.ID)
	if err != nil || again.SourceID != "1" {
		t.Fatalf("selection leaked across accounts: %+v %v", again, err)
	}
	s.Own = func(context.Context, string) ([]Binding, error) { return nil, nil }
	again, err = s.EventSource(ctx, manager, event.ID)
	if err != nil || !again.Invalid || again.SourceID != "" || again.Revision != "2" {
		t.Fatalf("unbound selection: %+v %v", again, err)
	}
}

func TestEventSourceMergeKeepsDestinationChoice(t *testing.T) {
	s, _ := attendanceFixture(t)
	s.AllianceLockAccounts = func(context.Context, pgx.Tx, []string) error { return nil }
	s.Manage = func(_ context.Context, user string, corp int64) (bool, error) {
		return (user == manager || user == member) && corp == 10, nil
	}
	ctx := context.Background()
	event := createTestEvent(t, s)
	if _, err := s.SetEventSource(ctx, manager, event.ID, SourceChange{SourceID: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetEventSource(ctx, member, event.ID, SourceChange{SourceID: 4}); err != nil {
		t.Fatal(err)
	}
	second, err := s.Create(ctx, manager, Create{CorporationID: 10, Title: "Second fleet", StartsAt: time.Now().Add(-time.Hour), RequestKey: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SetEventSource(ctx, member, second.ID, SourceChange{SourceID: 4}); err != nil {
		t.Fatal(err)
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = s.MergeAccountTx(ctx, tx, member, manager, true); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	selected, err := s.EventSource(ctx, manager, event.ID)
	if err != nil || selected.SourceID != "1" {
		t.Fatalf("destination choice: %+v %v", selected, err)
	}
	var count int
	if err = s.Pool.QueryRow(ctx, `SELECT count(*) FROM attendance_event_sources WHERE account_id=$1`, member).Scan(&count); err != nil || count != 0 {
		t.Fatalf("source account rows: %d %v", count, err)
	}
	var transferred int64
	if err = s.Pool.QueryRow(ctx, `SELECT source_id FROM attendance_event_sources WHERE event_id=$1 AND account_id=$2`, second.ID, manager).Scan(&transferred); err != nil || transferred != 4 {
		t.Fatalf("transferred selection: %d %v", transferred, err)
	}
}

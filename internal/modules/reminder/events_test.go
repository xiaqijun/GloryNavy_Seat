package reminder

import (
	"context"
	"errors"
	"glorynavy.local/seat/internal/modules/reminder/internal/store"
	"testing"
	"time"
)

func TestReminderEventSelectionRotationAndIsolation(t *testing.T) {
	s, admin, u := fixture(t)
	ctx := context.Background()
	calls := map[string]int{}
	s.Sources = map[string]Source{}
	for _, kind := range []string{"loan_due", "coin_change"} {
		s.Sources[kind] = func(_ context.Context, account string, since time.Time) ([]string, error) {
			if account != u || since.IsZero() {
				t.Fatal("recipient or baseline", account, since)
			}
			calls[kind]++
			return []string{"same-id", "same-id"}, nil
		}
	}
	c, e := s.Save(ctx, admin, SaveInput{Enabled: true, SenderID: 101, Revision: 2, Events: []string{"loan_due", "coin_change"}})
	if e != nil {
		t.Fatal(e)
	}
	if len(c.AvailableEvents) != 3 || len(c.Events) != 2 {
		t.Fatal(c)
	}
	for i := 0; i < 2; i++ {
		if e = s.deliver(ctx, u); e != nil {
			t.Fatal(e)
		}
		if _, e = s.Pool.Exec(ctx, `UPDATE reminder_deliveries SET created_at=created_at-interval '6 minutes'`); e != nil {
			t.Fatal(e)
		}
	}
	rows, e := s.Deliveries(ctx, admin, "")
	if e != nil || len(rows) != 2 || rows[0].Event == rows[1].Event {
		t.Fatal(rows, e)
	}
	for _, row := range rows {
		if row.Count != 1 || row.State != "sent" {
			t.Fatal(row)
		}
	}
	if e = s.deliver(ctx, u); e != nil {
		t.Fatal(e)
	}
	rows, _ = s.Deliveries(ctx, admin, "")
	if len(rows) != 2 {
		t.Fatal("duplicate", rows)
	}
	// A temporary loss of observations must not re-send the same non-approval fact.
	original := s.Sources["coin_change"]
	s.Sources["coin_change"] = func(context.Context, string, time.Time) ([]string, error) { return []string{}, nil }
	if e = s.deliver(ctx, u); e != nil {
		t.Fatal(e)
	}
	s.Sources["coin_change"] = original
	if e = s.deliver(ctx, u); e != nil {
		t.Fatal(e)
	}
	rows, _ = s.Deliveries(ctx, admin, "")
	if len(rows) != 2 {
		t.Fatal("observation gap replayed", rows)
	}

	// An unavailable source must not clear its receipt or suppress other categories.
	s.Sources["coin_change"] = func(context.Context, string, time.Time) ([]string, error) { return nil, errors.New("unavailable") }
	if e = s.deliver(ctx, u); e == nil {
		t.Fatal("source error hidden")
	}
	var active int
	if e = s.Pool.QueryRow(ctx, `SELECT count(*) FROM reminder_receipts WHERE active`).Scan(&active); e != nil || active != 2 {
		t.Fatal(active, e)
	}
	// Disabling or re-enabling must preserve deduplication while advancing the change baseline.
	old := c.Started["loan_due"]
	c, e = s.Save(ctx, admin, SaveInput{Enabled: false, SenderID: 101, Revision: c.Revision})
	if e != nil {
		t.Fatal(e)
	}
	c, e = s.Save(ctx, admin, SaveInput{Enabled: true, SenderID: 101, Revision: c.Revision})
	if e != nil || !c.Started["loan_due"].After(old) {
		t.Fatal(c, e)
	}
	if _, e = s.Save(ctx, admin, SaveInput{Enabled: true, SenderID: 101, Revision: c.Revision, Events: []string{}}); !errors.Is(e, ErrEvents) {
		t.Fatal(e)
	}
	for _, bad := range [][]string{{"unknown"}, {"loan_due", "loan_due"}, {"welfare_result"}} {
		if _, e = s.Save(ctx, admin, SaveInput{Enabled: true, SenderID: 101, Revision: c.Revision, Events: bad}); !errors.Is(e, ErrEvents) {
			t.Fatal(bad, e)
		}
	}
}

func TestReminderSourceWindowAndFinalEventRecheck(t *testing.T) {
	s, admin, u := fixture(t)
	ctx := context.Background()
	n := 0
	sent := false
	s.Sources = map[string]Source{"loan_due": func(_ context.Context, _ string, since time.Time) ([]string, error) {
		if since.Before(time.Now().Add(-31 * 24 * time.Hour)) {
			t.Fatal("unbounded history")
		}
		n++
		if n > 1 {
			return []string{}, nil
		}
		return []string{"a"}, nil
	}}
	if _, e := s.Save(ctx, admin, SaveInput{Enabled: true, SenderID: 101, Revision: 2, Events: []string{"loan_due"}}); e != nil {
		t.Fatal(e)
	}
	s.Send = func(context.Context, Character, int64, string, string) Result {
		sent = true
		return Result{State: "sent", ID: 1}
	}
	if e := s.deliver(ctx, u); e != nil || sent {
		t.Fatal(e, sent)
	}
	rows, _ := s.Deliveries(ctx, admin, "")
	if len(rows) != 1 || rows[0].Reason != "context_changed" {
		t.Fatal(rows)
	}
	// New event receipts survive account merging with their category intact.
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	if _, e = store.Merge(ctx, tx, u, admin, true); e != nil {
		t.Fatal(e)
	}
	var event string
	if e = tx.QueryRow(ctx, `SELECT event FROM reminder_receipts WHERE account_id=$1`, admin).Scan(&event); e != nil || event != "loan_due" {
		t.Fatal(event, e)
	}
}

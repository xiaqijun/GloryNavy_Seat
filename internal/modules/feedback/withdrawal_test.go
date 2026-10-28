package feedback_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"glorynavy.local/seat/internal/modules/exchange"
	"glorynavy.local/seat/internal/modules/feedback"
	"glorynavy.local/seat/migrations"
)

func TestFeedbackWithdrawalHistoryReplayAndResubmitReminder(t *testing.T) {
	s, p, a := fixture(t)
	ctx := context.Background()
	since := time.Now().Add(-time.Hour)
	v, err := s.Create(ctx, a[0], input(910))
	if err != nil {
		t.Fatal(err)
	}
	var id int64
	fmt.Sscan(v.ID, &id)
	initial, err := s.ReminderKeys(ctx, a[2].ID, "feedback_pending", since)
	if err != nil || len(initial) != 1 {
		t.Fatal(initial, err)
	}
	in := feedback.Reply{RequestKey: input(911).RequestKey, Revision: v.Revision, Status: "withdrawn"}
	_, err = s.Reply(ctx, a[2], id, in)
	check(t, err, 403)
	_, err = s.Reply(ctx, a[1], id, in)
	check(t, err, 404)
	v, err = s.Reply(ctx, a[0], id, in)
	if err != nil || v.Status != "withdrawn" || v.Revision != 2 || v.AcceptedAt != nil {
		t.Fatal(v, err)
	}
	again, err := s.Reply(ctx, a[0], id, in)
	if err != nil || again.Revision != 2 {
		t.Fatal(again, err)
	}
	keys, err := s.ReminderKeys(ctx, a[2].ID, "feedback_pending", since)
	if err != nil || len(keys) != 0 {
		t.Fatal(keys, err)
	}
	stale, err := s.ReminderDetails(ctx, a[2].ID, "feedback_pending", since)
	if err != nil || len(stale) != 0 {
		t.Fatal(stale, err)
	}
	listed, err := s.List(ctx, a[0].ID, false, "", "withdrawn", 0)
	if err != nil || len(listed.Tickets) != 1 || listed.Tickets[0].ID != v.ID {
		t.Fatal(listed, err)
	}
	d, err := s.Detail(ctx, a[0].ID, id)
	if err != nil || d.Ticket.Body == "" || len(d.Messages) != 1 || d.Messages[0].Staff {
		t.Fatal(d, err)
	}
	_, err = s.Reply(ctx, a[0], id, feedback.Reply{RequestKey: input(912).RequestKey, Revision: v.Revision, Status: "submitted"})
	check(t, err, 400)
	v, err = s.Reply(ctx, a[0], id, feedback.Reply{RequestKey: input(913).RequestKey, Revision: v.Revision, Status: "submitted", Body: "补充复现后重新提交"})
	if err != nil || v.Status != "submitted" {
		t.Fatal(v, err)
	}
	keys, err = s.ReminderKeys(ctx, a[2].ID, "feedback_pending", since)
	if err != nil || len(keys) != 1 || !strings.HasPrefix(keys[0], "returned:") || keys[0] == initial[0] {
		t.Fatal(keys, err)
	}
	details, err := s.ReminderDetails(ctx, a[2].ID, "feedback_pending", since)
	if err != nil || len(details) != 1 {
		t.Fatal(details, err)
	}
	// The old request is replayed even after resubmission; it cannot withdraw again.
	again, err = s.Reply(ctx, a[0], id, in)
	if err != nil || again.Status != "submitted" || again.Revision != v.Revision {
		t.Fatal(again, err)
	}
	db := stdlib.OpenDB(*p.Config().ConnConfig)
	defer db.Close()
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.DownTo(ctx, 107)
	if err == nil || !strings.Contains(err.Error(), "withdrawal history") {
		t.Fatalf("withdrawal history must block Down: %v", err)
	}
}

func TestFeedbackWithdrawalRacesAcceptanceWithoutDuplicateRewards(t *testing.T) {
	s, p, a := fixture(t)
	ctx := context.Background()
	coins := &exchange.Service{Pool: p, AllowNew: true, Administrator: s.Administrator}
	s.Credit = coins.FeedbackGrantTx
	_, err := s.ConfigureReward(ctx, a[2].ID, feedback.RewardConfig{Amount: 12345, Version: 1, RequestKey: input(920).RequestKey})
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.Create(ctx, a[0], input(921))
	if err != nil {
		t.Fatal(err)
	}
	var id int64
	fmt.Sscan(v.ID, &id)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i, state := range []string{"withdrawn", "accepted"} {
		wg.Go(func() {
			actor := a[0]
			if state == "accepted" {
				actor = a[2]
			}
			_, e := s.Reply(ctx, actor, id, feedback.Reply{RequestKey: input(922 + i).RequestKey, Revision: v.Revision, Status: state})
			errs <- e
		})
	}
	wg.Wait()
	close(errs)
	ok, conflicts := 0, 0
	for e := range errs {
		if e == nil {
			ok++
		} else {
			check(t, e, 409)
			conflicts++
		}
	}
	if ok != 1 || conflicts != 1 {
		t.Fatal(ok, conflicts)
	}
	d, err := s.Detail(ctx, a[0].ID, id)
	if err != nil || len(d.Messages) != 1 {
		t.Fatal(d, err)
	}
	var count int
	if err = p.QueryRow(ctx, `SELECT count(*) FROM exchange_coin_ledger WHERE account_id=$1`, a[0].ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	want := 0
	if d.Ticket.Status == "accepted" {
		want = 1
	}
	if count != want {
		t.Fatal("reward count", count, want)
	}
	if d.Ticket.Status == "accepted" {
		_, err = s.Reply(ctx, a[0], id, feedback.Reply{RequestKey: input(925).RequestKey, Revision: d.Ticket.Revision, Status: "withdrawn"})
		check(t, err, 409)
	}
}

package feedback_test

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/exchange"
	"glorynavy.local/seat/internal/modules/feedback"
	"sync"
	"testing"
)

func TestRewardAdoptionAtomicOnceAndConfiguration(t *testing.T) {
	s, p, a := fixture(t)
	ctx := context.Background()
	coins := &exchange.Service{Pool: p, AllowNew: true, Administrator: s.Administrator}
	s.Credit = coins.FeedbackGrantTx
	_, err := s.ConfigureReward(ctx, a[0].ID, feedback.RewardConfig{Amount: 12345, Version: 1, RequestKey: input(201).RequestKey})
	check(t, err, 403)
	cfg, err := s.ConfigureReward(ctx, a[2].ID, feedback.RewardConfig{Amount: 12345, Version: 1, RequestKey: input(201).RequestKey})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ConfigureReward(ctx, a[2].ID, feedback.RewardConfig{Amount: 12345, Version: 1, RequestKey: input(201).RequestKey})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ConfigureReward(ctx, a[2].ID, feedback.RewardConfig{Amount: 999, Version: cfg.Version, RequestKey: input(201).RequestKey})
	check(t, err, 409)
	v, err := s.Create(ctx, a[0], input(202))
	if err != nil {
		t.Fatal(err)
	}
	var id int64
	fmt.Sscan(v.ID, &id)
	r := feedback.Reply{RequestKey: input(203).RequestKey, Revision: 1, Status: "accepted"}
	_, err = s.Reply(ctx, a[0], id, r)
	check(t, err, 403)
	s.Credit = func(ctx context.Context, tx pgx.Tx, actor, owner string, id, amount int64) error {
		if err := coins.FeedbackGrantTx(ctx, tx, actor, owner, id, amount); err != nil {
			return err
		}
		return errors.New("forced after ledger write")
	}
	if _, err = s.Reply(ctx, a[2], id, r); err == nil {
		t.Fatal("failed ledger transaction accepted")
	}
	d, err := s.Detail(ctx, a[0].ID, id)
	if err != nil || d.Ticket.AcceptedAt != nil || len(d.Messages) != 0 || d.Ticket.Revision != 1 {
		t.Fatal(d, err)
	}
	var n int
	p.QueryRow(ctx, `SELECT count(*) FROM exchange_coin_ledger`).Scan(&n)
	if n != 0 {
		t.Fatal("rollback left ledger", n)
	}
	s.Credit = coins.FeedbackGrantTx
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for range 4 {
		wg.Go(func() { _, err := s.Reply(ctx, a[2], id, r); errs <- err })
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	d, err = s.Detail(ctx, a[0].ID, id)
	if err != nil || d.Ticket.RewardMinor == nil || *d.Ticket.RewardMinor != 12345 || len(d.Messages) != 1 {
		t.Fatal(d, err)
	}
	cfg, err = s.ConfigureReward(ctx, a[2].ID, feedback.RewardConfig{Amount: 99999, Version: cfg.Version, RequestKey: input(204).RequestKey})
	if err != nil {
		t.Fatal(err)
	}
	r.RequestKey = input(205).RequestKey
	r.Revision = d.Ticket.Revision
	r.Status = "in_progress"
	v, err = s.Reply(ctx, a[2], id, r)
	if err != nil {
		t.Fatal(err)
	}
	r.RequestKey = input(206).RequestKey
	r.Revision = v.Revision
	r.Status = "awaiting_confirmation"
	r.Body = "已完成处理，请确认"
	v, err = s.Reply(ctx, a[2], id, r)
	if err != nil || *v.RewardMinor != 12345 {
		t.Fatal(v, err)
	}
	var total int64
	if err = p.QueryRow(ctx, `SELECT count(*),coalesce(sum(delta),0) FROM exchange_coin_ledger WHERE account_id=$1`, a[0].ID).Scan(&n, &total); err != nil || n != 1 || total != 12345 {
		t.Fatal(n, total, err)
	}
	own, err := s.Create(ctx, a[2], input(207))
	if err != nil {
		t.Fatal(err)
	}
	fmt.Sscan(own.ID, &id)
	r.RequestKey = input(208).RequestKey
	r.Revision = 1
	_, err = s.Reply(ctx, a[2], id, r)
	check(t, err, 403)
}

func TestDisabledRewardDoesNotBackfillAndMergeRetainsClaim(t *testing.T) {
	s, p, a := fixture(t)
	ctx := context.Background()
	coins := &exchange.Service{Pool: p, AllowNew: true, Administrator: s.Administrator}
	s.Credit = coins.FeedbackGrantTx
	v, err := s.Create(ctx, a[0], input(301))
	if err != nil {
		t.Fatal(err)
	}
	var id int64
	fmt.Sscan(v.ID, &id)
	v, err = s.Reply(ctx, a[2], id, feedback.Reply{RequestKey: input(302).RequestKey, Revision: 1, Status: "accepted"})
	if err != nil || v.RewardMinor == nil || *v.RewardMinor != 0 {
		t.Fatal(v, err)
	}
	_, err = s.ConfigureReward(ctx, a[2].ID, feedback.RewardConfig{Amount: 500, Version: 1, RequestKey: input(303).RequestKey})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = s.MergeAccountTx(ctx, tx, a[0].ID, a[1].ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err = coins.MergeAccountTx(ctx, tx, a[0].ID, a[1].ID, true); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	d, err := s.Detail(ctx, a[1].ID, id)
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Reply(ctx, a[2], id, feedback.Reply{RequestKey: input(304).RequestKey, Revision: d.Ticket.Revision, Status: "in_progress"})
	if err != nil || v.RewardMinor == nil || *v.RewardMinor != 0 || v.AccountID != a[1].ID {
		t.Fatal(v, err)
	}
	var n int
	p.QueryRow(ctx, `SELECT count(*) FROM exchange_coin_ledger`).Scan(&n)
	if n != 0 {
		t.Fatal("disabled reward backfilled", n)
	}
}

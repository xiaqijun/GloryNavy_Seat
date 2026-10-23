package feedback_test

import (
	"context"
	"fmt"
	"glorynavy.local/seat/internal/modules/exchange"
	"glorynavy.local/seat/internal/modules/feedback"
	"testing"
)

func TestFeedbackPaidRewardMergeOnce(t *testing.T) {
	s, p, a := fixture(t)
	ctx := context.Background()
	coins := &exchange.Service{Pool: p, AllowNew: true, Administrator: s.Administrator}
	s.Credit = coins.FeedbackGrantTx
	_, err := s.ConfigureReward(ctx, a[2].ID, feedback.RewardConfig{Amount: 750, Version: 1, RequestKey: input(401).RequestKey})
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.Create(ctx, a[0], input(402))
	if err != nil {
		t.Fatal(err)
	}
	var id int64
	fmt.Sscan(v.ID, &id)
	_, err = s.Reply(ctx, a[2], id, feedback.Reply{RequestKey: input(403).RequestKey, Revision: 1, Status: "accepted"})
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
	v, err = s.Reply(ctx, a[2], id, feedback.Reply{RequestKey: input(404).RequestKey, Revision: d.Ticket.Revision, Status: "in_progress"})
	if err != nil || v.RewardMinor == nil || *v.RewardMinor != 750 {
		t.Fatal(v, err)
	}
	var n int
	var amount int64
	if err = p.QueryRow(ctx, `SELECT count(*),coalesce(sum(delta),0) FROM exchange_coin_ledger WHERE account_id=$1`, a[1].ID).Scan(&n, &amount); err != nil || n != 1 || amount != 750 {
		t.Fatal(n, amount, err)
	}
	var owner string
	if err = p.QueryRow(ctx, `SELECT account_id::text FROM exchange_source_awards WHERE source_id='feedback_reward' AND reference=$1`, v.ID).Scan(&owner); err != nil || owner != a[1].ID {
		t.Fatal(owner, err)
	}
}

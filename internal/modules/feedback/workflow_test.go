package feedback_test

import (
	"context"
	"fmt"
	"glorynavy.local/seat/internal/modules/exchange"
	"glorynavy.local/seat/internal/modules/feedback"
	"sync"
	"testing"
)

func TestFeedbackWorkflowConfirmationReturnAndReward(t *testing.T) {
	s, p, a := fixture(t)
	ctx := context.Background()
	coins := &exchange.Service{Pool: p, AllowNew: true, Administrator: s.Administrator}
	s.Credit = coins.FeedbackGrantTx
	_, err := s.ConfigureReward(ctx, a[2].ID, feedback.RewardConfig{Amount: 12345, Version: 1, RequestKey: input(501).RequestKey})
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.Create(ctx, a[0], input(502))
	if err != nil {
		t.Fatal(err)
	}
	var id int64
	fmt.Sscan(v.ID, &id)
	n := 510
	move := func(actor feedback.Actor, status, note string, want int) {
		t.Helper()
		n++
		in := feedback.Reply{RequestKey: input(n).RequestKey, Revision: v.Revision, Status: status, Body: note}
		next, e := s.Reply(ctx, actor, id, in)
		if want != 200 {
			check(t, e, want)
			return
		}
		if e != nil {
			t.Fatal(status, e)
		}
		if next.Status != status {
			t.Fatal(next.Status, status)
		}
		v = next
	}
	move(a[0], "accepted", "", 403)
	move(a[2], "resolved", "skip confirmation", 403)
	move(a[2], "awaiting_confirmation", "skip implementation", 409)
	move(a[2], "closed", "", 400)
	move(a[2], "accepted", "已采纳", 200)
	move(a[2], "in_progress", "", 200)
	move(a[2], "awaiting_confirmation", "", 400)
	move(a[2], "awaiting_confirmation", "已处理，请确认", 200)
	move(a[2], "resolved", "admin cannot confirm", 403)
	move(a[1], "resolved", "not owner", 404)
	move(a[0], "in_progress", "", 400)
	move(a[0], "in_progress", "仍有问题", 200)
	move(a[2], "accepted", "repeat reward", 409)
	move(a[2], "awaiting_confirmation", "修复完成", 200)
	move(a[0], "resolved", "确认完成", 200)
	move(a[0], "in_progress", "再次发现问题", 200)
	move(a[2], "awaiting_confirmation", "再次完成", 200)
	confirm := feedback.Reply{RequestKey: input(599).RequestKey, Revision: v.Revision, Status: "resolved"}
	v, err = s.Reply(ctx, a[0], id, confirm)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.Reply(ctx, a[0], id, confirm)
	if err != nil || again.Revision != v.Revision {
		t.Fatal(again, err)
	}
	var count int
	var amount int64
	if err = p.QueryRow(ctx, `SELECT count(*),coalesce(sum(delta),0) FROM exchange_coin_ledger WHERE account_id=$1`, a[0].ID).Scan(&count, &amount); err != nil || count != 1 || amount != 12345 {
		t.Fatal(count, amount, err)
	}
	d, err := s.Detail(ctx, a[0].ID, id)
	if err != nil || len(d.Messages) != 9 {
		t.Fatal(len(d.Messages), err)
	}
}

func TestFeedbackWorkflowDeclineResubmitAndConcurrentConfirmation(t *testing.T) {
	s, _, a := fixture(t)
	ctx := context.Background()
	v, err := s.Create(ctx, a[0], input(601))
	if err != nil {
		t.Fatal(err)
	}
	var id int64
	fmt.Sscan(v.ID, &id)
	moves := []struct {
		actor        feedback.Actor
		status, note string
	}{{a[2], "closed", "重复反馈"}, {a[0], "submitted", "补充依据，重新提交"}, {a[2], "accepted", ""}, {a[2], "in_progress", ""}, {a[2], "awaiting_confirmation", "已完成"}}
	for i, m := range moves {
		v, err = s.Reply(ctx, m.actor, id, feedback.Reply{RequestKey: input(610 + i).RequestKey, Revision: v.Revision, Status: m.status, Body: m.note})
		if err != nil {
			t.Fatal(m.status, err)
		}
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i, status := range []string{"resolved", "in_progress"} {
		wg.Go(func() {
			_, e := s.Reply(ctx, a[0], id, feedback.Reply{RequestKey: input(620 + i).RequestKey, Revision: v.Revision, Status: status, Body: "处理结果"})
			results <- e
		})
	}
	wg.Wait()
	close(results)
	ok, conflict := 0, 0
	for e := range results {
		if e == nil {
			ok++
		} else {
			check(t, e, 409)
			conflict++
		}
	}
	if ok != 1 || conflict != 1 {
		t.Fatal(ok, conflict)
	}
	d, err := s.Detail(ctx, a[0].ID, id)
	if err != nil || len(d.Messages) != 6 || d.Ticket.AcceptedAt == nil {
		t.Fatal(d, err)
	}
}

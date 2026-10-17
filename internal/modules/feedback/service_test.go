package feedback_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/modules/feedback"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/testutil"
	"sync"
	"testing"
)

func fixture(t *testing.T) (*feedback.Service, *pgxpool.Pool, []feedback.Actor) {
	t.Helper()
	p := testutil.Database(t)
	ids := identity.New(p)
	actors := []feedback.Actor{}
	for i := int64(1); i <= 3; i++ {
		token, e := ids.SignIn(context.Background(), i, fmt.Sprintf("Pilot %d", i), fmt.Sprintf("owner-%d", i), "")
		if e != nil {
			t.Fatal(e)
		}
		s, e := ids.Session(context.Background(), token)
		if e != nil {
			t.Fatal(e)
		}
		actors = append(actors, feedback.Actor{ID: s.UserID, Name: s.Character.Name})
	}
	return &feedback.Service{Pool: p, LockAccounts: ids.LockActiveAccounts, Administrator: func(_ context.Context, id string) (bool, error) { return id == actors[2].ID, nil }}, p, actors
}
func input(n int) feedback.Submission {
	return feedback.Submission{RequestKey: fmt.Sprintf("00000000-0000-4000-8000-%012d", n), Kind: "bug", Title: "无法提交申请", Body: "点击提交后显示错误。"}
}
func check(t *testing.T, e error, status int) {
	t.Helper()
	f, ok := e.(*feedback.Fault)
	if !ok || f.Status != status {
		t.Fatalf("want %d got %v", status, e)
	}
}
func TestPrivacyReplyAndOptimisticHandling(t *testing.T) {
	s, _, a := fixture(t)
	ctx := context.Background()
	v, e := s.Create(ctx, a[0], input(1))
	if e != nil {
		t.Fatal(e)
	}
	var id int64
	fmt.Sscan(v.ID, &id)
	l, e := s.List(ctx, a[1].ID, false, "", "", 0)
	if e != nil || len(l.Tickets) != 0 {
		t.Fatal(l, e)
	}
	_, e = s.List(ctx, a[1].ID, true, "", "", 0)
	check(t, e, 403)
	_, e = s.Detail(ctx, a[1].ID, id)
	check(t, e, 404)
	r := feedback.Reply{RequestKey: input(2).RequestKey, Revision: 1, Body: "补充操作步骤"}
	_, e = s.Reply(ctx, a[1], id, r)
	check(t, e, 404)
	r.Status = "resolved"
	_, e = s.Reply(ctx, a[0], id, r)
	check(t, e, 403)
	r.Status = ""
	v, e = s.Reply(ctx, a[0], id, r)
	if e != nil || v.Revision != 2 {
		t.Fatal(v, e)
	}
	again, e := s.Reply(ctx, a[0], id, r)
	if e != nil || again.Revision != 2 {
		t.Fatal(again, e)
	}
	r.RequestKey = input(3).RequestKey
	r.Status = "in_progress"
	_, e = s.Reply(ctx, a[2], id, r)
	check(t, e, 409)
	r.Revision = 2
	v, e = s.Reply(ctx, a[2], id, r)
	if e != nil || v.Status != "in_progress" {
		t.Fatal(v, e)
	}
	d, e := s.Detail(ctx, a[0].ID, id)
	if e != nil || len(d.Messages) != 2 || d.Messages[0].Staff || !d.Messages[1].Staff {
		t.Fatal(d, e)
	}
	l, e = s.List(ctx, a[2].ID, true, "bug", "in_progress", 0)
	if e != nil || len(l.Tickets) != 1 || l.Tickets[0].Body != "" {
		t.Fatal(l, e)
	}
}
func TestIdempotencyConcurrentSubmissionAndLimits(t *testing.T) {
	s, p, a := fixture(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Go(func() { _, e := s.Create(ctx, a[0], input(1)); errs <- e })
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var n int
	p.QueryRow(ctx, "SELECT count(*) FROM feedback_tickets").Scan(&n)
	if n != 1 {
		t.Fatal(n)
	}
	changed := input(1)
	changed.Body = "不同内容"
	_, e := s.Create(ctx, a[0], changed)
	check(t, e, 409)
	for i := 2; i <= 20; i++ {
		if _, e := s.Create(ctx, a[0], input(i)); e != nil {
			t.Fatal(e)
		}
	}
	_, e = s.Create(ctx, a[0], input(21))
	check(t, e, 429)
	if _, e = s.Create(ctx, a[0], input(1)); e != nil {
		t.Fatal("retry at limit", e)
	}
	for _, bad := range []feedback.Submission{{Kind: "bug", Title: "x", Body: "x"}, {RequestKey: input(22).RequestKey, Kind: "unknown", Title: "x", Body: "x"}, {RequestKey: input(23).RequestKey, Kind: "bug", Title: " ", Body: "x"}} {
		_, e = s.Create(ctx, a[1], bad)
		check(t, e, 400)
	}
}
func TestConcurrentRepliesOnlyOneRevisionWins(t *testing.T) {
	s, _, a := fixture(t)
	ctx := context.Background()
	v, e := s.Create(ctx, a[0], input(1))
	if e != nil {
		t.Fatal(e)
	}
	var id int64
	fmt.Sscan(v.ID, &id)
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 2; i <= 3; i++ {
		key := input(i).RequestKey
		wg.Go(func() {
			_, e := s.Reply(ctx, a[2], id, feedback.Reply{RequestKey: key, Revision: 1, Status: "resolved"})
			results <- e
		})
	}
	wg.Wait()
	close(results)
	successes, conflicts := 0, 0
	for e := range results {
		if e == nil {
			successes++
		} else {
			check(t, e, 409)
			conflicts++
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatal(successes, conflicts)
	}
}
func TestMergePreservesAuthorsAndInvalidatesPreview(t *testing.T) {
	s, p, a := fixture(t)
	ctx := context.Background()
	v, e := s.Create(ctx, a[0], input(1))
	if e != nil {
		t.Fatal(e)
	}
	var id int64
	fmt.Sscan(v.ID, &id)
	tx, e := p.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	before, e := s.MergeAccountTx(ctx, tx, a[0].ID, a[1].ID, false)
	tx.Rollback(ctx)
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.Reply(ctx, a[0], id, feedback.Reply{RequestKey: input(2).RequestKey, Revision: 1, Body: "更多信息"})
	if e != nil {
		t.Fatal(e)
	}
	tx, e = p.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	after, e := s.MergeAccountTx(ctx, tx, a[0].ID, a[1].ID, true)
	if e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	if string(before) == string(after) || !json.Valid(after) || len(after) == 0 || after[0] != '{' {
		t.Fatal("invalid fingerprint", string(before), string(after))
	}
	var owner, original, author string
	if e = p.QueryRow(ctx, "SELECT account_id::text,original_account_id::text,submitted_by::text FROM feedback_tickets WHERE id=$1", id).Scan(&owner, &original, &author); e != nil {
		t.Fatal(e)
	}
	if owner != a[1].ID || original != a[0].ID || author != a[0].ID {
		t.Fatal(owner, original, author)
	}
	_, e = s.Detail(ctx, a[0].ID, id)
	check(t, e, 404)
	d, e := s.Detail(ctx, a[1].ID, id)
	if e != nil || d.Ticket.Revision != 3 || d.Messages[0].AuthorName != a[0].Name {
		t.Fatal(d, e)
	}
	tx, _ = p.Begin(ctx)
	_, e = s.MergeAccountTx(ctx, tx, a[1].ID, a[2].ID, true)
	if e != nil {
		t.Fatal(e)
	}
	tx.Commit(ctx)
	p.QueryRow(ctx, "SELECT original_account_id::text FROM feedback_tickets WHERE id=$1", id).Scan(&original)
	if original != a[0].ID {
		t.Fatal(original)
	}
}

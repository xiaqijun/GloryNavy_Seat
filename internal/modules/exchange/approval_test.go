package exchange

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/platform/reviewqueue"
	"testing"
)

func TestApprovalExchangeSelfCancellationAndQueue(t *testing.T) {
	s, _, shop := rewardFixture(t)
	ctx := context.Background()
	id, e := s.ClaimReward(ctx, manager, quote(shop, 801))
	if e != nil {
		t.Fatal(e)
	}
	requestCancel(t, s, manager, id, 802)
	decision := OrderDecision{Version: 2, RequestKey: rewardKey(803), State: "cancelled", Note: "checked", UndeliveredConfirmed: true}
	if e = s.DecideOrder(ctx, manager, id, decision); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatalf("self cancellation: %v", e)
	}
	decision.State = "pending"
	if e = s.DecideOrder(ctx, manager, id, decision); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatalf("self rejection: %v", e)
	}
	s.Administrator = func(_ context.Context, u string) (bool, error) { return u == manager || u == member, nil }
	p, e := s.ApprovalQueue(ctx, manager, reviewqueue.Filter{View: "pending"}, reviewqueue.Position{}, 31)
	if e != nil || len(p.Items) != 0 || p.Counts["pending"] != 0 {
		t.Fatalf("self queue %+v %v", p, e)
	}
	if sorted, err := s.ApprovalQueue(ctx, member, reviewqueue.Filter{View: "pending", Sort: "id_asc"}, reviewqueue.Position{}, 31); err != nil || len(sorted.Items) != 1 {
		t.Fatalf("ID sort query: %+v %v", sorted, err)
	}
	p, e = s.ApprovalQueue(ctx, member, reviewqueue.Filter{View: "pending"}, reviewqueue.Position{}, 31)
	if e != nil || len(p.Items) != 1 || len(p.Items[0].Actions) != 2 {
		t.Fatalf("other reviewer %+v %v", p, e)
	}
	p, e = s.ApprovalQueue(ctx, member, reviewqueue.Filter{View: "pending", Corporation: "10"}, reviewqueue.Position{}, 31)
	if e != nil || len(p.Items) != 0 {
		t.Fatalf("exchange forged corp %+v %v", p, e)
	}
	decision.State = "cancelled"
	if e = s.DecideOrder(ctx, member, id, decision); e != nil {
		t.Fatal(e)
	}
	if e = s.DecideOrder(ctx, member, id, decision); e != nil {
		t.Fatalf("replay %v", e)
	}
	p, e = s.ApprovalQueue(ctx, member, reviewqueue.Filter{View: "history", Mine: true}, reviewqueue.Position{}, 31)
	if e != nil || len(p.Items) != 1 || p.Items[0].State != "cancelled" {
		t.Fatalf("history %+v %v", p, e)
	}
	s.Bindings = func(context.Context, pgx.Tx, []int64) ([]Binding, error) { return nil, nil }
	p, e = s.ApprovalQueue(ctx, member, reviewqueue.Filter{View: "history", ID: id}, reviewqueue.Position{}, 1)
	if e != nil || len(p.Items) != 0 || p.Counts["history"] != 0 {
		t.Fatalf("unbound recipient leaked %+v %v", p, e)
	}
	s.Administrator = func(context.Context, string) (bool, error) { return false, nil }
	if _, e = s.ApprovalQueue(ctx, member, reviewqueue.Filter{View: "history"}, reviewqueue.Position{}, 31); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatalf("revoked admin %v", e)
	}
}

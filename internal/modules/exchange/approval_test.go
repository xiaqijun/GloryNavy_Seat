package exchange

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/platform/reviewqueue"
	"testing"
)

type countingApprovalNames struct{ calls int }

func (n *countingApprovalNames) TypeNames(context.Context, []int64) (map[int64]eve.StaticTypeName, error) {
	n.calls++
	return map[int64]eve.StaticTypeName{34: {ID: 34, Name: "should-not-load"}}, nil
}

func TestApprovalDecorateDoesNotFanOutToSDE(t *testing.T) {
	names := &countingApprovalNames{}
	s := &Service{Names: names}
	payload, _ := json.Marshal(RewardOrder{ID: 1, TypeID: 34, Name: "Frozen name"})
	item := &reviewqueue.Item{Account: "member", State: "cancel_requested", Payload: payload}
	if err := s.ApprovalDecorate(context.Background(), "reviewer", item); err != nil {
		t.Fatal(err)
	}
	if names.calls != 0 || item.Title != "" || len(item.Actions) != 2 {
		t.Fatalf("indexed decorate fanned out: calls=%d title=%q actions=%v", names.calls, item.Title, item.Actions)
	}
}

type unavailableApprovalNames struct{}

func (unavailableApprovalNames) TypeNames(context.Context, []int64) (map[int64]eve.StaticTypeName, error) {
	return nil, errors.New("sde unavailable")
}

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

func TestApprovalQueueKeepsCountWhenOptionalNamesAreUnavailable(t *testing.T) {
	s, _, shop := rewardFixture(t)
	ctx := context.Background()
	id, err := s.ClaimReward(ctx, manager, quote(shop, 901))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Pool.Exec(ctx, `UPDATE exchange_redemptions SET reward_content=$2 WHERE id=$1`, id, []byte(`{"items":[{"type_id":"34","quantity":1}]}`)); err != nil {
		t.Fatal(err)
	}
	s.Names = unavailableApprovalNames{}
	s.Administrator = func(_ context.Context, u string) (bool, error) { return u == reviewer, nil }
	p, err := s.ApprovalQueue(ctx, reviewer, reviewqueue.Filter{View: "pending"}, reviewqueue.Position{}, 31)
	if err != nil || len(p.Items) != 1 || p.Counts["pending"] != 1 {
		t.Fatalf("optional name lookup made queue unavailable: %+v %v", p, err)
	}
}

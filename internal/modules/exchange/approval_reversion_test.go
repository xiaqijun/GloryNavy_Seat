package exchange

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/exchange/internal/store"
)

func TestRevertRejectedCancellationPreservesBalance(t *testing.T) {
	s, _, sh := rewardFixture(t)
	ctx := context.Background()
	c := quote(sh, 820)
	c.RecipientID = 4
	id, err := s.ClaimReward(ctx, member, c)
	if err != nil {
		t.Fatal(err)
	}
	requestCancel(t, s, member, id, 821)
	if err = s.DecideOrder(ctx, manager, id, OrderDecision{Version: 2, RequestKey: rewardKey(822), State: "pending", Note: "no"}); err != nil {
		t.Fatal(err)
	}
	input := OrderDecision{Version: 3, RequestKey: rewardKey(823), State: "revert_cancel_review", Note: "mistake"}
	if err = s.DecideOrder(ctx, member, id, input); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("self: %v", err)
	}
	if err = s.DecideOrder(ctx, manager, id, input); err != nil {
		t.Fatal(err)
	}
	if err = s.DecideOrder(ctx, manager, id, input); err != nil {
		t.Fatalf("replay: %v", err)
	}
	r, err := store.New(s.Pool).Redemption(ctx, id)
	if err != nil || r.State != "cancel_requested" {
		t.Fatalf("state: %+v %v", r, err)
	}
	balance, err := s.Shop(ctx, member, 0)
	if err != nil || balance.Reserved != 5 || balance.Available != 0 {
		t.Fatalf("balance: %+v %v", balance, err)
	}
	var refunds int
	if err = s.Pool.QueryRow(ctx, `SELECT count(*) FROM exchange_coin_ledger WHERE kind='refund'`).Scan(&refunds); err != nil || refunds != 0 {
		t.Fatalf("refunds %d: %v", refunds, err)
	}
	if err = s.DecideOrder(ctx, manager, id, OrderDecision{Version: r.Version, RequestKey: rewardKey(824), State: "cancelled", Note: "checked", UndeliveredConfirmed: true}); err != nil {
		t.Fatal(err)
	}
	r, err = store.New(s.Pool).Redemption(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	input.Version = r.Version
	input.RequestKey = rewardKey(825)
	if err = s.DecideOrder(ctx, manager, id, input); !errors.Is(err, ErrConflict) {
		t.Fatalf("refunded rollback: %v", err)
	}
}

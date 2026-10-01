package exchange

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/exchange/internal/store"
)

func requestCancel(t *testing.T, s *Service, user string, id int64, key int) {
	t.Helper()
	r, e := store.New(s.Pool).Redemption(context.Background(), id)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.DecideOrder(context.Background(), user, id, OrderDecision{Version: r.Version, RequestKey: rewardKey(key), State: "cancel_requested", Note: "不再需要"}); e != nil {
		t.Fatal(e)
	}
}
func installDelivery(t *testing.T, s *Service, id, issuer int64, status string) *eve.DeliveryContract {
	t.Helper()
	r, e := store.New(s.Pool).Redemption(context.Background(), id)
	if e != nil {
		t.Fatal(e)
	}
	c := &eve.DeliveryContract{ID: 900 + id, OwnerKind: "character", OwnerID: r.RecipientID, Title: r.SettlementReference, Type: "item_exchange", Status: status, IssuerID: issuer, AssigneeID: r.RecipientID, AcceptorID: r.RecipientID, Price: "0", Reward: "", Issued: r.CreatedAt.Time.Add(time.Millisecond), Completed: r.CreatedAt.Time.Add(2 * time.Millisecond).Format(time.RFC3339Nano), ItemsReady: true, Items: []eve.DeliveryItem{{TypeID: 34, Quantity: 10, Included: true}}}
	s.Contracts = func(_ context.Context, _ pgx.Tx, _ string, _ int64, reference string, _ time.Time) ([]eve.DeliveryContract, error) {
		if reference != r.SettlementReference {
			t.Fatalf("wrong lookup reference: %s", reference)
		}
		return []eve.DeliveryContract{*c}, nil
	}
	return c
}
func deliveryState(t *testing.T, s *Service, id int64, state, status string) {
	t.Helper()
	r, e := store.New(s.Pool).Redemption(context.Background(), id)
	if e != nil || r.State != state {
		t.Fatalf("order %s wanted %s: %v", r.State, state, e)
	}
	d, e := store.ReadDelivery(context.Background(), s.Pool, id)
	if e != nil || d.Status != status {
		t.Fatalf("delivery %+v expected %s: %v", d, status, e)
	}
}
func TestDeliveryStrictEvidenceAndAutomaticCompletion(t *testing.T) {
	s, _, sh := rewardFixture(t)
	ctx := context.Background()
	claim := quote(sh, 600)
	claim.RecipientID = 4
	id, e := s.ClaimReward(ctx, member, claim)
	if e != nil {
		t.Fatal(e)
	}
	c := installDelivery(t, s, id, 1, "outstanding")
	c.AcceptorID = 0
	if e = s.CheckDelivery(ctx, id); e != nil {
		t.Fatal(e)
	}
	deliveryState(t, s, id, "pending", "awaiting_acceptance")
	requestCancel(t, s, member, id, 601)
	decision := OrderDecision{Version: 2, RequestKey: rewardKey(602), State: "cancelled", Note: "checked", UndeliveredConfirmed: true}
	if e = s.DecideOrder(ctx, member, id, decision); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatal("owner refunded", e)
	}
	if e = s.DecideOrder(ctx, manager, id, decision); !errors.Is(e, ErrDeliveryPending) {
		t.Fatal("live contract refunded", e)
	}
	c.Status = "finished"
	c.AcceptorID = 4
	original := *c
	cases := []struct {
		name, status string
		change       func(*eve.DeliveryContract)
	}{
		{"items", "mismatch", func(c *eve.DeliveryContract) { c.Items = []eve.DeliveryItem{{TypeID: 34, Quantity: 9, Included: true}} }},
		{"requested items", "mismatch", func(c *eve.DeliveryContract) {
			c.Items = []eve.DeliveryItem{{TypeID: 34, Quantity: 10, Included: false}}
		}},
		{"unexpected reward", "mismatch", func(c *eve.DeliveryContract) { c.Reward = "1" }},
		{"missing price", "mismatch", func(c *eve.DeliveryContract) { c.Price = "" }},
		{"ISK", "mismatch", func(c *eve.DeliveryContract) { c.Price = "1" }},
		{"recipient", "mismatch", func(c *eve.DeliveryContract) { c.AcceptorID = 2 }},
		{"missing details", "waiting_items", func(c *eve.DeliveryContract) { c.ItemsReady = false }},
		{"unknown issuer", "issuer_unverified", func(c *eve.DeliveryContract) { c.IssuerID = 88 }},
		{"nonadmin issuer", "issuer_unverified", func(c *eve.DeliveryContract) { c.IssuerID = 4 }},
		{"reference", "mismatch", func(c *eve.DeliveryContract) { c.Title = "GNV-EX-100" }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			*c = original
			test.change(c)
			if e = s.CheckDelivery(ctx, id); e != nil {
				t.Fatal(e)
			}
			deliveryState(t, s, id, "cancel_requested", test.status)
		})
	}
	*c = original
	if e = s.CheckDelivery(ctx, id); e != nil {
		t.Fatal(e)
	}
	deliveryState(t, s, id, "fulfilled", "fulfilled")
	if e = s.CheckDelivery(ctx, id); e != nil {
		t.Fatal(e)
	}
	if e = s.DecideOrder(ctx, manager, id, decision); !errors.Is(e, ErrConflict) {
		t.Fatal("fulfilled refund", e)
	}
	var count int
	if e = s.Pool.QueryRow(ctx, `SELECT count(*) FROM exchange_shop_audit WHERE target_id=$1 AND kind='delivery'`, id).Scan(&count); e != nil || count != 1 {
		t.Fatal(count, e)
	}
}
func TestCancellationApprovalRejectionAndReplay(t *testing.T) {
	s, _, sh := rewardFixture(t)
	ctx := context.Background()
	c := quote(sh, 610)
	c.RecipientID = 4
	id, e := s.ClaimReward(ctx, member, c)
	if e != nil {
		t.Fatal(e)
	}
	requestCancel(t, s, member, id, 611)
	balance, e := s.Shop(ctx, member, 0)
	if e != nil || balance.Reserved != 5 || balance.Available != 0 {
		t.Fatal(balance, e)
	}
	reject := OrderDecision{Version: 2, RequestKey: rewardKey(612), State: "pending", Note: "已准备交付"}
	if e = s.DecideOrder(ctx, manager, id, reject); e != nil {
		t.Fatal(e)
	}
	requestCancel(t, s, member, id, 613)
	cancel := OrderDecision{Version: 4, RequestKey: rewardKey(614), State: "cancelled", Note: "游戏中核实没有交付"}
	if e = s.DecideOrder(ctx, manager, id, cancel); !errors.Is(e, ErrInvalid) {
		t.Fatal("attestation missing", e)
	}
	cancel.UndeliveredConfirmed = true
	for range 2 {
		if e = s.DecideOrder(ctx, manager, id, cancel); e != nil {
			t.Fatal(e)
		}
	}
	balance, e = s.Shop(ctx, member, 0)
	if e != nil || balance.Reserved != 0 || balance.Available != 5 || balance.Rewards[0].Stock != 2 {
		t.Fatal(balance, e)
	}
	var refunds int
	s.Pool.QueryRow(ctx, `SELECT count(*) FROM exchange_coin_ledger WHERE kind='refund'`).Scan(&refunds)
	if refunds != 1 {
		t.Fatal(refunds)
	}
}
func TestDeliveryDuplicateClaimAndCancelRace(t *testing.T) {
	s, _, sh := rewardFixture(t)
	ctx := context.Background()
	id, e := s.ClaimReward(ctx, manager, quote(sh, 620))
	if e != nil {
		t.Fatal(e)
	}
	c := installDelivery(t, s, id, 2, "finished")
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if e = eve.ClaimDeliveryTx(ctx, tx, c.ID, "welfare", 999); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	if e = s.CheckDelivery(ctx, id); e != nil {
		t.Fatal(e)
	}
	deliveryState(t, s, id, "pending", "contract_claimed")
	previous := *c
	previous.Status = "deleted"
	c.ID++
	s.Contracts = func(context.Context, pgx.Tx, string, int64, string, time.Time) ([]eve.DeliveryContract, error) {
		return []eve.DeliveryContract{previous, *c}, nil
	}
	requestCancel(t, s, manager, id, 621)
	var wg sync.WaitGroup
	wg.Add(2)
	errs := make(chan error, 2)
	go func() { defer wg.Done(); errs <- s.CheckDelivery(ctx, id) }()
	go func() {
		defer wg.Done()
		errs <- s.DecideOrder(ctx, reviewer, id, OrderDecision{Version: 2, RequestKey: rewardKey(622), State: "cancelled", Note: "checked", UndeliveredConfirmed: true})
	}()
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil && !errors.Is(e, ErrDeliveryPending) && !errors.Is(e, ErrConflict) {
			t.Fatal(e)
		}
	}
	deliveryState(t, s, id, "fulfilled", "fulfilled")
	assertShop(t, s, 10, 0, 5, 5, 1)
}
func TestDeliveryAmbiguousAndDeletedContracts(t *testing.T) {
	s, _, sh := rewardFixture(t)
	ctx := context.Background()
	id, e := s.ClaimReward(ctx, manager, quote(sh, 630))
	if e != nil {
		t.Fatal(e)
	}
	first := *installDelivery(t, s, id, 2, "outstanding")
	second := first
	second.ID++
	s.Contracts = func(context.Context, pgx.Tx, string, int64, string, time.Time) ([]eve.DeliveryContract, error) {
		return []eve.DeliveryContract{first, second}, nil
	}
	if e = s.CheckDelivery(ctx, id); e != nil {
		t.Fatal(e)
	}
	deliveryState(t, s, id, "pending", "multiple_contracts")
	second.Status = "deleted"
	first.Status = "finished"
	if e = s.CheckDelivery(ctx, id); e != nil {
		t.Fatal(e)
	}
	deliveryState(t, s, id, "fulfilled", "fulfilled")
}

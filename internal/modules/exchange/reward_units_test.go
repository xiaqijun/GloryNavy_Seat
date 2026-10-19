package exchange

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/exchange/internal/store"
)

func unitCount(n int32) *int32 { return &n }

func TestRewardUnitsReserveCancelAndSnapshot(t *testing.T) {
	s, _, sh := rewardFixture(t)
	ctx := context.Background()
	c := quote(sh, 8000)
	c.Units = unitCount(2)
	id, err := s.ClaimReward(ctx, manager, c)
	if err != nil {
		t.Fatal(err)
	}
	assertShop(t, s, 10, 10, 0, 0, 0)
	if again, err := s.ClaimReward(ctx, manager, c); err != nil || again != id {
		t.Fatal(again, err)
	}
	c.Units = unitCount(1)
	if _, err = s.ClaimReward(ctx, manager, c); !errors.Is(err, ErrConflict) {
		t.Fatal("changed quantity replay", err)
	}
	orders, err := s.RewardOrders(ctx, manager, false, 0)
	if err != nil || len(orders.Items) != 1 {
		t.Fatal(orders, err)
	}
	o := orders.Items[0]
	if o.Units != 2 || o.Quantity != 20 || o.Value != 802 || o.CoinsMinor != 10 || o.Content.Items[0].Quantity != 20 {
		t.Fatalf("wrong totals: %+v", o)
	}
	snapshots, err := s.ApprovalSnapshot(ctx)
	if err != nil || len(snapshots) != 1 {
		t.Fatal(snapshots, err)
	}
	var projected RewardOrder
	json.Unmarshal(snapshots[0].Payload, &projected)
	if projected.Units != 2 {
		t.Fatal("projection units", projected)
	}
	detail, err := s.ApprovalDetail(ctx, reviewer, id)
	if err != nil {
		t.Fatal(err)
	}
	json.Unmarshal(detail.Payload, &projected)
	if projected.Units != 2 || projected.Content.Items[0].Quantity != 20 {
		t.Fatal("detail totals", projected)
	}
	requestCancel(t, s, manager, id, 8001)
	assertShop(t, s, 10, 10, 0, 0, 0)
	cancel := OrderDecision{Version: 2, RequestKey: rewardKey(8002), State: "cancelled", Note: "checked", UndeliveredConfirmed: true}
	for range 2 {
		if err = s.DecideOrder(ctx, reviewer, id, cancel); err != nil {
			t.Fatal(err)
		}
	}
	assertShop(t, s, 10, 0, 0, 10, 2)
}

func TestRewardUnitsLegacyReplayAndValidation(t *testing.T) {
	s, _, sh := rewardFixture(t)
	ctx := context.Background()
	for i, n := range []int32{0, -1, 1000001, 3} {
		c := quote(sh, 8100+i)
		c.Units = unitCount(n)
		_, err := s.ClaimReward(ctx, manager, c)
		want := ErrInvalid
		if n == 3 {
			want = ErrRewardUnavailable
		}
		if !errors.Is(err, want) {
			t.Fatal(n, err)
		}
	}
	c := quote(sh, 8110)
	c.Units = unitCount(2)
	c.RecipientID = 4
	if _, err := s.ClaimReward(ctx, member, c); !errors.Is(err, ErrInsufficientCoinsMinor) {
		t.Fatal(err)
	}
	assertShop(t, s, 10, 0, 0, 10, 2)
	c = quote(sh, 8111)
	id, err := s.ClaimReward(ctx, manager, c)
	if err != nil {
		t.Fatal(err)
	}
	c.Units = unitCount(1)
	if again, err := s.ClaimReward(ctx, manager, c); err != nil || again != id {
		t.Fatal("legacy replay", again, err)
	}
	row, err := store.New(s.Pool).Redemption(ctx, id)
	if err != nil || row.Units != 1 {
		t.Fatal(row, err)
	}
}

func TestRewardUnitsMixedDeliveryAndFrozenCatalog(t *testing.T) {
	s, _, sh := rewardFixture(t)
	ctx := context.Background()
	// One pack: 100 fitted ships, 1M ordinary items and 123.45 ISK.
	raw := `{"fittings":[{"fitting_id":"8","ship_type_id":"34","quantity":100,"fit":{"ship_type_id":"34","items":[{"type_id":"35","quantity":2}]}}],"items":[{"type_id":"36","quantity":1000000}],"isk_minor":12345}`
	if _, err := s.Pool.Exec(ctx, `UPDATE exchange_rewards SET content=$1`, raw); err != nil {
		t.Fatal(err)
	}
	c := quote(sh, 8200)
	c.Units = unitCount(2)
	id, err := s.ClaimReward(ctx, manager, c)
	if err != nil {
		t.Fatal(err)
	}
	reward, _, _, _, err := s.SettlementReward(ctx, id)
	if err != nil || reward.Fittings[0].Quantity != 200 || reward.Items[0].Quantity != 2000000 || reward.ISKMinor != 24690 {
		t.Fatal(reward, err)
	}
	if _, err = s.Pool.Exec(ctx, `UPDATE exchange_rewards SET content='{}',catalog_version=catalog_version+1,stock=9,isk_value=999`); err != nil {
		t.Fatal(err)
	}
	contract := installDelivery(t, s, id, 2, "finished")
	contract.Items = []eve.DeliveryItem{{TypeID: 34, Quantity: 200, Included: true}, {TypeID: 35, Quantity: 400, Included: true}, {TypeID: 36, Quantity: 2000000, Included: true}}
	contract.Reward = "123" // One pack's cash must not fulfill two packs.
	if err = s.CheckDelivery(ctx, id); err != nil {
		t.Fatal(err)
	}
	deliveryState(t, s, id, "pending", "mismatch")
	contract.Reward = "246"
	for range 2 {
		if err = s.CheckDelivery(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	deliveryState(t, s, id, "fulfilled", "fulfilled")
	assertShop(t, s, 10, 0, 10, 0, 9)
}

func TestRewardUnitsCancelChangedCatalogAndRollback(t *testing.T) {
	for _, changed := range []bool{false, true} {
		s, _, sh := rewardFixture(t)
		ctx := context.Background()
		c := quote(sh, 8300)
		c.Units = unitCount(2)
		if !changed {
			if _, err := s.Pool.Exec(ctx, `CREATE FUNCTION reject_reserve() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'test'; END$$; CREATE TRIGGER reject_reserve BEFORE INSERT ON exchange_coin_ledger FOR EACH ROW EXECUTE FUNCTION reject_reserve()`); err != nil {
				t.Fatal(err)
			}
			if _, err := s.ClaimReward(ctx, manager, c); err == nil {
				t.Fatal("failure swallowed")
			}
			assertShop(t, s, 10, 0, 0, 10, 2)
			var count int
			s.Pool.QueryRow(ctx, `SELECT count(*) FROM exchange_redemptions`).Scan(&count)
			if count != 0 {
				t.Fatal("partial order", count)
			}
			continue
		}
		id, err := s.ClaimReward(ctx, manager, c)
		if err != nil {
			t.Fatal(err)
		}
		s.Pool.Exec(ctx, `UPDATE exchange_rewards SET catalog_version=catalog_version+1,stock=9`)
		requestCancel(t, s, manager, id, 8301)
		if err = s.DecideOrder(ctx, reviewer, id, OrderDecision{Version: 2, RequestKey: rewardKey(8302), State: "cancelled", Note: "checked", UndeliveredConfirmed: true}); err != nil {
			t.Fatal(err)
		}
		assertShop(t, s, 10, 0, 0, 10, 9)
	}
}

func TestRewardUnitsConcurrentStockAndBalance(t *testing.T) {
	for _, same := range []bool{true, false} {
		s, _, sh := rewardFixture(t)
		ctx := context.Background()
		if _, err := s.Pool.Exec(ctx, `UPDATE exchange_rewards SET stock=3,isk_value=101`); err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		errs := make(chan error, 2)
		for i := range 2 {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				c := quote(sh, 8400+i)
				c.Units = unitCount(2)
				user := manager
				if i == 1 && !same {
					user = member
					c.RecipientID = 4
				}
				_, err := s.ClaimReward(ctx, user, c)
				errs <- err
			}(i)
		}
		wg.Wait()
		close(errs)
		success := 0
		for err := range errs {
			if err == nil {
				success++
			} else if !errors.Is(err, ErrConflict) && !errors.Is(err, ErrRewardUnavailable) {
				t.Fatal(err)
			}
		}
		if success != 1 {
			t.Fatal("oversell", success)
		}
		var stock, orders int
		s.Pool.QueryRow(ctx, `SELECT stock FROM exchange_rewards`).Scan(&stock)
		s.Pool.QueryRow(ctx, `SELECT count(*) FROM exchange_redemptions`).Scan(&orders)
		if stock != 1 || orders != 1 {
			t.Fatal(stock, orders)
		}
	}
}

func TestRewardUnitsTotalLimits(t *testing.T) {
	if _, err := rewardTotal(maxRewardTotal, 2); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	for _, raw := range []string{`{"fittings":[],"items":[{"type_id":"34","quantity":1000000}]}`, `{"fittings":[],"items":[],"isk_minor":100000000000000}`} {
		if _, err := expandReward([]byte(raw), 1000000); !errors.Is(err, ErrInvalid) {
			t.Fatal("unsafe total", err)
		}
	}
}

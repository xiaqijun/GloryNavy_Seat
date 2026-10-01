package exchange

import (
	"context"
	"encoding/json"
	"errors"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/exchange/internal/store"
	"glorynavy.local/seat/internal/modules/market"
	"testing"
	"time"
)

func TestMixedRewardCashAndItems(t *testing.T) {
	ctx := context.Background()
	s := &Service{}
	cash := PhysicalReward{ISKMinor: 12345, Fittings: []PhysicalFitting{}, Items: []PhysicalItem{}}
	v, err := s.valueContent(ctx, cash)
	if err != nil || v.Value != 124 || v.Mid != "123.45" || !v.Complete {
		t.Fatal(v, err)
	}
	cash.Items = []PhysicalItem{{ID: 44992, Quantity: 2}}
	mid := "10.50"
	s.EstimateReward = func(_ context.Context, items []market.Item) (market.Appraisal, int64, error) {
		if len(items) != 1 || items[0].Quantity != 2 {
			t.Fatal(items)
		}
		return market.Appraisal{Complete: true, Totals: market.Amounts{Mid: mid}, Lines: []market.Line{{TypeID: 44992, Mid: &mid}}}, 1, nil
	}
	v, err = s.valueContent(ctx, cash)
	if err != nil || v.Mid != "133.95" || v.Value != 134 {
		t.Fatal(v, err)
	}
	raw, _ := json.Marshal(cash)
	c := eve.DeliveryContract{ItemsReady: true, Price: "0.00", Reward: "123.45", Items: []eve.DeliveryItem{{TypeID: 44992, Quantity: 2, Included: true}}}
	if ok, err := MatchRewardDelivery(raw, c); !ok || err != nil {
		t.Fatal(ok, err)
	}
	c.Reward = "123"
	if ok, err := MatchRewardDelivery(raw, c); !ok || err != nil {
		t.Fatal("whole ISK contract rejected", ok, err)
	}
	for _, mutate := range []func(*eve.DeliveryContract){func(c *eve.DeliveryContract) { c.Price = "123.45"; c.Reward = "0" }, func(c *eve.DeliveryContract) { c.Reward = "123.44" }, func(c *eve.DeliveryContract) { c.Items = nil }, func(c *eve.DeliveryContract) { c.ItemsReady = false }} {
		bad := c
		mutate(&bad)
		if ok, _ := MatchRewardDelivery(raw, bad); ok {
			t.Fatal("accepted incomplete/wrong delivery", bad)
		}
	}
}

func TestAutomaticRewardPricingPersistenceAndFences(t *testing.T) {
	s, _, sh := rewardFixture(t)
	ctx := context.Background()
	r := sh.Rewards[0]
	q := store.New(s.Pool)
	if err := q.SetRewardPricing(ctx, r.ID, true); err != nil {
		t.Fatal(err)
	}
	// Freeze an existing order before any price change.
	orderID, err := s.ClaimReward(ctx, manager, quote(sh, 901))
	if err != nil {
		t.Fatal(err)
	}
	mid := "1234.56"
	calls := 0
	s.EstimateReward = func(context.Context, []market.Item) (market.Appraisal, int64, error) {
		calls++
		return market.Appraisal{Complete: true, Totals: market.Amounts{Mid: mid}, Lines: []market.Line{{TypeID: 34, Mid: &mid}}}, 1, nil
	}
	if err = s.RefreshRewardPrice(ctx, r.ID); err != nil {
		t.Fatal(err)
	}
	prices, err := q.RewardPrices(ctx, []int64{r.ID})
	if err != nil {
		t.Fatal(err)
	}
	if p := prices[r.ID]; p.Status != "ready" || time.Until(p.NextAt) < 5*time.Hour || time.Until(p.NextAt) > 6*time.Hour+time.Minute {
		t.Fatal(p)
	}
	var value int64
	if err = s.Pool.QueryRow(ctx, `SELECT isk_value FROM exchange_rewards WHERE id=$1`, r.ID).Scan(&value); err != nil || value != 1235 {
		t.Fatal(value, err)
	}
	order, err := q.Redemption(ctx, orderID)
	if err != nil || order.IskValue != r.Value {
		t.Fatal(order, err)
	}
	if err = s.RefreshRewardPrice(ctx, r.ID); err != nil || calls != 1 {
		t.Fatal("duplicate/due guard", calls, err)
	}
	if err = q.SetRewardPricing(ctx, r.ID, true); err != nil {
		t.Fatal(err)
	}
	s.EstimateReward = func(context.Context, []market.Item) (market.Appraisal, int64, error) {
		return market.Appraisal{}, 0, errors.New("offline")
	}
	if err = s.RefreshRewardPrice(ctx, r.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.Pool.QueryRow(ctx, `SELECT isk_value FROM exchange_rewards WHERE id=$1`, r.ID).Scan(&value); err != nil || value != 1235 {
		t.Fatal("failed quote overwrote value", value, err)
	}
	if err = q.SetRewardPricing(ctx, r.ID, true); err != nil {
		t.Fatal(err)
	}
	s.EstimateReward = func(context.Context, []market.Item) (market.Appraisal, int64, error) {
		if err := q.SetRewardPricing(ctx, r.ID, false); err != nil {
			t.Fatal(err)
		}
		return market.Appraisal{Complete: true, Totals: market.Amounts{Mid: "1"}, Lines: []market.Line{{TypeID: 34, Mid: &mid}}}, 1, nil
	}
	if err = s.RefreshRewardPrice(ctx, r.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.Pool.QueryRow(ctx, `SELECT isk_value FROM exchange_rewards WHERE id=$1`, r.ID).Scan(&value); err != nil || value != 1235 {
		t.Fatal("late result overwrote manual mode", value, err)
	}
}

func TestCashCatalogRoundTrip(t *testing.T) {
	s, _, _ := rewardFixture(t)
	ctx := context.Background()
	id, err := s.SaveCatalog(ctx, manager, CatalogEdit{CatalogEntry: CatalogEntry{Name: "ISK", Content: PhysicalReward{ISKMinor: 12345}}, RequestKey: rewardKey(902)})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RefreshRewardPrice(ctx, id); err != nil {
		t.Fatal(err)
	}
	shop, err := s.Shop(ctx, manager, 0)
	if err != nil {
		t.Fatal(err)
	}
	var r Reward
	for _, entry := range shop.Rewards {
		if entry.ID == id {
			r = entry
		}
	}
	if r.TypeID != 0 || r.Content == nil || r.Content.ISKMinor != 12345 || r.Value != 124 {
		t.Fatal(r)
	}
	if err = s.EditShop(ctx, manager, "reward", ShopEdit{ID: id, Version: r.Version, RequestKey: rewardKey(903), TypeID: 0, Quantity: 1, Value: 124, Stock: 1, Enabled: true, AutomaticPricing: true}); err != nil {
		t.Fatal(err)
	}
	shop, err = s.Shop(ctx, manager, 0)
	if err != nil {
		t.Fatal(err)
	}
	claim := quote(shop, 904)
	claim.RewardID = id
	claim.RecipientID = 4
	for _, entry := range shop.Rewards {
		if entry.ID == id {
			claim.RewardVersion = entry.Version
		}
	}
	orderID, err := s.ClaimReward(ctx, member, claim)
	if err != nil {
		t.Fatal(err)
	}
	handoff, err := s.Handoff(ctx, manager, orderID)
	if err != nil || handoff.Amount != "123" {
		t.Fatal("contract amount must be whole ISK", handoff, err)
	}
}

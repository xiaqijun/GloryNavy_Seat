package exchange

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/market"
)

func TestRewardValuationComposition(t *testing.T) {
	content := PhysicalReward{Fittings: []PhysicalFitting{{ID: 1, ShipTypeID: 587, Quantity: 2, Fit: json.RawMessage(`{"ship_type_id":"587","items":[{"type_id":"34","quantity":3,"charge_id":"35"},{"type_id":"35","quantity":100}]}`)}}, Items: []PhysicalItem{{ID: 34, Quantity: 4}}}
	items, err := rewardMarketItems(content)
	if err != nil || len(items) != 3 || items[0].TypeID != 34 || items[0].Quantity != 10 || items[1].Quantity != 200 || items[2].Quantity != 2 {
		t.Fatal(items, err)
	}
	content.Fittings[0].Fit = json.RawMessage(`{"ship_type_id":"587","items":[{"type_id":"34","quantity":9223372036854775807}]}`)
	if _, err = rewardMarketItems(content); !errors.Is(err, ErrInvalid) {
		t.Fatal("overflow accepted", err)
	}
	content.Fittings[0].Fit = json.RawMessage(`{"ship_type_id":"588","items":[]}`)
	if _, err = rewardMarketItems(content); !errors.Is(err, ErrInvalid) {
		t.Fatal("wrong hull accepted", err)
	}
}

func TestRewardValuationGuardsAndRawMidpoint(t *testing.T) {
	s, _, shop := rewardFixture(t)
	r := shop.Rewards[0]
	ctx := context.Background()
	mid := "123.45"
	calls := 0
	s.EstimateReward = func(_ context.Context, items []market.Item) (market.Appraisal, int64, error) {
		calls++
		if len(items) != 1 || items[0].TypeID != 34 || items[0].Quantity != 10 {
			t.Fatal(items)
		}
		return market.Appraisal{Complete: true, Totals: market.Amounts{Mid: mid}, Adjusted: market.Amounts{Mid: "12.35"}, Lines: []market.Line{{TypeID: 34, Mid: &mid}}}, 7, nil
	}
	v, err := s.ValueReward(ctx, manager, r.ID, r.Version)
	if err != nil || !v.Complete || v.Value != 124 || v.Mid != "123.45" {
		t.Fatal(v, err)
	}
	if _, err = s.ValueReward(ctx, rewardKey(999), r.ID, r.Version); !errors.Is(err, pgx.ErrNoRows) || calls != 1 {
		t.Fatal("admin guard", err, calls)
	}
	if _, err = s.ValueReward(ctx, manager, r.ID, r.Version+1); !errors.Is(err, ErrConflict) || calls != 1 {
		t.Fatal("version guard", err, calls)
	}
	s.EstimateReward = func(context.Context, []market.Item) (market.Appraisal, int64, error) {
		return market.Appraisal{Totals: market.Amounts{Mid: "0.00"}, Lines: []market.Line{{TypeID: 34}}}, 1, nil
	}
	v, err = s.ValueReward(ctx, manager, r.ID, r.Version)
	if err != nil || v.Complete || v.Value != 0 || v.Missing != 1 {
		t.Fatal("partial estimate", v, err)
	}
	s.EstimateReward = func(context.Context, []market.Item) (market.Appraisal, int64, error) {
		_, err := s.Pool.Exec(ctx, "UPDATE exchange_rewards SET version=version+1 WHERE id=$1", r.ID)
		return market.Appraisal{}, 1, err
	}
	if _, err = s.ValueReward(ctx, manager, r.ID, r.Version); !errors.Is(err, ErrConflict) {
		t.Fatal("concurrent content edit", err)
	}
}

package market

import (
	"context"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/testutil"
	"testing"
	"time"
)

func TestIdentifiedItemsReusePolicyAndPriceEngine(t *testing.T) {
	pool := testutil.Database(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE market_settings SET ratio_bps=8000,version=7`); err != nil {
		t.Fatal(err)
	}
	calls := 0
	s := &Service{Pool: pool, Prices: func(_ context.Context, id int64) (eve.MarketPrices, error) {
		if id != 34 {
			t.Fatal("incorrect type", id)
		}
		calls++
		return eve.MarketPrices{Buy: str("4.01"), Sell: str("6.03")}, nil
	}}
	items := make([]Item, 101)
	for i := range items {
		items[i] = Item{TypeID: 34, Quantity: 1, Name: "display-only"}
	}
	v, version, err := s.EstimateItems(ctx, items)
	if err != nil || version != 7 || !v.Complete || calls != 1 || v.Totals.Mid != "507.02" || v.Adjusted.Mid != "405.62" {
		t.Fatal(v, version, calls, err)
	}
}

func str(s string) *string { return &s }
func TestAppraisalParsingPricesAndMissingData(t *testing.T) {
	calls := 0
	s := Service{Resolve: func(_ context.Context, n []string) (map[string]eve.StaticTypeName, error) {
		return map[string]eve.StaticTypeName{"三钛合金": {ID: 34, Name: "三钛合金"}, "tritanium": {ID: 34, Name: "三钛合金"}, "only buy": {ID: 35, Name: "Only buy"}}, nil
	}, Prices: func(_ context.Context, id int64) (eve.MarketPrices, error) {
		calls++
		if id == 35 {
			return eve.MarketPrices{Buy: str("2.00")}, nil
		}
		return eve.MarketPrices{Buy: str("4.01"), Sell: str("6.03"), ObservedAt: time.Now()}, nil
	}}
	v, err := s.Estimate(context.Background(), "三钛合金\t1,000\t矿物\nTritanium × 2\nOnly buy\n不存在物品\n三钛合金\t1,2", 8000)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || v.Complete || len(v.Lines) != 5 {
		t.Fatal("dedup or partial status", calls, v)
	}
	if v.Totals.Buy != "4020.02" || v.Totals.Mid != "5030.04" || v.Totals.Sell != "6042.06" || v.Adjusted.Mid != "4024.03" {
		t.Fatal(v.Totals, v.Adjusted)
	}
	if v.Lines[2].Mid != nil || v.Lines[3].Status != "unknown_type" || v.Lines[4].Status != "invalid_quantity" {
		t.Fatal(v.Lines)
	}
}
func TestParseRejectsInvalidLimits(t *testing.T) {
	for _, s := range []string{"", "\n"} {
		if _, err := parse(s); err == nil {
			t.Fatal("accepted empty")
		}
	}
	for _, s := range []string{"x\t-1", "x\t0", "x\t1.2", "x\t1000000001"} {
		v, err := parse(s)
		if err != nil || v[0].Status != "invalid_quantity" {
			t.Fatal(s, v, err)
		}
	}
}

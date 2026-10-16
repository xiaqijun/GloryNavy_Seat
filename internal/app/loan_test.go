package app

import (
	"math/big"
	"testing"
	"time"

	"glorynavy.local/seat/internal/modules/eve"
)

func TestValueLoanAssetsSkipsUnpricedHoldings(t *testing.T) {
	observed := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	priced := "125.50"
	invalid := "0"
	result := valueLoanAssets(
		map[int64]*big.Int{34: big.NewInt(2), 35: big.NewInt(3), 36: big.NewInt(4)},
		map[int64]eve.MarketPrices{
			34: {Mid: &priced, ObservedAt: observed},
			35: {Mid: &invalid, ObservedAt: observed},
		},
		time.Time{},
	)
	if result.pricedTypes != 1 || result.unpricedTypes != 2 {
		t.Fatalf("priced/unpriced=%d/%d, want 1/2", result.pricedTypes, result.unpricedTypes)
	}
	if result.total.Cmp(new(big.Rat).SetInt64(251)) != 0 {
		t.Fatalf("value=%s ISK, want 251", result.total.FloatString(2))
	}
	if !result.cutoff.Equal(observed) {
		t.Fatalf("cutoff=%s, want %s", result.cutoff, observed)
	}
}

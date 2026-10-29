package app

import (
	"math/big"
	"testing"
	"time"

	"glorynavy.local/seat/internal/modules/eve"
)

func TestLoanAssetPriceFreshnessUsesRevalidationTime(t *testing.T) {
	now := time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)
	price := "10"
	result := valueLoanAssets(map[int64]*big.Int{34: big.NewInt(1)}, map[int64]eve.MarketPrices{34: {Mid: &price, ObservedAt: now.Add(-10 * 24 * time.Hour), ValidatedAt: now}}, now)
	if !result.cutoff.Equal(now) {
		t.Fatalf("valid unchanged price was treated as stale: %v", result.cutoff)
	}
}

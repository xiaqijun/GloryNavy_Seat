package exchange

import (
	"encoding/json"

	"glorynavy.local/seat/internal/contractamount"
	"glorynavy.local/seat/internal/modules/eve"
)

// MatchRewardDelivery expands only the frozen reward snapshot. Shared by the
// exchange and host-injected welfare service; no current catalog reads occur.
func MatchRewardDelivery(snapshot json.RawMessage, c eve.DeliveryContract) (bool, error) {
	var content PhysicalReward
	if err := json.Unmarshal(snapshot, &content); err != nil {
		return false, err
	}
	want, err := rewardMarketItems(content)
	if err != nil {
		return false, err
	}
	if !c.ItemsReady || !rewardCashMatches(content.ISKMinor, c.Price, c.Reward) {
		return false, nil
	}
	counts := map[int64]int64{}
	for _, i := range c.Items {
		if !i.Included || i.TypeID <= 0 || i.Quantity <= 0 || i.Quantity > 1000000000 || counts[i.TypeID] > 1000000000-i.Quantity || (i.RawQuantity != nil && *i.RawQuantity == -2) {
			return false, nil
		}
		counts[i.TypeID] += i.Quantity
	}
	if len(counts) != len(want) {
		return false, nil
	}
	for _, i := range want {
		if counts[i.TypeID] != i.Quantity {
			return false, nil
		}
	}
	return true, nil
}

// ISK flows from issuer to recipient. Never net an incoming price against reward.
func rewardCashMatches(minor int64, price, reward string) bool {
	if minor < 0 || !zeroISK(price) {
		return false
	}
	if minor == 0 {
		return reward == "" || zeroISK(reward)
	}
	return contractamount.Matches(minor, reward)
}

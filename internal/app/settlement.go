package app

import (
	"encoding/json"
	"sort"

	"glorynavy.local/seat/internal/modules/exchange"
	"glorynavy.local/seat/internal/modules/fittings"
	"glorynavy.local/seat/internal/modules/market"
)

func exchangeRewardItems(content exchange.PhysicalReward) ([]market.Item, error) {
	counts := map[int64]int64{}
	add := func(id, quantity int64) error {
		if id <= 0 || quantity <= 0 || quantity > 1000000000 || counts[id] > 1000000000-quantity {
			return exchange.ErrInvalid
		}
		counts[id] += quantity
		return nil
	}
	for _, item := range content.Items {
		if err := add(item.ID, item.Quantity); err != nil {
			return nil, err
		}
	}
	for _, fitting := range content.Fittings {
		var fit fittings.Fit
		if len(fitting.Fit) == 0 || json.Unmarshal(fitting.Fit, &fit) != nil || fit.ShipTypeID != fitting.ShipTypeID {
			return nil, exchange.ErrInvalid
		}
		if err := add(fit.ShipTypeID, fitting.Quantity); err != nil {
			return nil, err
		}
		for _, item := range fit.Items {
			if err := add(item.TypeID, item.Quantity*fitting.Quantity); err != nil {
				return nil, err
			}
		}
	}
	out := make([]market.Item, 0, len(counts))
	for id, quantity := range counts {
		out = append(out, market.Item{TypeID: id, Quantity: quantity})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TypeID < out[j].TypeID })
	return out, nil
}

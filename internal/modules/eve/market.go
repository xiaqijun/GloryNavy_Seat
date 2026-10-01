package eve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
	"math/big"
	"strings"
	"time"
)

// ResolveTypeNames uses exact active SDE names; ambiguous aliases are not guessed.
func (s *StaticDataService) ResolveTypeNames(ctx context.Context, names []string) (map[string]StaticTypeName, error) {
	keys := []string{}
	for _, name := range names {
		keys = append(keys, strings.ToLower(strings.TrimSpace(name)))
	}
	rows, err := store.ResolveTypeNames(ctx, s.pool, keys)
	if err != nil {
		return nil, err
	}
	out := map[string]StaticTypeName{}
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	localized, err := s.TypeNames(ctx, ids)
	if err != nil {
		return nil, err
	}
	ambiguous := map[string]bool{}
	for _, r := range rows {
		if old, ok := out[r.Input]; ok && old.ID != r.ID {
			ambiguous[r.Input] = true
		}
		out[r.Input] = StaticTypeName{ID: r.ID, Name: r.Name, Source: "sde"}
		if n, ok := localized[r.ID]; ok {
			out[r.Input] = n
		}
	}
	for key := range ambiguous {
		delete(out, key)
	}
	return out, nil
}

type MarketPrices struct {
	Buy, Sell             *string
	ObservedAt, ExpiresAt time.Time
}
type marketOrder struct {
	TypeID     int64       `json:"type_id"`
	LocationID int64       `json:"location_id"`
	Buy        bool        `json:"is_buy_order"`
	Price      json.Number `json:"price"`
	Remaining  int64       `json:"volume_remain"`
}

// JitaPrices deliberately uses orders located at Jita 4-4. It is a top-of-book
// reference, not a depth-weighted fill quote or regional-range execution engine.
func (s *ESIService) JitaPrices(ctx context.Context, id int64) (MarketPrices, error) {
	out := MarketPrices{}
	var buy, sell *big.Rat
	pages := 0
	for page := 1; page <= max(1, pages); page++ {
		var orders []marketOrder
		var meta ESIResponse
		var err error
		for attempt := 0; attempt < 4; attempt++ {
			meta, err = s.Request(ctx, ESIRequest{Method: "GET", Path: fmt.Sprintf("/markets/10000002/orders/?order_type=all&type_id=%d&page=%d", id, page), ExpectPages: true}, &orders)
			var retry retryError
			if !errors.As(err, &retry) || time.Until(retry.Until) > 300*time.Millisecond {
				break
			}
			timer := time.NewTimer(max(time.Millisecond, time.Until(retry.Until)))
			select {
			case <-ctx.Done():
				timer.Stop()
				return MarketPrices{}, ctx.Err()
			case <-timer.C:
			}
		}
		if err != nil {
			return MarketPrices{}, err
		}
		if meta.Pages > 20 || pages != 0 && pages != meta.Pages {
			return MarketPrices{}, errors.New("market pagination changed")
		}
		pages = meta.Pages
		if out.ObservedAt.IsZero() || meta.ContentUpdatedAt.Before(out.ObservedAt) {
			out.ObservedAt = meta.ContentUpdatedAt
		}
		if out.ExpiresAt.IsZero() || meta.ExpiresAt.Before(out.ExpiresAt) {
			out.ExpiresAt = meta.ExpiresAt
		}
		for _, o := range orders {
			if o.TypeID != id || o.LocationID != 60003760 || o.Remaining <= 0 {
				continue
			}
			price, ok := new(big.Rat).SetString(o.Price.String())
			if !ok || price.Sign() <= 0 {
				return MarketPrices{}, errors.New("invalid market price")
			}
			if o.Buy {
				if buy == nil || price.Cmp(buy) > 0 {
					buy = price
				}
			} else if sell == nil || price.Cmp(sell) < 0 {
				sell = price
			}
		}
	}
	if buy != nil {
		v := buy.FloatString(2)
		out.Buy = &v
	}
	if sell != nil {
		v := sell.FloatString(2)
		out.Sell = &v
	}
	return out, nil
}

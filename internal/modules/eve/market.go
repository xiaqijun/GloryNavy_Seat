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
	Buy, Sell *string
	// Mid is an authoritative midpoint supplied by ESI when an item has no
	// usable order book. It is currently reserved for PLEX as a last resort.
	Mid                   *string
	ObservedAt, ExpiresAt time.Time
}
type marketOrder struct {
	TypeID     int64       `json:"type_id"`
	LocationID int64       `json:"location_id"`
	Buy        bool        `json:"is_buy_order"`
	Price      json.Number `json:"price"`
	Remaining  int64       `json:"volume_remain"`
}
type marketAveragePrice struct {
	TypeID       int64       `json:"type_id"`
	AveragePrice json.Number `json:"average_price"`
}

const (
	plexTypeID       int64 = 44992
	plexMarketRegion       = 19000001
	jitaMarketRegion       = 10000002
	jitaStation            = 60003760
)

// JitaPrices returns a top-of-book reference. Ordinary items use Jita 4-4;
// PLEX uses EVE's dedicated Global PLEX Market. It is not depth-weighted.
func (s *ESIService) JitaPrices(ctx context.Context, id int64) (MarketPrices, error) {
	out := MarketPrices{}
	var buy, sell *big.Rat
	regionID := jitaMarketRegion
	stationID := int64(jitaStation)
	if id == plexTypeID {
		// PLEX uses EVE's dedicated Global PLEX Market region rather than a
		// normal regional/Jita order book.
		regionID = plexMarketRegion
		stationID = 0
	}
	pages := 0
	for page := 1; page <= max(1, pages); page++ {
		var orders []marketOrder
		var meta ESIResponse
		var err error
		for attempt := 0; attempt < 4; attempt++ {
			meta, err = s.Request(ctx, ESIRequest{Method: "GET", Path: fmt.Sprintf("/markets/%d/orders/?order_type=all&type_id=%d&page=%d", regionID, id, page), ExpectPages: true}, &orders)
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
			if o.TypeID != id || (stationID != 0 && o.LocationID != stationID) || o.Remaining <= 0 {
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
	// PLEX occasionally has no Global PLEX Market orders at all. ESI still
	// publishes an official average price for it, which is suitable as a last
	// resort midpoint. Keep the ordinary two-sided order requirement otherwise.
	if id == plexTypeID && (out.Buy == nil || out.Sell == nil) {
		var rows []marketAveragePrice
		var meta ESIResponse
		var err error
		for attempt := 0; attempt < 4; attempt++ {
			meta, err = s.Request(ctx, ESIRequest{Method: "GET", Path: "/markets/prices/"}, &rows)
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
		if meta.ContentUpdatedAt.Before(out.ObservedAt) || out.ObservedAt.IsZero() {
			out.ObservedAt = meta.ContentUpdatedAt
		}
		if meta.ExpiresAt.Before(out.ExpiresAt) || out.ExpiresAt.IsZero() {
			out.ExpiresAt = meta.ExpiresAt
		}
		for _, row := range rows {
			if row.TypeID != id || row.AveragePrice.String() == "" {
				continue
			}
			price, ok := new(big.Rat).SetString(row.AveragePrice.String())
			if !ok || price.Sign() <= 0 {
				return MarketPrices{}, errors.New("invalid market average price")
			}
			v := price.FloatString(2)
			out.Mid = &v
			break
		}
	}
	return out, nil
}

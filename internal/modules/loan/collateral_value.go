package loan

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"strconv"
)

var errCollateralItems = errors.New("collateral item snapshot is incomplete")

// CollateralPriceLookup returns authoritative ISK prices for the type IDs in a
// completed item-exchange contract. The loan module owns the traversal and
// validation; the host supplies the ESI/SDE-backed price source.
type CollateralPriceLookup func(context.Context, []int64) (map[int64]string, error)

type collateralItem struct {
	RecordID json.RawMessage  `json:"record_id"`
	TypeID   json.RawMessage  `json:"type_id"`
	Quantity json.RawMessage  `json:"quantity"`
	Included bool             `json:"included"`
	Items    []collateralItem `json:"items"`
}

func rawInt64(raw json.RawMessage) (int64, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, false
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		if n, err := strconv.ParseInt(text, 10, 64); err == nil {
			return n, true
		}
		return 0, false
	}
	var n int64
	if json.Unmarshal(raw, &n) == nil {
		return n, true
	}
	return 0, false
}

func flattenCollateralItems(raw json.RawMessage) ([]collateralItem, error) {
	var items []collateralItem
	if json.Unmarshal(raw, &items) == nil {
		return items, nil
	}
	var wrapper struct {
		Items []collateralItem `json:"items"`
	}
	if json.Unmarshal(raw, &wrapper) != nil || wrapper.Items == nil {
		return nil, errCollateralItems
	}
	return wrapper.Items, nil
}

// ValueCollateralItems values the included side of a completed item contract.
// Included parent and child rows are both counted, while a repeated record ID
// is counted once. Any included row without a valid quantity or market price
// invalidates the whole snapshot; partial valuation is unsafe for collateral.
func ValueCollateralItems(ctx context.Context, raw json.RawMessage, lookup CollateralPriceLookup) (int64, error) {
	if len(raw) == 0 || lookup == nil {
		return 0, errCollateralItems
	}
	items, err := flattenCollateralItems(raw)
	if err != nil {
		return 0, err
	}
	ids := map[int64]struct{}{}
	seenRecords := map[int64]struct{}{}
	var walk func([]collateralItem) error
	walk = func(rows []collateralItem) error {
		for _, item := range rows {
			if item.Included {
				typeID, typeOK := rawInt64(item.TypeID)
				quantity, quantityOK := rawInt64(item.Quantity)
				if !typeOK || typeID <= 0 || !quantityOK || quantity <= 0 {
					return errCollateralItems
				}
				if recordID, ok := rawInt64(item.RecordID); ok && recordID > 0 {
					if _, exists := seenRecords[recordID]; exists {
						if err := walk(item.Items); err != nil {
							return err
						}
						continue
					}
					seenRecords[recordID] = struct{}{}
				}
				ids[typeID] = struct{}{}
			}
			if err := walk(item.Items); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(items); err != nil || len(ids) == 0 {
		return 0, errCollateralItems
	}
	ordered := make([]int64, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}
	prices, err := lookup(ctx, ordered)
	if err != nil {
		return 0, err
	}
	// Re-walk to preserve quantities and apply the same duplicate-record rule.
	seenRecords = map[int64]struct{}{}
	total := new(big.Rat)
	var sum func([]collateralItem) error
	sum = func(rows []collateralItem) error {
		for _, item := range rows {
			if item.Included {
				typeID, _ := rawInt64(item.TypeID)
				quantity, _ := rawInt64(item.Quantity)
				if recordID, ok := rawInt64(item.RecordID); ok && recordID > 0 {
					if _, exists := seenRecords[recordID]; exists {
						if err := sum(item.Items); err != nil {
							return err
						}
						continue
					}
					seenRecords[recordID] = struct{}{}
				}
				price, ok := prices[typeID]
				if !ok {
					return errCollateralItems
				}
				unit, ok := new(big.Rat).SetString(price)
				if !ok || unit.Sign() <= 0 {
					return errCollateralItems
				}
				total.Add(total, new(big.Rat).Mul(unit, new(big.Rat).SetInt64(quantity)))
			}
			if err := sum(item.Items); err != nil {
				return err
			}
		}
		return nil
	}
	if err := sum(items); err != nil {
		return 0, err
	}
	minor := new(big.Rat).Mul(total, big.NewRat(100, 1))
	value := new(big.Int).Quo(minor.Num(), minor.Denom())
	if !value.IsInt64() || value.Sign() <= 0 {
		return 0, errCollateralItems
	}
	return value.Int64(), nil
}

func collateralQuantities(raw json.RawMessage) (map[int64]int64, error) {
	items, err := flattenCollateralItems(raw)
	if err != nil {
		return nil, err
	}
	out := map[int64]int64{}
	seen := map[int64]struct{}{}
	var walk func([]collateralItem) error
	walk = func(rows []collateralItem) error {
		for _, item := range rows {
			if item.Included {
				typeID, typeOK := rawInt64(item.TypeID)
				quantity, quantityOK := rawInt64(item.Quantity)
				if !typeOK || typeID <= 0 || !quantityOK || quantity <= 0 {
					return errCollateralItems
				}
				if recordID, ok := rawInt64(item.RecordID); ok && recordID > 0 {
					if _, exists := seen[recordID]; exists {
						if err := walk(item.Items); err != nil {
							return err
						}
						continue
					}
					seen[recordID] = struct{}{}
				}
				if out[typeID] > math.MaxInt64-quantity {
					return errCollateralItems
				}
				out[typeID] += quantity
			}
			if err := walk(item.Items); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(items); err != nil {
		return nil, err
	}
	return out, nil
}

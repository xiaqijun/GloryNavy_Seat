package eve

import (
	"context"
	"strconv"
	"strings"
	"time"

	"glorynavy.local/seat/internal/modules/eve/internal/store"
	"glorynavy.local/seat/internal/platform/locale"
)

// Generate optional presentation summaries only after owner authorization.
// Original descriptions and synchronized payloads remain unchanged.
func (h *ContractHTTP) summaries(parent context.Context, kind string, owner int64, contracts []contractView) {
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	ids := []int64{}
	for i := range contracts {
		c := &contracts[i]
		c.TradeDirection = "unknown"
		if c.Type == "courier" {
			c.TradeDirection = "transport"
			if strings.TrimSpace(c.Title) != "" {
				continue
			}
			place := func(e contractEntity) string {
				if e.Name != "" {
					return e.Name
				}
				if e.ID != "" && e.ID != "0" {
					return "#" + e.ID
				}
				return ""
			}
			start, end := place(c.Start), place(c.End)
			if start != "" && end != "" {
				c.Summary = start + " → " + end
			}
			continue
		}
		if c.Type != "item_exchange" && c.Type != "auction" {
			continue
		}
		id, _ := strconv.ParseInt(c.ID, 10, 64)
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return
	}
	rows, err := store.New(h.pool).ReadContractSummaryItems(ctx, store.ReadContractSummaryItemsParams{OwnerKind: kind, OwnerID: owner, ContractIds: ids})
	if err != nil {
		return
	}
	typeIDs := []int64{}
	for _, r := range rows {
		typeIDs = append(typeIDs, r.TypeID)
	}
	names := map[int64]StaticTypeName{}
	if h.StaticData != nil {
		// Name lookup is optional; it must not prevent direction classification.
		if resolved, lookupErr := h.StaticData.TypeNames(ctx, typeIDs); lookupErr == nil {
			names = resolved
		}
	}
	type pair struct{ provided, requested string }
	parts := map[int64]pair{}
	for _, r := range rows {
		name := "#" + strconv.FormatInt(r.TypeID, 10)
		if n, ok := names[r.TypeID]; ok && n.Name != "" {
			name = n.Name
		}
		if r.TypeCount > 1 {
			name += locale.Choose(ctx, "等 ", " and others · ") + strconv.FormatInt(r.TypeCount, 10) + locale.Choose(ctx, " 种物品", " item types")
		} else {
			name += " × " + r.Quantity
		}
		p := parts[r.ContractID]
		if r.IsIncluded {
			p.provided = name
		} else {
			p.requested = name
		}
		parts[r.ContractID] = p
	}
	for i := range contracts {
		c := &contracts[i]
		if c.Type != "item_exchange" && c.Type != "auction" {
			continue
		}
		id, _ := strconv.ParseInt(c.ID, 10, 64)
		p := parts[id]
		summary := ""
		switch {
		case p.provided != "" && p.requested != "":
			c.TradeDirection = "exchange"
			summary = locale.Choose(ctx, "提供 ", "Offer ") + p.provided + locale.Choose(ctx, " · 换取 ", " · Receive ") + p.requested
		case p.provided != "":
			c.TradeDirection = "sell"
			summary = p.provided
		case p.requested != "":
			c.TradeDirection = "buy"
			summary = p.requested
		}
		if strings.TrimSpace(c.Title) == "" {
			c.Summary = summary
		}
	}
}

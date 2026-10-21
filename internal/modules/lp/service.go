// Package lp is a read-only NPC loyalty store and market comparison tool.
package lp

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/platform/locale"
)

var ErrInput = errors.New("invalid LP query")
var ErrUnavailable = errors.New("LP data unavailable")
var ErrBusy = errors.New("LP prices busy")

type Service struct {
	Reference func(context.Context) (*eve.LPReference, error)
	Request   func(context.Context, eve.ESIRequest, any) (eve.ESIResponse, error)
	Names     func(context.Context, []int64) (map[int64]eve.StaticTypeName, error)
	Market    *MarketClient
}
type Offer struct {
	ID        int64            `json:"offer_id"`
	TypeID    int64            `json:"type_id"`
	Quantity  int64            `json:"quantity"`
	LP        int64            `json:"lp_cost"`
	AK        int64            `json:"ak_cost"`
	ISK       float64          `json:"isk_cost"`
	Required  []eve.LPMaterial `json:"required_items"`
	Blueprint *eve.LPBlueprint `json:"blueprint,omitempty"`
}
type Name struct {
	Chinese string `json:"zh"`
	English string `json:"en"`
}
type Offers struct {
	Corporation int64          `json:"corporation_id"`
	Build       int64          `json:"sde_build"`
	Offers      []Offer        `json:"offers"`
	Names       map[int64]Name `json:"names"`
	ObservedAt  time.Time      `json:"observed_at"`
}

func (s *Service) Offers(ctx context.Context, corporation int64) (Offers, error) {
	out := Offers{Corporation: corporation, Offers: []Offer{}, Names: map[int64]Name{}}
	ref, err := s.Reference(ctx)
	if err != nil {
		return out, err
	}
	valid := false
	for _, c := range ref.Corporations {
		if c.ID == corporation {
			valid = true
			break
		}
	}
	if !valid {
		return out, ErrInput
	}
	if s.Request == nil {
		return out, ErrUnavailable
	}
	var rows []Offer
	meta, err := s.Request(ctx, eve.ESIRequest{Method: "GET", Path: fmt.Sprintf("/loyalty/stores/%d/offers/", corporation), MaxResponseBytes: 8 << 20}, &rows)
	if err != nil {
		return out, err
	}
	if rows == nil || len(rows) > 10000 {
		return out, ErrUnavailable
	}
	ids := map[int64]bool{}
	seen := map[int64]bool{}
	for _, row := range rows {
		if row.ID <= 0 || seen[row.ID] || row.TypeID <= 0 || row.Quantity < 1 || row.Quantity > 1000000000 || row.LP < 0 || row.AK < 0 || !finite(row.ISK) || len(row.Required) > 1000 {
			return out, ErrUnavailable
		}
		row.Blueprint = nil
		seen[row.ID] = true
		ids[row.TypeID] = true
		if row.Required == nil {
			row.Required = []eve.LPMaterial{}
		}
		for _, item := range row.Required {
			if item.TypeID <= 0 || item.Quantity <= 0 || item.Quantity > 1000000000 {
				return out, ErrUnavailable
			}
			ids[item.TypeID] = true
		}
		if b, ok := ref.Blueprints[row.TypeID]; ok {
			row.Blueprint = &b
			if b.Product.TypeID > 0 {
				ids[b.Product.TypeID] = true
			}
			for _, item := range b.Materials {
				ids[item.TypeID] = true
			}
		}
		out.Offers = append(out.Offers, row)
	}
	keys := make([]int64, 0, len(ids))
	for id := range ids {
		keys = append(keys, id)
	}
	for _, lang := range []string{"zh-CN", "en"} {
		names, e := s.Names(locale.With(ctx, lang), keys)
		if e != nil {
			return out, e
		}
		for id, item := range names {
			n := out.Names[id]
			if lang == "en" {
				n.English = item.Name
			} else {
				n.Chinese = item.Name
			}
			out.Names[id] = n
		}
	}
	out.Build, out.ObservedAt = ref.Build, meta.ContentUpdatedAt
	if out.ObservedAt.IsZero() {
		out.ObservedAt = meta.ValidatedAt
	}
	return out, nil
}
func finite(n float64) bool { return !math.IsNaN(n) && !math.IsInf(n, 0) && n >= 0 && n <= 9e15 }

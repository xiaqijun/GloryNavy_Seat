package market

import (
	"context"
	"errors"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/market/internal/store"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Line struct {
	Input      string     `json:"input"`
	Name       string     `json:"name"`
	Quantity   int64      `json:"quantity,string"`
	TypeID     int64      `json:"type_id,string"`
	Status     string     `json:"status"`
	Buy        *string    `json:"buy"`
	Mid        *string    `json:"mid"`
	Sell       *string    `json:"sell"`
	ObservedAt *time.Time `json:"observed_at"`
}
type Amounts struct {
	Buy  string `json:"buy"`
	Mid  string `json:"mid"`
	Sell string `json:"sell"`
}
type Appraisal struct {
	Lines    []Line  `json:"lines"`
	Totals   Amounts `json:"totals"`
	Adjusted Amounts `json:"adjusted"`
	RatioBPS int     `json:"ratio_bps"`
	Complete bool    `json:"complete"`
}

// Item is an identified inventory entry supplied by a trusted business service.
type Item struct {
	TypeID   int64
	Quantity int64
	Name     string
}

// EstimateItems shares the same quote engine and configured ratio as pasted lists.
// Names are display-only; prices always use the original EVE type ID.
func (s *Service) EstimateItems(ctx context.Context, items []Item) (Appraisal, int64, error) {
	if len(items) == 0 || len(items) > 2001 {
		return Appraisal{}, 0, errors.New("invalid appraisal items")
	}
	settings, err := store.Read(ctx, s.Pool)
	if err != nil {
		return Appraisal{}, 0, err
	}
	lines := make([]Line, 0, len(items))
	for _, i := range items {
		if i.TypeID <= 0 || i.Quantity <= 0 || i.Quantity > 1000000000 {
			return Appraisal{}, 0, errors.New("invalid appraisal item")
		}
		name := i.Name
		if name == "" {
			name = strconv.FormatInt(i.TypeID, 10)
		}
		lines = append(lines, Line{Input: name, Name: name, TypeID: i.TypeID, Quantity: i.Quantity, Status: "pending"})
	}
	v, err := s.estimateLines(ctx, lines, settings.RatioBPS)
	return v, settings.Version, err
}

var suffixQuantity = regexp.MustCompile(`^(.+?)\s+[x×]\s*([0-9,]+)$`)
var quantitySyntax = regexp.MustCompile(`^(?:[0-9]+|[0-9]{1,3}(?:,[0-9]{3})+)$`)

func parse(text string) ([]Line, error) {
	if len(text) > 32768 {
		return nil, errors.New("清单过长，最多 100 行")
	}
	lines := []Line{}
	for _, raw := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		v := Line{Input: raw, Name: raw, Quantity: 1, Status: "pending"}
		quantity := "1"
		if cols := strings.Split(raw, "\t"); len(cols) > 1 {
			v.Name = strings.TrimSpace(cols[0])
			quantity = strings.TrimSpace(cols[1])
		} else if match := suffixQuantity.FindStringSubmatch(raw); match != nil {
			v.Name = strings.TrimSpace(match[1])
			quantity = match[2]
		}
		q, err := strconv.ParseInt(strings.ReplaceAll(quantity, ",", ""), 10, 64)
		if !quantitySyntax.MatchString(quantity) || err != nil || q <= 0 || q > 1000000000 || v.Name == "" {
			v.Status = "invalid_quantity"
			v.Quantity = 0
		} else {
			v.Quantity = q
		}
		lines = append(lines, v)
	}
	if len(lines) == 0 || len(lines) > 100 {
		return nil, errors.New("请输入 1–100 行物品")
	}
	return lines, nil
}
func amount(v *string, quantity int64) *big.Rat {
	if v == nil {
		return nil
	}
	r, ok := new(big.Rat).SetString(*v)
	if !ok || r.Sign() < 0 {
		return nil
	}
	return r.Mul(r, new(big.Rat).SetInt64(quantity))
}
func stringAmount(v *big.Rat) *string {
	if v == nil {
		return nil
	}
	s := v.FloatString(2)
	return &s
}
func (s *Service) Estimate(ctx context.Context, text string, ratio int) (Appraisal, error) {
	lines, err := parse(text)
	if err != nil {
		return Appraisal{}, err
	}
	names := []string{}
	for _, l := range lines {
		if l.Status == "pending" {
			names = append(names, l.Name)
		}
	}
	types, err := s.Resolve(ctx, names)
	if err != nil {
		return Appraisal{}, err
	}
	for i := range lines {
		v := &lines[i]
		if v.Status != "pending" {
			continue
		}
		typ, ok := types[strings.ToLower(v.Name)]
		if !ok {
			v.Status = "unknown_type"
			continue
		}
		v.TypeID, v.Name = typ.ID, typ.Name
	}
	return s.estimateLines(ctx, lines, ratio)
}

func (s *Service) estimateLines(ctx context.Context, lines []Line, ratio int) (Appraisal, error) {
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	prices := map[int64]eve.MarketPrices{}
	failures := map[int64]bool{}
	totals := [3]*big.Rat{new(big.Rat), new(big.Rat), new(big.Rat)}
	complete := true
	for i := range lines {
		v := &lines[i]
		if v.Status != "pending" {
			complete = false
			continue
		}
		p, cached := prices[v.TypeID]
		if !cached && !failures[v.TypeID] {
			if ctx.Err() != nil || s.Prices == nil {
				failures[v.TypeID] = true
			} else {
				var err error
				p, err = s.Prices(ctx, v.TypeID)
				if err != nil {
					failures[v.TypeID] = true
				} else {
					prices[v.TypeID] = p
				}
			}
		}
		if failures[v.TypeID] {
			v.Status = "unavailable"
			complete = false
			continue
		}
		buy, sell := amount(p.Buy, v.Quantity), amount(p.Sell, v.Quantity)
		var mid *big.Rat
		if p.Mid != nil {
			mid = amount(p.Mid, v.Quantity)
		} else if buy != nil && sell != nil {
			mid = new(big.Rat).Quo(new(big.Rat).Add(buy, sell), big.NewRat(2, 1))
		}
		v.Buy = stringAmount(buy)
		v.Mid = stringAmount(mid)
		v.Sell = stringAmount(sell)
		if !p.ObservedAt.IsZero() {
			at := p.ObservedAt
			v.ObservedAt = &at
		}
		v.Status = "ready"
		if mid == nil {
			v.Status = "missing_orders"
			complete = false
		}
		for j, value := range []*big.Rat{buy, mid, sell} {
			if value != nil {
				totals[j].Add(totals[j], value)
			}
		}
	}
	raw := [3]string{}
	adjusted := [3]string{}
	for i, v := range totals {
		raw[i] = v.FloatString(2)
		adjusted[i] = new(big.Rat).Mul(v, big.NewRat(int64(ratio), 10000)).FloatString(2)
	}
	return Appraisal{Lines: lines, Totals: Amounts{raw[0], raw[1], raw[2]}, Adjusted: Amounts{adjusted[0], adjusted[1], adjusted[2]}, RatioBPS: ratio, Complete: complete}, nil
}

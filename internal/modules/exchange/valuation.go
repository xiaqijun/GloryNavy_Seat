package exchange

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"sort"
	"time"

	"github.com/go-chi/chi/v5"
	"glorynavy.local/seat/internal/modules/exchange/internal/store"
	"glorynavy.local/seat/internal/modules/fittings"
	"glorynavy.local/seat/internal/modules/market"
)

type RewardValuation struct {
	Version    int64      `json:"version,string"`
	Value      int64      `json:"isk_value"`
	Mid        string     `json:"mid"`
	Complete   bool       `json:"complete"`
	Missing    int        `json:"missing_types"`
	ObservedAt *time.Time `json:"observed_at"`
}

// Price the saved reward snapshot, not the current fitting library or browser contents.
func rewardMarketItems(content PhysicalReward) ([]market.Item, error) {
	if !physicalValid(content) {
		return nil, ErrInvalid
	}
	counts := map[int64]int64{}
	add := func(id, quantity, packs int64) bool {
		if id <= 0 || quantity <= 0 || packs <= 0 || quantity > 1000000000/packs {
			return false
		}
		amount := quantity * packs
		if counts[id] > 1000000000-amount {
			return false
		}
		counts[id] += amount
		return true
	}
	for _, item := range content.Items {
		if !add(item.ID, item.Quantity, 1) {
			return nil, ErrInvalid
		}
	}
	for _, entry := range content.Fittings {
		var fit fittings.Fit
		if json.Unmarshal(entry.Fit, &fit) != nil || fit.ShipTypeID != entry.ShipTypeID || fit.Items == nil || !add(fit.ShipTypeID, 1, entry.Quantity) {
			return nil, ErrInvalid
		}
		for _, item := range fit.Items {
			if !add(item.TypeID, item.Quantity, entry.Quantity) {
				return nil, ErrInvalid
			}
		}
		// Loaded charge IDs describe a type, not an amount. Imported EFT requires
		// explicit ammunition quantities in cargo, already included in Items.
	}
	if len(counts) > 2001 {
		return nil, ErrInvalid
	}
	out := make([]market.Item, 0, len(counts))
	for id, quantity := range counts {
		out = append(out, market.Item{TypeID: id, Quantity: quantity})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TypeID < out[j].TypeID })
	return out, nil
}

func (s *Service) ValueReward(ctx context.Context, user string, id, version int64) (RewardValuation, error) {
	out := RewardValuation{Version: version}
	if err := s.shopAdmin(ctx, user); err != nil {
		return out, err
	}
	if id <= 0 || version <= 0 {
		return out, ErrInvalid
	}
	q := store.New(s.Pool)
	r, err := q.ReadReward(ctx, id)
	if err != nil {
		return out, err
	}
	if r.Archived || r.Version != version {
		return out, ErrConflict
	}
	var content PhysicalReward
	if json.Unmarshal(r.Content, &content) != nil {
		return out, ErrInvalid
	}
	out, err = s.valueContent(ctx, content)
	out.Version = version
	if err != nil {
		return out, err
	}
	// Network work must not hold business locks. Recheck permission and version.
	if err := s.shopAdmin(ctx, user); err != nil {
		return out, err
	}
	current, err := q.ReadReward(ctx, id)
	if err != nil {
		return out, err
	}
	if current.Archived || current.Version != version {
		return out, ErrConflict
	}
	return out, nil
}

// No locks or user identity are held during public market requests.
func (s *Service) valueContent(ctx context.Context, content PhysicalReward) (RewardValuation, error) {
	out := RewardValuation{}
	items, err := rewardMarketItems(content)
	if err != nil {
		return out, err
	}
	appraisal := market.Appraisal{Complete: true, Totals: market.Amounts{Mid: "0.00"}}
	if len(items) > 0 {
		if s.EstimateReward == nil {
			return out, ErrUnavailable
		}
		appraisal, _, err = s.EstimateReward(ctx, items)
		if err != nil {
			return out, err
		}
	}
	total := appraisal.Totals.Mid
	if total == "" && !appraisal.Complete {
		total = "0.00"
	}
	amount, valid := new(big.Rat).SetString(total)
	if !valid || amount.Sign() < 0 {
		return out, ErrInvalid
	}
	amount.Add(amount, big.NewRat(content.ISKMinor, 100))
	out.Mid = amount.FloatString(2) // Raw Jita midpoint plus face-value ISK.
	out.Complete = appraisal.Complete && len(appraisal.Lines) == len(items)
	for _, line := range appraisal.Lines {
		if line.Mid == nil {
			out.Missing++
			out.Complete = false
		}
		if line.ObservedAt != nil && (out.ObservedAt == nil || line.ObservedAt.Before(*out.ObservedAt)) {
			at := *line.ObservedAt
			out.ObservedAt = &at
		}
	}
	if out.Complete {
		if amount.Sign() <= 0 || amount.Cmp(big.NewRat(1000000000000, 1)) > 0 {
			return out, ErrInvalid
		}
		// Existing offer values are whole ISK; round the complete bundle up once.
		v, rem := new(big.Int), new(big.Int)
		v.QuoRem(amount.Num(), amount.Denom(), rem)
		if rem.Sign() > 0 {
			v.Add(v, big.NewInt(1))
		}
		out.Value = v.Int64()
	}
	return out, nil
}

func (h Handler) rewardValuation(w http.ResponseWriter, r *http.Request) {
	id, err := number(chi.URLParam(r, "id"))
	if err != nil {
		respond(w, r, nil, err)
		return
	}
	version, err := number(r.URL.Query().Get("version"))
	if err != nil {
		respond(w, r, nil, err)
		return
	}
	out, err := h.Service.ValueReward(r.Context(), h.User(r), id, version)
	respond(w, r, out, err)
}

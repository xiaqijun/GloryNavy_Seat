package welfare

import (
	"context"
	"encoding/json"
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
	"slices"
	"strconv"
	"strings"
)

type GrowthFitting struct {
	ID         int64           `json:"fitting_id,string"`
	Quantity   int64           `json:"quantity"`
	Name       string          `json:"name,omitempty"`
	ShipTypeID int64           `json:"ship_type_id,string,omitempty"`
	Version    int64           `json:"version,string,omitempty"`
	Fit        json.RawMessage `json:"fit,omitempty"`
}
type GrowthItem struct {
	ID       int64  `json:"type_id,string"`
	Quantity int64  `json:"quantity"`
	Name     string `json:"name,omitempty"`
}
type GrowthRewards struct {
	ISKMinor int64           `json:"isk_minor,omitempty"`
	Fittings []GrowthFitting `json:"fittings"`
	Items    []GrowthItem    `json:"items"`
	Coins    int64           `json:"coins_minor"`
}

func growthFittingID(kind string) int64 {
	value, ok := strings.CutPrefix(kind, "growth_fitting_")
	if !ok {
		return 0
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != value {
		return 0
	}
	return id
}
func validKind(kind string) bool {
	return slices.Contains(kinds, kind) || growthFittingID(kind) > 0 || isActivity(kind)
}

func validateGrowthRewards(r *GrowthRewards) error {
	if r == nil || r.ISKMinor < 0 || r.ISKMinor > 100000000000000 || (r.ISKMinor > 0 && r.ISKMinor < 100) || r.Coins < 0 || r.Coins > 1000000000000 || len(r.Fittings) > 10 || len(r.Items) > 30 {
		return ErrInvalid
	}
	if len(r.Fittings) == 0 && len(r.Items) == 0 && r.Coins == 0 && r.ISKMinor == 0 {
		return ErrInvalid
	}
	seen := map[int64]bool{}
	for _, f := range r.Fittings {
		if f.ID <= 0 || f.Quantity < 1 || f.Quantity > 100 || seen[f.ID] {
			return ErrInvalid
		}
		seen[f.ID] = true
	}
	seen = map[int64]bool{}
	for _, i := range r.Items {
		if i.ID <= 0 || i.Quantity < 1 || i.Quantity > 1000000 || seen[i.ID] {
			return ErrInvalid
		}
		seen[i.ID] = true
	}
	return nil
}

// All names and snapshots are server-owned. A client can select only IDs and quantities.
func (s *Service) growthRewards(ctx context.Context, actor string, corp int64, r *GrowthRewards, snapshot bool) (*GrowthRewards, error) {
	if err := validateGrowthRewards(r); err != nil {
		return nil, err
	}
	out := &GrowthRewards{Fittings: []GrowthFitting{}, Items: []GrowthItem{}, Coins: r.Coins, ISKMinor: r.ISKMinor}
	for _, f := range r.Fittings {
		if s.GrowthFitting == nil {
			return nil, ErrRule
		}
		resolved, err := s.GrowthFitting(ctx, actor, corp, f.ID)
		if err != nil {
			return nil, err
		}
		if resolved.ID != f.ID || resolved.ShipTypeID <= 0 || resolved.Name == "" {
			return nil, ErrRule
		}
		resolved.Quantity = f.Quantity
		if !snapshot {
			resolved.Fit = nil
		}
		out.Fittings = append(out.Fittings, resolved)
	}
	if len(r.Items) > 0 {
		if s.Names == nil {
			return nil, ErrRule
		}
		ids := []int64{}
		for _, i := range r.Items {
			ids = append(ids, i.ID)
		}
		names, err := s.Names.TypeNames(ctx, ids)
		if err != nil {
			return nil, err
		}
		for _, i := range r.Items {
			n := names[i.ID]
			if n.Name == "" || n.Source != "sde" {
				return nil, ErrRule
			}
			out.Items = append(out.Items, GrowthItem{ID: i.ID, Quantity: i.Quantity, Name: n.Name})
		}
	}
	return out, nil
}

func (s *Service) prepareGrowth(ctx context.Context, actor string, corp int64, kind string, c Config) (Config, error) {
	id := growthFittingID(kind)
	if id == 0 {
		return c, nil
	}
	if c.RewardID < 0 || c.RewardVersion < 0 || (c.RewardID == 0 && c.RewardVersion != 0) {
		return c, ErrInvalid
	}
	if !c.Enabled {
		policies, err := store.Policies(ctx, s.Pool, corp)
		if err != nil {
			return c, err
		}
		for _, p := range policies {
			if p.Kind == kind {
				var previous Config
				if err = json.Unmarshal(p.Config, &previous); err != nil {
					return c, err
				}
				previous.Enabled = false
				return previous, nil
			}
		}
	}
	if c.FittingID != id || s.GrowthFitting == nil {
		return c, ErrInvalid
	}
	f, err := s.GrowthFitting(ctx, actor, corp, id)
	if err != nil {
		return c, err
	}
	c.ShipTypeID, c.ProjectName = f.ShipTypeID, f.Name
	if c.RewardID > 0 {
		if s.LibraryReward == nil || c.RewardVersion < 1 || c.Rewards == nil {
			return c, ErrInvalid
		}
		coins := c.Rewards.Coins
		c.Rewards, err = s.LibraryReward(ctx, actor, corp, c.RewardID, c.RewardVersion)
		if err != nil {
			return c, err
		}
		c.Rewards.Coins = coins
		return c, validateGrowthRewards(c.Rewards)
	}
	// New physical selections use the library; legacy saved policies still apply unchanged.
	if s.LibraryReward != nil && c.Rewards != nil && (len(c.Rewards.Fittings)+len(c.Rewards.Items) > 0 || c.Rewards.ISKMinor > 0) {
		return c, ErrInvalid
	}
	c.Rewards, err = s.growthRewards(ctx, actor, corp, c.Rewards, false)
	return c, err
}

func growthMet(evidence json.RawMessage) bool {
	var v struct {
		State string `json:"state"`
	}
	return json.Unmarshal(evidence, &v) == nil && v.State == "met"
}

package welfare

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
)

func automaticFulfillment(kind string) bool {
	return cashLoss(kind) || isSuper(kind) || isGrowth(kind) || isActivity(kind)
}
func coinOnly(d Detail) bool {
	return d.Rewards != nil && d.Rewards.Coins > 0 && d.Rewards.ISKMinor == 0 && len(d.Rewards.Fittings) == 0 && len(d.Rewards.Items) == 0
}

// Old growth records may carry the frozen fitting without a reward envelope.
// Never reconstruct rewards from today's editable library.
func (s *Service) prepareFulfillment(kind string, d *Detail) error {
	if !isGrowth(kind) && !isActivity(kind) {
		return nil
	}
	if isActivity(kind) && d.Rewards == nil {
		return ErrRule
	}
	if d.Rewards == nil {
		var old struct {
			Fit  json.RawMessage `json:"fit"`
			Name string          `json:"name"`
			ID   int64           `json:"id,string"`
		}
		if json.Unmarshal(d.FittingEvidence, &old) != nil || len(old.Fit) == 0 {
			return ErrRule
		}
		id := d.Rule.FittingID
		if id <= 0 {
			id = old.ID
		}
		d.Rewards = &GrowthRewards{Fittings: []GrowthFitting{{ID: id, Quantity: 1, Name: old.Name, ShipTypeID: d.ShipTypeID, Fit: old.Fit}}, Items: []GrowthItem{}}
	}
	if validateGrowthRewards(d.Rewards) != nil {
		return ErrRule
	}
	if !coinOnly(*d) {
		if s.MatchReward == nil {
			return ErrRule
		}
		if _, err := s.MatchReward(raw(d.Rewards), eve.DeliveryContract{}); err != nil {
			return ErrRule
		}
	}
	return nil
}

func (s *Service) creditGrowth(ctx context.Context, tx pgx.Tx, v Case, d Detail) error {
	if !(isGrowth(v.Kind) || isActivity(v.Kind)) || d.Rewards == nil || d.Rewards.Coins == 0 {
		return nil
	}
	if s.Credit == nil {
		return ErrRule
	}
	h := sha256.Sum256([]byte(fmt.Sprintf("welfare-fulfillment-%d", v.ID)))
	h[6], h[8] = (h[6]&15)|80, (h[8]&63)|128
	key := fmt.Sprintf("%x-%x-%x-%x-%x", h[:4], h[4:6], h[6:8], h[8:10], h[10:16])
	return s.Credit(ctx, tx, v.AccountID, v.ID, 0, d.Rewards.Coins, key, "福利奖励自动发放")
}

func (s *Service) startFulfillment(ctx context.Context, tx pgx.Tx, v *Case, d *Detail) error {
	if !automaticFulfillment(v.Kind) {
		return nil
	}
	if (isGrowth(v.Kind) || isActivity(v.Kind)) && coinOnly(*d) {
		if err := s.creditGrowth(ctx, tx, *v, *d); err != nil {
			return err
		}
		v.State, d.PaymentStatus = "completed", "coins_credited"
		return nil
	}
	d.PaymentStatus = "waiting_contract"
	return store.ScheduleDelivery(ctx, tx, v.ID, time.Now())
}

package welfare

import (
	"context"
	"errors"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/contractamount"
	"glorynavy.local/seat/internal/modules/eve"
)

var ErrCapitalPurchase = errors.New("verified capital purchase contract required")

// Missing rates in historical snapshots retain the original category rate.
func capitalAward(kind string, base int64, rule Config) (int64, error) {
	if !isSuper(kind) || base <= 0 || base > 100000000000000 {
		return 0, ErrInvalid
	}
	rate := int64(1000)
	if kind == "titan" {
		rate = 500
	}
	if rule.SubsidyRateBPS != nil {
		rate = *rule.SubsidyRateBPS
	}
	if rate < 1 || rate > 10000 {
		return 0, ErrInvalid
	}
	amount := base/10000*rate + base%10000*rate/10000
	if amount < 100 {
		return 0, ErrInvalid
	}
	return amount, nil
}

func isSuper(kind string) bool { return kind == "supercarrier" || kind == "titan" }

// Validate a single purchased hull; other included fittings are part of its price.
func (s *Service) capitalPurchase(kind string, character int64, c eve.DeliveryContract) (int64, int64, error) {
	if c.ID <= 0 || c.OwnerKind != "character" || c.OwnerID != character || c.Type != "item_exchange" || c.Status != "finished" || !c.ItemsReady || c.AcceptorID != character || c.IssuerID <= 0 || c.IssuerID == character || c.AssigneeID != 0 && c.AssigneeID != character || s.IsShip == nil || s.ShipGroup == nil {
		return 0, 0, ErrCapitalPurchase
	}
	completed, err := time.Parse(time.RFC3339, c.Completed)
	if err != nil || c.Issued.IsZero() || completed.Before(c.Issued) || completed.After(time.Now()) {
		return 0, 0, ErrCapitalPurchase
	}
	reward, ok := new(big.Rat).SetString(c.Reward)
	if !ok || reward.Sign() != 0 {
		return 0, 0, ErrCapitalPurchase
	}
	var hull int64
	for _, item := range c.Items {
		if !item.Included || item.Quantity <= 0 {
			return 0, 0, ErrCapitalPurchase
		}
		if s.IsShip(item.TypeID) {
			if hull != 0 || item.Quantity != 1 {
				return 0, 0, ErrCapitalPurchase
			}
			hull = item.TypeID
		}
	}
	group := s.ShipGroup(hull)
	if hull == 0 || kind == "supercarrier" && group != "超级航母" || kind == "titan" && group != "泰坦" {
		return 0, 0, ErrCapitalPurchase
	}
	price, ok := minorPrice(c.Price)
	if !ok {
		return 0, 0, ErrCapitalPurchase
	}
	return hull, price, nil
}

func (s *Service) findCapitalPurchase(ctx context.Context, actor string, corp int64, kind string, id int64, characters []Character) (Character, eve.DeliveryContract, int64, int64, error) {
	if id <= 0 || s.PurchaseContract == nil {
		return Character{}, eve.DeliveryContract{}, 0, 0, ErrCapitalPurchase
	}
	for _, ch := range characters {
		if ch.CorporationID != corp {
			continue
		}
		c, err := s.PurchaseContract(ctx, actor, ch.ID, id)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return Character{}, c, 0, 0, err
		}
		if c.ID != id {
			continue
		}
		hull, price, err := s.capitalPurchase(kind, ch.ID, c)
		if err == nil {
			return ch, c, hull, price, nil
		}
	}
	return Character{}, eve.DeliveryContract{}, 0, 0, ErrCapitalPurchase
}

// Recheck the authorized local snapshot in the publication transaction, never via ESI.
func (s *Service) guardCapitalPurchase(ctx context.Context, tx pgx.Tx, actor, kind string, d Detail) error {
	if s.Contract == nil || d.Purchase == nil || d.Purchase.ID != d.ContractID || d.Purchase.ContentToken == "" {
		return ErrCapitalPurchase
	}
	c, err := s.Contract(ctx, tx, actor, "character", d.CharacterID, d.ContractID)
	if err != nil {
		return err
	}
	hull, price, err := s.capitalPurchase(kind, d.CharacterID, c)
	if err != nil || c.ID != d.ContractID || c.ContentToken != d.Purchase.ContentToken || hull != d.ShipTypeID || price != d.BaseMinor {
		return ErrCapitalPurchase
	}
	return nil
}

// For an issuer-funded item exchange, ESI exposes the offered ISK in reward.
// Fail closed on mixed payment directions or requested goods; never use abs(price).
func capitalPaymentMatches(c eve.DeliveryContract, award int64) bool {
	price, ok := new(big.Rat).SetString(c.Price)
	if !ok || price.Sign() != 0 || award <= 0 {
		return false
	}
	if !contractamount.Matches(award, c.Reward) {
		return false
	}
	for _, item := range c.Items {
		if !item.Included || item.Quantity <= 0 {
			return false
		}
	}
	return true
}

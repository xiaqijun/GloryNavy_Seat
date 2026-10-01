package welfare

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/market"
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
)

var ErrValuation = errors.New("reimbursement valuation requires review")

// Valuation is server-produced evidence, never accepted as a client quote.
// A refresh replaces the current snapshot; the existing audit retains every version.
type Valuation struct {
	Source          string                `json:"source"`
	State           string                `json:"state"`
	Reason          string                `json:"reason"`
	At              time.Time             `json:"at"`
	AmountMinor     int64                 `json:"amount_minor"`
	SettingsVersion int64                 `json:"settings_version,string"`
	Market          *market.Appraisal     `json:"market,omitempty"`
	Contract        *eve.DeliveryContract `json:"contract,omitempty"`
}

func minorPrice(price string) (int64, bool) {
	r, ok := new(big.Rat).SetString(price)
	if !ok || r.Sign() <= 0 {
		return 0, false
	}
	r.Mul(r, big.NewRat(100, 1))
	n := new(big.Int).Quo(r.Num(), r.Denom())
	if !n.IsInt64() || n.Int64() <= 0 || n.Int64() > 100000000000000 {
		return 0, false
	}
	return n.Int64(), true
}

func purchaseReason(c eve.DeliveryContract, d Detail, isShip func(int64) bool) string {
	if c.ID != d.ContractID || c.OwnerKind != "character" || c.OwnerID != d.CharacterID || c.Type != "item_exchange" || c.Status != "finished" || !c.ItemsReady {
		return "购舰合同尚未完成或物品未同步"
	}
	if c.AcceptorID != d.CharacterID || c.IssuerID == d.CharacterID || c.AssigneeID != 0 && c.AssigneeID != d.CharacterID {
		return "购舰合同的购买角色不符"
	}
	completed, err := time.Parse(time.RFC3339, c.Completed)
	lost, errLost := time.Parse(time.RFC3339, d.OccurredAt)
	if err != nil || errLost != nil || c.Issued.IsZero() || completed.Before(c.Issued) || completed.After(lost) {
		return "购舰合同完成时间不在损失之前"
	}
	reward, ok := new(big.Rat).SetString(c.Reward)
	if !ok || reward.Sign() != 0 {
		return "含奖励的交换合同需人工核价"
	}
	ships := int64(0)
	for _, i := range c.Items {
		if !i.Included || i.Quantity <= 0 {
			return "包含索取物品或数量异常，需人工核价"
		}
		if i.TypeID == d.ShipTypeID {
			if i.Quantity != 1 || ships != 0 {
				return "多船合同需人工拆分核价"
			}
			ships++
		} else if isShip != nil && isShip(i.TypeID) {
			return "多船合同需人工拆分核价"
		}
	}
	if ships != 1 {
		return "合同未包含损失船型"
	}
	if _, ok := minorPrice(c.Price); !ok {
		return "购舰合同价格无效或超出范围"
	}
	return ""
}

func lossMarketItems(d Detail) ([]market.Item, error) {
	l := d.LossEvidence
	if !d.SyncedLoss || l == nil || l.ID != d.KillmailID || l.CharacterID != d.CharacterID || l.ShipTypeID != d.ShipTypeID || len(l.Items) > 2000 {
		return nil, ErrValuation
	}
	items := []market.Item{{TypeID: l.ShipTypeID, Quantity: 1, Name: l.ShipName}}
	// Stored killmail entries are already flattened, including nested cargo.
	for _, i := range l.Items {
		if i.TypeID <= 0 || i.Quantity <= 0 || i.Quantity > 1000000000 || i.Destroyed < 0 || i.Dropped < 0 || i.Destroyed > i.Quantity || i.Dropped != i.Quantity-i.Destroyed {
			return nil, ErrValuation
		}
		items = append(items, market.Item{TypeID: i.TypeID, Quantity: i.Quantity, Name: i.Name})
	}
	return items, nil
}

func (s *Service) valuate(ctx context.Context, actor string, d Detail) *Valuation {
	// HTTP writes have a 15s deadline. Leave time to publish the application even
	// when upstream quotes time out; the shared standalone engine allows 25s.
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	v := &Valuation{Source: "market", State: "unavailable", At: time.Now().UTC(), Reason: "市场估价服务暂不可用"}
	if d.ContractID > 0 {
		v.Source, v.Reason = "purchase_contract", "购舰合同未同步或当前无权读取，请核对后重新核价"
		if s.PurchaseContract == nil {
			return v
		}
		c, err := s.PurchaseContract(ctx, actor, d.CharacterID, d.ContractID)
		if err != nil {
			return v
		}
		v.Contract = &c
		v.Reason = purchaseReason(c, d, s.IsShip)
		if v.Reason != "" {
			return v
		}
		v.AmountMinor, _ = minorPrice(c.Price)
		v.State = "ready"
		return v
	}
	items, err := lossMarketItems(d)
	if err != nil {
		v.Reason = "申请缺少完整击毁证据，需人工核价"
		return v
	}
	if s.EstimateLoss == nil {
		return v
	}
	a, version, err := s.EstimateLoss(ctx, items)
	if err != nil {
		return v
	}
	v.Market, v.SettingsVersion = &a, version
	if !a.Complete {
		v.State, v.Reason = "incomplete", "部分物品缺少双边报价，不能按小计核准"
		return v
	}
	n, ok := minorPrice(a.Adjusted.Mid)
	if !ok {
		v.Reason = "折算金额为零或超出核价范围"
		return v
	}
	v.State, v.Reason, v.AmountMinor = "ready", "", n
	return v
}

// AutoAppraise uses the same versioned, audited command as a manual refresh.
// A concurrent review or cancellation wins; the worker never restores a stale
// quote after that state transition.
func (s *Service) AutoAppraise(ctx context.Context, id int64) error {
	v, err := store.Read(ctx, s.Pool, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	if !pendingValuation(v) {
		return nil
	}
	var detail Detail
	if err = json.Unmarshal(v.Detail, &detail); err != nil {
		return err
	}
	if detail.Valuation == nil || detail.Valuation.State != "pending" {
		return nil
	}
	var key string
	if err = s.Pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&key); err != nil {
		return err
	}
	_, err = s.Execute(ctx, v.AccountID, Command{Action: "appraise", ID: id, Version: v.Version, RequestKey: key, Note: "自动核价"})
	if errors.Is(err, ErrConflict) || errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	return err
}

func pendingValuation(v Case) bool {
	return v.Kind == "solo" && slices.Contains([]string{"submitted", "information", "external"}, v.State)
}

func approveValuation(d *Detail, c Command) error {
	if c.ManualPricing {
		if !validText(c.Note, 1000) {
			return ErrInvalid
		}
		d.PricingMode = "manual"
		return nil
	}
	if d.Valuation == nil || d.Valuation.State != "ready" || d.Valuation.AmountMinor <= 0 || c.Detail.BaseMinor != d.Valuation.AmountMinor {
		return ErrValuation
	}
	d.PricingMode = "automatic"
	return nil
}

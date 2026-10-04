package welfare

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"

	"glorynavy.local/seat/internal/contractamount"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/fittings"
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
)

// SettlementItemSummary is the frozen item multiset that the administrator
// can copy into one in-game contract. Type IDs remain authoritative; names are
// deliberately resolved by the existing SDE presentation layer.
type SettlementItemSummary struct {
	TypeID   int64 `json:"type_id,string"`
	Quantity int64 `json:"quantity,string"`
}

const eveContractTitleLimit = 50

func settlementContractTitleMatches(title, reference string) bool {
	title = strings.TrimSpace(title)
	if title == reference {
		return true
	}
	return len(reference) > eveContractTitleLimit && title == reference[:eveContractTitleLimit]
}

func settlementContractDead(status string) bool {
	return status == "cancelled" || status == "deleted" || status == "rejected" || status == "failed" || status == "reversed"
}

func settlementContractMatches(b store.SettlementBatch, c eve.DeliveryContract, recipientIDs []int64) bool {
	price, priceOK := new(big.Rat).SetString(c.Price)
	if c.Type != "item_exchange" || !settlementContractTitleMatches(c.Title, b.SettlementReference) || c.IssuerID <= 0 || !c.ItemsReady || !priceOK || price.Sign() != 0 {
		return false
	}
	allowed := false
	for _, id := range recipientIDs {
		if c.AssigneeID == id {
			allowed = true
			break
		}
	}
	if !allowed || c.AcceptorID != 0 && c.AcceptorID != c.AssigneeID {
		return false
	}
	if b.ISKMinor > 0 && !contractamount.Matches(b.ISKMinor, c.Reward) {
		return false
	}
	if b.ISKMinor == 0 {
		reward, ok := new(big.Rat).SetString(c.Reward)
		if c.Reward != "" && (!ok || reward.Sign() != 0) {
			return false
		}
	}
	var want []SettlementItemSummary
	if len(b.Items) > 0 && json.Unmarshal(b.Items, &want) != nil {
		return false
	}
	counts := map[int64]int64{}
	for _, item := range c.Items {
		if !item.Included || item.TypeID <= 0 || item.Quantity <= 0 || item.RawQuantity != nil && *item.RawQuantity == -2 {
			return false
		}
		counts[item.TypeID] += item.Quantity
	}
	if len(counts) != len(want) {
		return false
	}
	for _, item := range want {
		if counts[item.TypeID] != item.Quantity {
			return false
		}
	}
	return true
}

// settlementRecipients returns the only character that may receive a merged
// settlement. Older batches may still contain every selected character in
// recipient_ids; resolving the current main character here keeps those
// batches from scanning and potentially claiming contracts for an alt.
func (s *Service) settlementRecipients(ctx context.Context, b store.SettlementBatch) ([]int64, error) {
	if s.MainCharacterID == nil || b.AccountID == "" {
		return nil, ErrSettlementUnsupported
	}
	mainID, err := s.MainCharacterID(ctx, b.AccountID)
	if err != nil {
		return nil, err
	}
	if mainID <= 0 {
		return nil, ErrSettlementUnsupported
	}
	return []int64{mainID}, nil
}

func (s *Service) processAggregateSettlement(ctx context.Context, b store.SettlementBatch, items []store.SettlementItem) (bool, error) {
	if b.SettlementReference == "" {
		return false, nil
	}
	recipientIDs, err := s.settlementRecipients(ctx, b)
	if err != nil {
		return true, err
	}
	if s.PaymentContracts == nil || s.ClaimDelivery == nil || s.PaymentBindings == nil || s.SettlementCompleteTx == nil || s.ExchangeSettlementCompleteTx == nil {
		return true, ErrSettlement
	}
	contracts := map[int64]eve.DeliveryContract{}
	for _, recipient := range recipientIDs {
		rows, err := s.PaymentContracts(ctx, nil, b.AccountID, recipient, b.SettlementReference, b.CreatedAt)
		if err != nil {
			return true, err
		}
		for _, c := range rows {
			if settlementContractTitleMatches(c.Title, b.SettlementReference) && !settlementContractDead(c.Status) {
				contracts[c.ID] = c
			}
		}
	}
	if len(contracts) == 0 {
		return true, store.RescheduleSettlementBatch(ctx, s.Pool, b.ID, "等待合并合同同步")
	}
	if len(contracts) != 1 {
		return true, store.RescheduleSettlementBatch(ctx, s.Pool, b.ID, "存在多个同编号合同，请保留一份")
	}
	var contract eve.DeliveryContract
	for _, c := range contracts {
		contract = c
	}
	if !settlementContractMatches(b, contract, recipientIDs) {
		return true, store.RescheduleSettlementBatch(ctx, s.Pool, b.ID, "合同金额或物品尚未匹配")
	}
	if contract.Status != "finished" {
		return true, store.RescheduleSettlementBatch(ctx, s.Pool, b.ID, store.SettlementAwaitingAcceptanceError)
	}
	completedAt, err := time.Parse(time.RFC3339, contract.Completed)
	if err != nil || completedAt.Before(contract.Issued) || completedAt.After(time.Now().Add(time.Minute)) {
		return true, store.RescheduleSettlementBatch(ctx, s.Pool, b.ID, "合同完成时间尚未同步")
	}
	bindings, err := s.PaymentBindings(ctx, nil, []int64{contract.IssuerID})
	if err != nil {
		return true, err
	}
	issuer := bindings[contract.IssuerID]
	if issuer == "" {
		return true, store.RescheduleSettlementBatch(ctx, s.Pool, b.ID, "发放方身份尚未同步")
	}
	if err := s.admin(ctx, issuer); err != nil {
		return true, store.RescheduleSettlementBatch(ctx, s.Pool, b.ID, "发放方不是本站管理员")
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return true, err
	}
	defer tx.Rollback(context.Background())
	if err = s.ClaimDelivery(ctx, tx, contract.ID, "welfare", b.ID); err != nil {
		return true, err
	}
	if err = store.SetSettlementContract(ctx, tx, b.ID, contract.ID, contract.AssigneeID); err != nil {
		return true, err
	}
	for _, item := range items {
		if item.State == "completed" {
			continue
		}
		if item.Source == "welfare" {
			err = s.SettlementCompleteTx(ctx, tx, item.SourceID, contract, issuer)
		} else {
			err = s.ExchangeSettlementCompleteTx(ctx, tx, item.SourceID, contract, issuer)
		}
		if err != nil {
			return true, err
		}
		if err = store.MarkSettlementItem(ctx, tx, item.ID, "completed", ""); err != nil {
			return true, err
		}
	}
	if _, err = store.RefreshSettlementBatch(ctx, tx, b.ID); err != nil {
		return true, err
	}
	return true, tx.Commit(ctx)
}

type SettlementReward struct {
	ISKMinor     int64                   `json:"isk_minor"`
	Items        []SettlementItemSummary `json:"items"`
	RecipientID  int64                   `json:"recipient_id,string,omitempty"`
	RecipientIDs []int64                 `json:"recipient_ids"`
	AccountID    string                  `json:"account_id"`
}

var ErrSettlementUnsupported = errors.New("settlement contains a reward that cannot be merged")

func addSettlementItem(counts map[int64]int64, id, quantity int64) error {
	if id <= 0 || quantity <= 0 || quantity > 1000000000 || counts[id] > 1000000000-quantity {
		return ErrInvalid
	}
	counts[id] += quantity
	return nil
}

func mergeSettlementReward(dst *SettlementReward, src SettlementReward) error {
	if src.ISKMinor < 0 || src.ISKMinor > 100000000000000 || dst.ISKMinor > 100000000000000-src.ISKMinor {
		return ErrInvalid
	}
	dst.ISKMinor += src.ISKMinor
	counts := map[int64]int64{}
	for _, item := range dst.Items {
		counts[item.TypeID] = item.Quantity
	}
	for _, item := range src.Items {
		if err := addSettlementItem(counts, item.TypeID, item.Quantity); err != nil {
			return err
		}
	}
	dst.Items = dst.Items[:0]
	for id, quantity := range counts {
		dst.Items = append(dst.Items, SettlementItemSummary{TypeID: id, Quantity: quantity})
	}
	sort.Slice(dst.Items, func(i, j int) bool { return dst.Items[i].TypeID < dst.Items[j].TypeID })
	return nil
}

func welfareSettlementReward(row Case) (SettlementReward, error) {
	var detail Detail
	if err := json.Unmarshal(row.Detail, &detail); err != nil {
		return SettlementReward{}, err
	}
	out := SettlementReward{ISKMinor: row.Award, RecipientID: detail.CharacterID, RecipientIDs: []int64{detail.CharacterID}, AccountID: row.AccountID, Items: []SettlementItemSummary{}}
	if detail.Rewards != nil {
		if detail.Rewards.Coins != 0 {
			return SettlementReward{}, ErrSettlementUnsupported
		}
		out.ISKMinor += detail.Rewards.ISKMinor
		for _, item := range detail.Rewards.Items {
			out.Items = append(out.Items, SettlementItemSummary{TypeID: item.ID, Quantity: item.Quantity})
		}
		for _, fitting := range detail.Rewards.Fittings {
			var fit fittings.Fit
			if len(fitting.Fit) == 0 || json.Unmarshal(fitting.Fit, &fit) != nil || fit.ShipTypeID <= 0 {
				return SettlementReward{}, ErrSettlementUnsupported
			}
			out.Items = append(out.Items, SettlementItemSummary{TypeID: fit.ShipTypeID, Quantity: fitting.Quantity})
			for _, item := range fit.Items {
				out.Items = append(out.Items, SettlementItemSummary{TypeID: item.TypeID, Quantity: item.Quantity * fitting.Quantity})
			}
		}
	}
	if out.ISKMinor < 0 || (out.ISKMinor > 0 && out.ISKMinor < 100) {
		return SettlementReward{}, ErrInvalid
	}
	return out, nil
}

func (s *Service) settlementSummary(ctx context.Context, items []SettlementInputItem) (string, SettlementReward, error) {
	account := ""
	out := SettlementReward{Items: []SettlementItemSummary{}}
	for _, item := range items {
		var part SettlementReward
		var err error
		if item.Source == "welfare" {
			row, e := store.Read(ctx, s.Pool, item.ID)
			if e != nil {
				return "", SettlementReward{}, e
			}
			part, err = welfareSettlementReward(row)
		} else {
			if s.ExchangeSettlementReward == nil {
				return "", SettlementReward{}, ErrSettlement
			}
			part, err = s.ExchangeSettlementReward(ctx, item.ID)
		}
		if err != nil {
			return "", SettlementReward{}, err
		}
		if account == "" {
			account = part.AccountID
		} else if part.AccountID != account {
			return "", SettlementReward{}, ErrSettlementGroup
		}
		if part.RecipientID <= 0 {
			return "", SettlementReward{}, ErrSettlementUnsupported
		}
		if err := mergeSettlementReward(&out, part); err != nil {
			return "", SettlementReward{}, err
		}
		out.RecipientID = part.RecipientID
		out.RecipientIDs = append(out.RecipientIDs, part.RecipientIDs...)
	}
	if account == "" || out.RecipientID <= 0 || s.MainCharacterID == nil {
		return "", SettlementReward{}, ErrSettlementUnsupported
	}
	mainID, err := s.MainCharacterID(ctx, account)
	if err != nil {
		return "", SettlementReward{}, err
	}
	if mainID <= 0 {
		return "", SettlementReward{}, ErrSettlementUnsupported
	}
	out.AccountID = account
	out.RecipientID = mainID
	out.RecipientIDs = []int64{mainID}
	return account, out, nil
}

func settlementReference(ctx context.Context, db store.DB) (string, error) {
	var value string
	if err := db.QueryRow(ctx, `SELECT gn_settlement_reference('BATCH', now())`).Scan(&value); err != nil {
		return "", err
	}
	if value == "" {
		return "", fmt.Errorf("empty settlement reference")
	}
	return value, nil
}

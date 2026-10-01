package welfare

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
	"glorynavy.local/seat/internal/platform/locale"
)

var ErrDelivery = errors.New("delivery contract cannot be linked")

type Delivery struct {
	Automatic   bool                 `json:"automatic,omitempty"`
	Contract    eve.DeliveryContract `json:"contract"`
	ConfirmedBy string               `json:"confirmed_by"`
	ConfirmedAt time.Time            `json:"confirmed_at"`
	VerifiedAt  *time.Time           `json:"verified_at,omitempty"`
}
type DeliverySelection struct {
	OwnerKind    string `json:"owner_kind"`
	OwnerID      int64  `json:"owner_id,string"`
	ContractID   int64  `json:"contract_id,string"`
	ContentToken string `json:"content_token"`
}
type DeliveryCandidate struct {
	Contract eve.DeliveryContract `json:"contract"`
	CanLink  bool                 `json:"can_link"`
	Reason   string               `json:"reason"`
}

func deliveryFinished(c eve.DeliveryContract, recipient int64) bool {
	at, e := time.Parse(time.RFC3339, c.Completed)
	return e == nil && !at.Before(c.Issued) && c.Status == "finished" && c.AcceptorID == recipient
}

func deliveryReason(v Case, d Detail, c eve.DeliveryContract) string {
	if c.Type != "item_exchange" {
		return "仅支持物品交换合同"
	}
	if c.AssigneeID != d.CharacterID {
		return "接收角色不符"
	}
	if c.IssuerID <= 0 || c.IssuerCorporationID != v.CorporationID || c.IssuerID == d.CharacterID {
		return "发放方不符"
	}
	at, e := time.Parse(time.RFC3339, d.OccurredAt)
	if isSuper(v.Kind) {
		at, e = v.CreatedAt, nil
		if c.ID == d.ContractID {
			return "不能使用购舰合同作为发放合同"
		}
	}
	if e != nil || c.Issued.Before(at) {
		if isSuper(v.Kind) {
			return "合同早于申请时间"
		}
		return "合同早于损失时间"
	}
	if !slices.Contains([]string{"outstanding", "in_progress", "finished_issuer", "finished_contractor", "finished"}, c.Status) {
		return "合同已失效"
	}
	if c.AcceptorID != 0 && c.AcceptorID != d.CharacterID {
		return "实际领取角色不符"
	}
	if !c.ItemsReady {
		return "物品明细尚未同步"
	}
	if isSuper(v.Kind) {
		if !capitalPaymentMatches(c, v.Award) {
			return "合同支付方向或补贴金额不符"
		}
		return ""
	}
	for _, i := range c.Items {
		if i.Included && i.TypeID == d.ShipTypeID && i.Quantity > 0 {
			return ""
		}
	}
	return "未包含对应舰船"
}
func (s *Service) DeliveryCandidates(ctx context.Context, actor string, id, contractID int64) ([]DeliveryCandidate, error) {
	v, e := s.Read(ctx, actor, id)
	if e != nil {
		return nil, e
	}
	if e = s.admin(ctx, actor); e != nil {
		return nil, e
	}
	if e = s.allowed(ctx, actor, v.CorporationID, true); e != nil {
		return nil, e
	}
	if actor == v.AccountID || !slices.Contains([]string{"srp", "solo", "supercarrier", "titan"}, v.Kind) || !slices.Contains([]string{"approved", "executing"}, v.State) || s.Contracts == nil {
		return nil, pgx.ErrNoRows
	}
	var d Detail
	if e = json.Unmarshal(v.Detail, &d); e != nil {
		return nil, e
	}
	at, e := time.Parse(time.RFC3339, d.OccurredAt)
	if isSuper(v.Kind) {
		at, e = v.CreatedAt, nil
	}
	if e != nil {
		return nil, ErrInvalid
	}
	rows, e := s.Contracts(ctx, actor, v.CorporationID, d.CharacterID, contractID, at)
	if e != nil {
		return nil, e
	}
	out := []DeliveryCandidate{}
	for _, c := range rows {
		reason := deliveryReason(v, d, c)
		used, e := store.Claimed(ctx, s.Pool, fmt.Sprintf("delivery:%d", c.ID))
		if e != nil {
			return nil, e
		}
		if used {
			reason = "合同已关联补损单"
		}
		if contractID == 0 && reason != "" {
			continue
		}
		out = append(out, DeliveryCandidate{c, reason == "", locale.Message(ctx, reason)})
	}
	return out, nil
}

// Called inside Execute's account/publication locks, preserving its replay and audit.
func (s *Service) linkDelivery(ctx context.Context, tx pgx.Tx, actor string, v *Case, d *Detail, c Command) error {
	// Reimbursements now use exact-reference ISK verification. Old manually
	// linked evidence is still monitored by CheckDelivery, but new manual links
	// must not bypass the automatic amount/issuer checks.
	if cashLoss(v.Kind) || isGrowth(v.Kind) || isActivity(v.Kind) || (isSuper(v.Kind) && d.PaymentStatus != "") {
		return ErrDelivery
	}
	if !slices.Contains([]string{"srp", "solo", "supercarrier", "titan"}, v.Kind) || !slices.Contains([]string{"approved", "executing"}, v.State) || d.Delivery != nil {
		return ErrConflict
	}
	if actor == v.AccountID {
		return pgx.ErrNoRows
	}
	if e := s.admin(ctx, actor); e != nil {
		return e
	}
	if s.Contract == nil || c.Delivery.ContractID <= 0 || c.Delivery.OwnerID <= 0 || !slices.Contains([]string{"character", "corporation"}, c.Delivery.OwnerKind) {
		return ErrInvalid
	}
	evidence, e := s.Contract(ctx, tx, actor, c.Delivery.OwnerKind, c.Delivery.OwnerID, c.Delivery.ContractID)
	if e != nil {
		return e
	}
	if deliveryReason(*v, *d, evidence) != "" {
		return ErrDelivery
	}
	if c.Delivery.ContentToken == "" || c.Delivery.ContentToken != evidence.ContentToken {
		return ErrConflict
	}
	if s.ClaimDelivery != nil {
		if e = s.ClaimDelivery(ctx, tx, evidence.ID, "welfare", v.ID); e != nil {
			if errors.Is(e, eve.ErrDeliveryClaimed) {
				return ErrConflict
			}
			return e
		}
	}
	claim := fmt.Sprintf("delivery:%d", evidence.ID)
	if e = store.Claim(ctx, tx, claim, v.ID); e != nil {
		return normalize(e)
	}
	v.Keys = append(v.Keys, claim)
	v.State = "executing"
	d.Executor = actor
	now := time.Now().UTC()
	d.Delivery = &Delivery{Contract: evidence, ConfirmedBy: actor, ConfirmedAt: now}
	if deliveryFinished(evidence, d.CharacterID) {
		v.State = "completed"
		d.Delivery.VerifiedAt = &now
		d.Receipt = fmt.Sprintf("ESI 合同 #%d 已完成", evidence.ID)
	}
	return store.ScheduleDelivery(ctx, tx, v.ID, time.Now())
}

func (s *Service) CheckDelivery(ctx context.Context, id int64) error {
	previous, e := store.Read(ctx, s.Pool, id)
	if e != nil {
		return e
	}
	var pd Detail
	if e = json.Unmarshal(previous.Detail, &pd); e != nil {
		return e
	}
	if automaticFulfillment(previous.Kind) && (pd.Delivery == nil || pd.Delivery.Automatic) && slices.Contains([]string{"approved", "executing", "cancel_requested"}, previous.State) {
		return s.checkLossPayment(ctx, previous, pd)
	}
	if (previous.State != "executing" && previous.State != "cancel_requested") || pd.Delivery == nil {
		return nil
	}
	actor := pd.Delivery.ConfirmedBy
	if actor == previous.AccountID || s.Contract == nil {
		return pgx.ErrNoRows
	}
	// Background checks act under the confirmer's current permissions, never a system bypass.
	if e = s.admin(ctx, actor); e != nil {
		return e
	}
	if e = s.allowed(ctx, actor, previous.CorporationID, true); e != nil {
		return e
	}
	if e = s.member(ctx, actor, previous.CorporationID, previous.AccountID); e != nil {
		return e
	}
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(context.Background())
	if s.LockCharacter != nil {
		if e = s.LockCharacter(ctx, tx, previous.AccountID, pd.CharacterID); e != nil {
			return e
		}
	}
	if e = s.LockAccounts(ctx, tx, []string{actor, previous.AccountID}); e != nil {
		return e
	}
	if e = store.Lock(ctx, tx); e != nil {
		return e
	}
	v, e := store.Read(ctx, tx, id)
	if e != nil {
		return e
	}
	if v.Version != previous.Version || v.AccountID != previous.AccountID {
		return ErrConflict
	}
	if e = s.admin(ctx, actor); e != nil {
		return e
	}
	if e = s.allowed(ctx, actor, v.CorporationID, true); e != nil {
		return e
	}
	c := pd.Delivery.Contract
	latest, e := s.Contract(ctx, tx, actor, c.OwnerKind, c.OwnerID, c.ID)
	if e != nil {
		return e
	}
	if latest.ContentToken != c.ContentToken {
		return ErrDelivery
	}
	// Invalid/unfinished states are evidence, not delivery; retain the claim to prevent double payout.
	pd.Delivery.Contract = latest
	if deliveryReason(v, pd, latest) == "" && deliveryFinished(latest, pd.CharacterID) {
		now := time.Now().UTC()
		pd.Delivery.VerifiedAt = &now
		v.State = "completed"
		pd.Receipt = fmt.Sprintf("ESI 合同 #%d 已完成", latest.ID)
	}
	if latest.Status != c.Status || latest.AcceptorID != c.AcceptorID || v.State == "completed" {
		v.Detail = raw(pd)
		v, e = store.Save(ctx, tx, v)
		if e != nil {
			return e
		}
		if e = store.DeliveryAudit(ctx, tx, actor, v); e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}

// CompleteMergedDeliveryTx finalizes one welfare case after the aggregate
// batch matcher has verified a single contract for the whole account group.
func (s *Service) CompleteMergedDeliveryTx(ctx context.Context, tx pgx.Tx, id int64, c eve.DeliveryContract, actor string) error {
	row, err := store.Read(ctx, tx, id)
	if err != nil {
		return err
	}
	if row.State == "completed" {
		return nil
	}
	if row.State != "approved" && row.State != "executing" && row.State != "cancel_requested" {
		return ErrConflict
	}
	var detail Detail
	if err := json.Unmarshal(row.Detail, &detail); err != nil {
		return err
	}
	now := time.Now().UTC()
	detail.Delivery = &Delivery{Automatic: true, Contract: c, ConfirmedBy: actor, ConfirmedAt: now, VerifiedAt: &now}
	detail.Executor = actor
	detail.Receipt = fmt.Sprintf("ESI 合并合同 #%d 已完成", c.ID)
	row.Detail = raw(detail)
	return store.CompleteMergedCaseTx(ctx, tx, actor, row)
}

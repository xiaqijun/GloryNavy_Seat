package welfare

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
)

func lossReference(v Case) string { return v.Reference }

func lossPaymentState(v Case, d Detail, c eve.DeliveryContract) string {
	return fulfillmentState(v, d, c, nil)
}

func fulfillmentState(v Case, d Detail, c eve.DeliveryContract, match func(json.RawMessage, eve.DeliveryContract) (bool, error)) string {
	if v.Reference == "" || c.ID <= 0 || c.ContentToken == "" || c.Type != "item_exchange" || strings.TrimSpace(c.Title) != lossReference(v) || c.IssuerCorporationID != v.CorporationID || c.IssuerID <= 0 || c.IssuerID == d.CharacterID || c.AssigneeID != d.CharacterID || c.Issued.Before(v.CreatedAt) || c.ID == d.ContractID || (c.AcceptorID != 0 && c.AcceptorID != d.CharacterID) {
		return "mismatch"
	}
	if !c.ItemsReady {
		return "waiting_items"
	}
	if isGrowth(v.Kind) || isActivity(v.Kind) {
		if match == nil || d.Rewards == nil {
			return "snapshot_required"
		}
		ok, err := match(raw(d.Rewards), c)
		if err != nil {
			return "snapshot_required"
		}
		if !ok {
			return "mismatch"
		}
	} else if !capitalPaymentMatches(c, v.Award) {
		return "mismatch"
	}
	if c.Status == "outstanding" {
		return "awaiting_acceptance"
	}
	if deliveryFinished(c, d.CharacterID) {
		at, _ := time.Parse(time.RFC3339, c.Completed)
		if !at.After(time.Now().Add(time.Minute)) {
			return "finished"
		}
	}
	return "mismatch"
}

func (s *Service) checkLossPayment(ctx context.Context, previous Case, pd Detail) error {
	if s.PaymentContracts == nil || s.PaymentBindings == nil || s.ClaimDelivery == nil {
		return ErrRule
	}
	actor := pd.Reviewer
	if actor == previous.AccountID || !validUUID(actor) {
		return pgx.ErrNoRows
	}
	if err := s.allowedCase(ctx, actor, previous.CorporationID, previous.Kind, true); err != nil {
		return err
	}
	preview, err := s.PaymentContracts(ctx, nil, previous.AccountID, pd.CharacterID, lossReference(previous), previous.CreatedAt)
	if err != nil {
		return err
	}
	ids := []int64{pd.CharacterID}
	for _, c := range preview {
		ids = append(ids, c.IssuerID)
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	owners, err := s.PaymentBindings(ctx, tx, ids)
	if err != nil {
		return err
	}
	if owners[pd.CharacterID] != previous.AccountID {
		return pgx.ErrNoRows
	}
	accounts := []string{actor, previous.AccountID}
	for _, owner := range owners {
		accounts = append(accounts, owner)
	}
	if err = s.LockAccounts(ctx, tx, accounts); err != nil {
		return err
	}
	if err = store.Lock(ctx, tx); err != nil {
		return err
	}
	v, err := store.Read(ctx, tx, previous.ID)
	if err != nil {
		return err
	}
	if v.Version != previous.Version || v.AccountID != previous.AccountID {
		return ErrConflict
	}
	if err = s.allowedCase(ctx, actor, v.CorporationID, v.Kind, true); err != nil {
		return err
	}
	rows, err := s.PaymentContracts(ctx, tx, v.AccountID, pd.CharacterID, lossReference(v), v.CreatedAt)
	if err != nil {
		return err
	}
	live := []eve.DeliveryContract{}
	for _, c := range rows {
		if !slices.Contains([]string{"cancelled", "deleted", "rejected", "failed", "reversed"}, c.Status) {
			live = append(live, c)
		}
	}
	status := "waiting_contract"
	snapshotErr := s.prepareFulfillment(v.Kind, &pd)
	if isGrowth(v.Kind) {
		occupied, err := store.GrowthOccupied(ctx, tx, v.AccountID, v.Kind, pd.ShipTypeID, v.ID)
		if err != nil {
			return err
		}
		if occupied {
			return ErrGrowthClaimed
		}
	}
	if snapshotErr != nil {
		status = "snapshot_required"
	} else if (isGrowth(v.Kind) || isActivity(v.Kind)) && coinOnly(pd) {
		status = "coins_review_required"
	} else if len(rows) >= 11 || len(live) > 1 {
		status = "multiple_contracts"
	} else if len(live) == 1 {
		c := live[0]
		if !slices.Contains(ids, c.IssuerID) {
			return ErrConflict
		}
		status = fulfillmentState(v, pd, c, s.MatchReward)
		issuer := owners[c.IssuerID]
		if issuer == "" || issuer == v.AccountID {
			status = "issuer_unverified"
		} else if err = s.allowed(ctx, issuer, v.CorporationID, true); err != nil {
			if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			status = "issuer_unverified"
		}
		if pd.Delivery != nil && (pd.Delivery.Contract.ID != c.ID || pd.Delivery.Contract.ContentToken != c.ContentToken) {
			status = "mismatch"
		}
		if status == "finished" || status == "awaiting_acceptance" {
			if err = s.ClaimDelivery(ctx, tx, c.ID, "welfare", v.ID); err != nil {
				if !errors.Is(err, eve.ErrDeliveryClaimed) {
					return err
				}
				status = "contract_claimed"
			} else {
				claim := fmt.Sprintf("delivery:%d", c.ID)
				if !slices.Contains(v.Keys, claim) {
					if err = store.Claim(ctx, tx, claim, v.ID); err != nil {
						return normalize(err)
					}
					v.Keys = append(v.Keys, claim)
				}
				now := time.Now().UTC()
				if pd.Delivery == nil {
					pd.Delivery = &Delivery{Automatic: true, ConfirmedBy: actor, ConfirmedAt: now}
				}
				pd.Delivery.Contract = c
				pd.Executor = issuer
				if v.State != "cancel_requested" {
					v.State = "executing"
				}
				if status == "finished" {
					if err = s.creditGrowth(ctx, tx, v, pd); err != nil {
						return err
					}
					v.State = "completed"
					pd.Delivery.VerifiedAt = &now
					pd.Receipt = fmt.Sprintf("ESI 合同 #%d 已完成", c.ID)
				}
			}
		}
	}
	if pd.Delivery != nil && status == "waiting_contract" {
		status = "contract_unavailable"
	}
	pd.PaymentStatus = status
	if v.State != previous.State || !equalDetail(v.Detail, raw(pd)) {
		v.Detail = raw(pd)
		v, err = store.Save(ctx, tx, v)
		if err != nil {
			return err
		}
		if err = store.DeliveryAudit(ctx, tx, actor, v); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func equalDetail(a, b json.RawMessage) bool {
	var x, y any
	da, db := json.NewDecoder(bytes.NewReader(a)), json.NewDecoder(bytes.NewReader(b))
	da.UseNumber()
	db.UseNumber()
	if da.Decode(&x) != nil || db.Decode(&y) != nil {
		return false
	}
	xb, _ := json.Marshal(x)
	yb, _ := json.Marshal(y)
	return string(xb) == string(yb)
}

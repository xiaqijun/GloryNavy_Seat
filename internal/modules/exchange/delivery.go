package exchange

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/exchange/internal/store"
)

var ErrDeliveryPending = errors.New("delivery must be resolved before cancellation")

func deadContract(c eve.DeliveryContract) bool {
	return slices.Contains([]string{"cancelled", "deleted", "rejected", "failed", "reversed"}, c.Status)
}
func zeroISK(v string) bool { n, ok := new(big.Rat).SetString(v); return ok && n.Sign() == 0 }

// Strict multiset comparison: extra items, requested items, BPCs and partial evidence
// never silently satisfy a delivery. Fits use their saved composition, not current library data.
func deliveryStatus(row store.ExchangeRedemption, c eve.DeliveryContract) string {
	if c.Type != "item_exchange" || c.AssigneeID != row.RecipientID || c.IssuerID <= 0 || c.IssuerID == row.RecipientID || strings.TrimSpace(c.Title) != row.SettlementReference || c.Issued.Before(row.CreatedAt.Time) {
		return "mismatch"
	}
	if c.AcceptorID != 0 && c.AcceptorID != row.RecipientID {
		return "mismatch"
	}
	if !c.ItemsReady {
		return "waiting_items"
	}
	matched, err := MatchRewardDelivery(row.RewardContent, c)
	if err != nil || !matched {
		return "mismatch"
	}
	if c.Status == "finished" {
		at, e := time.Parse(time.RFC3339, c.Completed)
		if e != nil || at.Before(c.Issued) || at.After(time.Now().Add(time.Minute)) || c.AcceptorID != row.RecipientID {
			return "mismatch"
		}
		return "fulfilled"
	}
	if c.Status == "outstanding" {
		return "awaiting_acceptance"
	}
	return "mismatch"
}

// Called by the shared River worker; no network fetches occur in this transaction.
func (s *Service) CheckDelivery(ctx context.Context, id int64) error {
	if s.Contracts == nil || s.ClaimDelivery == nil || s.Bindings == nil || s.LockAccounts == nil {
		return ErrUnavailable
	}
	before, e := store.New(s.Pool).Redemption(ctx, id)
	if e != nil {
		return e
	}
	if before.State != "pending" && before.State != "cancel_requested" {
		return nil
	}
	// Collect identity locks before wallet/order locks to match account merge ordering.
	preview, e := s.Contracts(ctx, nil, before.AccountID.String(), before.RecipientID, before.SettlementReference, before.CreatedAt.Time)
	if e != nil {
		return s.deliveryReadFailure(ctx, id, e)
	}
	ids := []int64{before.RecipientID}
	for _, c := range preview {
		ids = append(ids, c.IssuerID)
	}
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(context.Background())
	bindings, e := s.Bindings(ctx, tx, ids)
	if e != nil {
		return e
	}
	owners := map[int64]string{}
	accounts := []string{before.AccountID.String()}
	for _, b := range bindings {
		owners[b.ID] = b.UserID
		accounts = append(accounts, b.UserID)
	}
	if owners[before.RecipientID] != before.AccountID.String() {
		return pgx.ErrNoRows
	}
	if e = s.LockAccounts(ctx, tx, accounts); e != nil {
		return e
	}
	q := store.New(tx)
	if e = q.LockAccount(ctx, before.AccountID.String()); e != nil {
		return e
	}
	row, e := q.LockRedemption(ctx, id)
	if e != nil {
		return e
	}
	if row.AccountID != before.AccountID || row.Version != before.Version {
		return ErrConflict
	}
	if row.State != "pending" && row.State != "cancel_requested" {
		return nil
	}
	evidence, e := s.Contracts(ctx, tx, row.AccountID.String(), row.RecipientID, row.SettlementReference, row.CreatedAt.Time)
	if e != nil {
		return e
	}
	status := "waiting_contract"
	live := []eve.DeliveryContract{}
	for _, c := range evidence {
		if !deadContract(c) {
			live = append(live, c)
		}
	}
	var issuer string
	if len(evidence) >= 11 || len(live) > 1 {
		status = "multiple_contracts"
	} else if len(live) == 1 {
		c := live[0]
		status = deliveryStatus(row, c)
		issuer = owners[c.IssuerID]
		// A newly discovered issuer wasn't locked, so retry after collecting its identity.
		if !slices.Contains(ids, c.IssuerID) {
			return ErrConflict
		}
		if issuer == "" {
			status = "issuer_unverified"
		} else if e = s.shopAdmin(ctx, issuer); e != nil {
			if !errors.Is(e, pgx.ErrNoRows) {
				return e
			}
			status = "issuer_unverified"
		}
		if status == "fulfilled" || status == "awaiting_acceptance" {
			if e = s.ClaimDelivery(ctx, tx, c.ID, "exchange", id); e != nil {
				if !errors.Is(e, eve.ErrDeliveryClaimed) {
					return e
				}
				status = "contract_claimed"
			}
		}
	}
	// Keep prior evidence if a still-live contract disappears from the source cache.
	old, e := store.ReadDelivery(ctx, tx, id)
	if e != nil {
		return e
	}
	var previous []eve.DeliveryContract
	if e = json.Unmarshal(old.Evidence, &previous); e != nil {
		return e
	}
	for _, past := range previous {
		if deadContract(past) {
			continue
		}
		found := false
		for _, current := range evidence {
			if current.ID == past.ID {
				found = true
				break
			}
		}
		if !found {
			status = "evidence_unavailable"
			evidence = append(evidence, past)
		}
	}

	if e = store.SaveDelivery(ctx, tx, id, status, evidence); e != nil {
		return e
	}
	if status == "fulfilled" {
		actor, _ := uuid(issuer)
		if e = q.DecideRedemption(ctx, store.DecideRedemptionParams{ID: id, State: "fulfilled", Note: "", DecidedBy: actor}); e != nil {
			return e
		}
		if e = store.DeliveryAudit(ctx, tx, id, issuer, evidence); e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}

// CompleteMergedDeliveryTx finalizes one order after welfare has matched the
// aggregate batch contract. The contract claim itself is made once by the
// batch coordinator; this method only writes this order's evidence and state.
func (s *Service) CompleteMergedDeliveryTx(ctx context.Context, tx pgx.Tx, id int64, c eve.DeliveryContract, actor string) error {
	q := store.New(tx)
	row, err := q.LockRedemption(ctx, id)
	if err != nil {
		return err
	}
	if row.State == "fulfilled" {
		return nil
	}
	if row.State != "pending" && row.State != "cancel_requested" {
		return ErrConflict
	}
	if err := store.SaveDelivery(ctx, tx, id, "fulfilled", []eve.DeliveryContract{c}); err != nil {
		return err
	}
	decidedBy, err := uuid(actor)
	if err != nil {
		return err
	}
	if err := q.DecideRedemption(ctx, store.DecideRedemptionParams{ID: id, State: "fulfilled", Note: "", DecidedBy: decidedBy}); err != nil {
		return err
	}
	return store.DeliveryAudit(ctx, tx, id, actor, []eve.DeliveryContract{c})
}

func (s *Service) deliveryReadFailure(ctx context.Context, id int64, cause error) error {
	// Keep the last good evidence. The list must distinguish a blocked read from no contract.
	e := store.MarkDeliveryUnavailable(ctx, s.Pool, id)
	if e != nil {
		return e
	}
	return cause
}

// Under the same order lock as fulfillment, reject cancellation while any cached
// contract can still deliver. The separate human attestation covers unsynced game state.
func (s *Service) guardCancellation(ctx context.Context, tx pgx.Tx, row store.ExchangeRedemption) error {
	if s.Contracts == nil {
		return ErrUnavailable
	}
	contracts, e := s.Contracts(ctx, tx, row.AccountID.String(), row.RecipientID, row.SettlementReference, row.CreatedAt.Time)
	if e != nil {
		return e
	}
	if len(contracts) >= 11 {
		return ErrDeliveryPending
	}
	for _, c := range contracts {
		if !deadContract(c) {
			return ErrDeliveryPending
		}
	}
	old, e := store.ReadDelivery(ctx, tx, row.ID)
	if e != nil {
		return e
	}
	var previous []eve.DeliveryContract
	if e = json.Unmarshal(old.Evidence, &previous); e != nil {
		return e
	}
	for _, c := range previous {
		if deadContract(c) {
			continue
		}
		found := false
		for _, now := range contracts {
			if now.ID == c.ID && deadContract(now) {
				found = true
			}
		}
		if !found {
			return ErrDeliveryPending
		}
	}
	return nil
}

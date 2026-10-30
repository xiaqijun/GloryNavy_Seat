package welfare

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
)

// A rejected cancellation did not release the entitlement or alter the
// approval. Restore only that pending cancellation, preserving its reason.
func (s *Service) revertCancelReview(ctx context.Context, tx pgx.Tx, actor string, v *Case, d *Detail, c Command) error {
	if actor == v.AccountID {
		return pgx.ErrNoRows
	}
	if !automaticFulfillment(v.Kind) || d.Cancellation == nil {
		return ErrConflict
	}
	if v.State == "cancelled" && d.Cancellation.Decision == "approved" {
		if !c.ConfirmedNotDelivered {
			return ErrInvalid
		}
		batched, err := store.ReversionBatchExists(ctx, tx, v.ID)
		if err != nil {
			return err
		}
		if batched {
			return ErrConflict
		}
		if _, err = s.guardLossCancellation(ctx, tx, actor, *v, *d); err != nil {
			return err
		}
		data, err := store.LastCancellationRequest(ctx, tx, v.ID)
		if err != nil {
			return err
		}
		var before Case
		if err = json.Unmarshal(data, &before); err != nil {
			return err
		}
		if before.ID != v.ID || before.AccountID != v.AccountID || before.State != "cancel_requested" || len(before.Keys) == 0 {
			return ErrConflict
		}
		claimed := 0
		for _, key := range before.Keys {
			if strings.HasPrefix(key, "delivery:") {
				continue
			}
			if err = store.Claim(ctx, tx, key, v.ID); err != nil {
				return normalize(err)
			}
			v.Keys = append(v.Keys, key)
			claimed++
		}
		if claimed == 0 {
			return ErrConflict
		}
	} else if !slices.Contains([]string{"approved", "executing"}, v.State) || d.Cancellation.Decision != "rejected" {
		return ErrConflict
	}
	d.Cancellation.Decision, d.Cancellation.Reviewer, d.Cancellation.ReviewNote = "", "", ""
	d.Cancellation.ReviewedAt = nil
	v.State = "cancel_requested"
	return store.ScheduleDelivery(ctx, tx, v.ID, time.Now().UTC())
}

// revertApproval leaves the original decision, payment evidence and quota
// snapshot intact in the reversion history, under the publication lock.
func (s *Service) revertApproval(ctx context.Context, tx pgx.Tx, actor string, v *Case, d *Detail, c Command) error {
	if actor == v.AccountID {
		return pgx.ErrNoRows
	}
	if v.Kind == "grant" || !slices.Contains([]string{"approved", "executing", "rejected"}, v.State) {
		return ErrConflict
	}
	batched, err := store.ReversionBatchExists(ctx, tx, v.ID)
	if err != nil {
		return err
	}
	if batched {
		return ErrConflict
	}
	contracts := []eve.DeliveryContract{}
	if v.State != "rejected" {
		if !c.ConfirmedNotDelivered {
			return ErrInvalid
		}
		if automaticFulfillment(v.Kind) {
			contracts, err = s.guardLossCancellation(ctx, tx, actor, *v, *d)
			if err != nil {
				return err
			}
		} else if d.Delivery != nil || d.Receipt != "" || v.State == "executing" {
			return ErrDelivery
		}
	}
	encoded, err := json.Marshal(contracts)
	if err != nil {
		return err
	}
	if err = store.ArchiveApprovalReversion(ctx, tx, actor, c.RequestKey, *v, encoded); err != nil {
		return err
	}
	if err = store.RemoveRevertedQuota(ctx, tx, v.ID); err != nil {
		return err
	}
	if err = store.ReleaseEntitlements(ctx, tx, v.ID); err != nil {
		return err
	}
	v.Keys = slices.DeleteFunc(v.Keys, func(key string) bool { return !strings.HasPrefix(key, "delivery:") })
	v.Award = 0
	v.State = "submitted"
	d.LossQuotaApproval, d.Cancellation, d.Delivery = nil, nil, nil
	d.PaymentStatus, d.Receipt, d.Reviewer, d.Executor, d.ReviewNote = "", "", "", "", ""
	return nil
}

// Recheck old contracts before any new approval, including an old linked
// contract whose cancellation has disappeared from the local cache.
func (s *Service) guardRevertedApproval(ctx context.Context, tx pgx.Tx, actor string, v Case, d Detail) error {
	prior, err := store.LastReversionSnapshot(ctx, tx, v.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if !automaticFulfillment(v.Kind) {
		return nil
	}
	var old Detail
	if err = json.Unmarshal(prior, &old); err != nil {
		return err
	}
	if old.CharacterID != d.CharacterID {
		return ErrConflict
	}
	_, err = s.guardLossCancellation(ctx, tx, actor, v, old)
	return err
}

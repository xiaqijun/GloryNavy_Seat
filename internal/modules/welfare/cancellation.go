package welfare

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
)

var ErrCancellationPayment = errors.New("welfare payment may still be delivered")

type Cancellation struct {
	Reason      string                 `json:"reason"`
	RequestedAt time.Time              `json:"requested_at"`
	Decision    string                 `json:"decision,omitempty"`
	Reviewer    string                 `json:"reviewer,omitempty"`
	ReviewNote  string                 `json:"review_note,omitempty"`
	ReviewedAt  *time.Time             `json:"reviewed_at,omitempty"`
	Contracts   []eve.DeliveryContract `json:"contracts,omitempty"`
}

// Unknown, outstanding and completed statuses all block cancellation. A missing
// cached contract is never evidence that a previously observed payment was voided.
func cancelledPayment(c eve.DeliveryContract) bool {
	return slices.Contains([]string{"cancelled", "deleted", "rejected"}, c.Status)
}

func (s *Service) guardLossCancellation(ctx context.Context, tx pgx.Tx, actor string, v Case, d Detail) ([]eve.DeliveryContract, error) {
	if s.PaymentContracts == nil {
		return nil, ErrRule
	}
	rows, err := s.PaymentContracts(ctx, tx, v.AccountID, d.CharacterID, lossReference(v), v.CreatedAt)
	if err != nil {
		return nil, err
	}
	if len(rows) >= 11 {
		return nil, ErrCancellationPayment
	}
	for _, c := range rows {
		if !cancelledPayment(c) {
			return nil, ErrCancellationPayment
		}
	}
	if d.Delivery != nil {
		old := d.Delivery.Contract
		found := false
		for _, c := range rows {
			if c.ID == old.ID {
				found = true
			}
		}
		if !found {
			if s.Contract == nil {
				return nil, ErrCancellationPayment
			}
			latest, err := s.Contract(ctx, tx, actor, old.OwnerKind, old.OwnerID, old.ID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return nil, ErrCancellationPayment
				}
				return nil, err
			}
			if latest.ID != old.ID || !cancelledPayment(latest) {
				return nil, ErrCancellationPayment
			}
			rows = append(rows, latest)
		}
	}
	return rows, nil
}

func (s *Service) cancelLoss(ctx context.Context, tx pgx.Tx, actor string, v *Case, d *Detail, c Command) error {
	if !automaticFulfillment(v.Kind) {
		return ErrInvalid
	}
	now := time.Now().UTC()
	if c.Action == "request_cancel" {
		if actor != v.AccountID {
			return pgx.ErrNoRows
		}
		if v.State != "approved" && v.State != "executing" {
			return ErrConflict
		}
		d.Cancellation = &Cancellation{Reason: c.Note, RequestedAt: now}
		v.State = "cancel_requested"
		return store.ScheduleDelivery(ctx, tx, v.ID, now)
	}
	if actor == v.AccountID {
		return pgx.ErrNoRows
	}
	if v.State != "cancel_requested" || d.Cancellation == nil {
		return ErrConflict
	}
	if c.Action == "approve_cancel" {
		if !c.ConfirmedNotDelivered {
			return ErrInvalid
		}
		rows, err := s.guardLossCancellation(ctx, tx, actor, *v, *d)
		if err != nil {
			return err
		}
		d.Cancellation.Contracts = rows
		if err = store.ReleaseEntitlements(ctx, tx, v.ID); err != nil {
			return err
		}
		v.Keys = slices.DeleteFunc(v.Keys, func(k string) bool { return !strings.HasPrefix(k, "delivery:") })
		v.State = "cancelled"
		d.Cancellation.Decision = "approved"
	} else {
		v.State = "approved"
		if d.Delivery != nil {
			v.State = "executing"
		}
		d.Cancellation.Decision = "rejected"
	}
	d.Cancellation.Reviewer, d.Cancellation.ReviewNote, d.Cancellation.ReviewedAt = actor, c.Note, &now
	return nil
}

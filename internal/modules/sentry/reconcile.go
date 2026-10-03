package sentry

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrInvalidAlertDelivery = errors.New("invalid sentry alert delivery")

// AlertIntervalSettlement is the narrow host-injected boundary to exchange.
// The sentry module does not import exchange's private store or decide prices;
// it only translates the remote delivery state into idempotent interval steps.
type AlertIntervalSettlement interface {
	ReserveAlertInterval(context.Context, string, string, string, time.Time, time.Time) error
	SettleAlertInterval(context.Context, string, string, string) error
	ReleaseAlertInterval(context.Context, string, string, string) error
	RefundAlertInterval(context.Context, string, string, string) error
}

func AlertIntervalID(d AlertDelivery) string {
	return strings.Join([]string{d.ChargeEventID, fmt.Sprintf("%d", d.Revision), d.StartedAt, d.EndedAt}, ":")
}

// ReconcileAlertDelivery is retained for historical replay tooling only.
// Production billing no longer calls it: new charges come exclusively from
// authenticated client heartbeat intervals.
func ReconcileAlertDelivery(ctx context.Context, settlement AlertIntervalSettlement, d AlertDelivery) error {
	if settlement == nil || strings.TrimSpace(d.GrantID) == "" || strings.TrimSpace(d.DeliveryID) == "" || strings.TrimSpace(d.ChargeEventID) == "" || d.Revision <= 0 || d.DurationSeconds <= 0 {
		return ErrInvalidAlertDelivery
	}
	started, err := time.Parse(time.RFC3339, d.StartedAt)
	if err != nil {
		return ErrInvalidAlertDelivery
	}
	ended, err := time.Parse(time.RFC3339, d.EndedAt)
	if err != nil || !ended.After(started) || int64(ended.Sub(started)/time.Second) != d.DurationSeconds || ended.Sub(started)%time.Second != 0 {
		return ErrInvalidAlertDelivery
	}
	intervalID := AlertIntervalID(d)
	switch strings.TrimSpace(d.ConsumptionState) {
	case "reserved":
		return settlement.ReserveAlertInterval(ctx, d.GrantID, intervalID, d.DeliveryID, started, ended)
	case "consumed":
		if err := settlement.ReserveAlertInterval(ctx, d.GrantID, intervalID, d.DeliveryID, started, ended); err != nil {
			return err
		}
		return settlement.SettleAlertInterval(ctx, d.GrantID, intervalID, d.DeliveryID)
	case "released":
		if err := settlement.ReserveAlertInterval(ctx, d.GrantID, intervalID, d.DeliveryID, started, ended); err != nil {
			return err
		}
		return settlement.ReleaseAlertInterval(ctx, d.GrantID, intervalID, d.DeliveryID)
	case "refunded":
		// A refund is a correction to a previously settled interval. Do not
		// create a new reservation if the reconciliation cursor starts late.
		return settlement.RefundAlertInterval(ctx, d.GrantID, intervalID, d.DeliveryID)
	default:
		return ErrInvalidAlertDelivery
	}
}

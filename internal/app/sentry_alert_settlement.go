package app

import (
	"context"
	"time"

	"glorynavy.local/seat/internal/modules/exchange"
)

// sentryAlertSettlement adapts the sentry module's narrow host boundary to
// exchange. It keeps module-private SQL out of the integration worker.
type sentryAlertSettlement struct {
	exchange *exchange.Service
}

func (a *sentryAlertSettlement) ReserveAlertTime(ctx context.Context, grantID, accountID, requestKey string, priceVersion string, unitSeconds, unitPriceMinor, reservedSeconds int64, expiresAt time.Time) error {
	return a.exchange.ReserveAlertTime(ctx, exchange.AlertTimeGrantRequest{ID: grantID, AccountID: accountID, RequestKey: requestKey, PriceVersion: priceVersion, UnitSeconds: unitSeconds, UnitPriceMinor: unitPriceMinor, ReservedSeconds: reservedSeconds, ExpiresAt: expiresAt})
}

func (a *sentryAlertSettlement) ReleaseAlertGrant(ctx context.Context, grantID, requestKey string) error {
	return a.exchange.ReleaseAlertGrant(ctx, grantID, requestKey)
}

func (a *sentryAlertSettlement) AlertGrantAccount(ctx context.Context, grantID string) (string, error) {
	return a.exchange.AlertGrantAccount(ctx, grantID)
}

func (a *sentryAlertSettlement) AlertGrantExpiresAt(ctx context.Context, grantID string) (time.Time, error) {
	return a.exchange.AlertGrantExpiresAt(ctx, grantID)
}

func (a *sentryAlertSettlement) ReserveAlertInterval(ctx context.Context, grantID, intervalID, requestKey string, startedAt, endedAt time.Time) error {
	return a.exchange.ReserveAlertInterval(ctx, exchange.AlertTimeIntervalRequest{
		GrantID: grantID, IntervalID: intervalID, RequestKey: requestKey, StartedAt: startedAt, EndedAt: endedAt,
	})
}

func (a *sentryAlertSettlement) SettleAlertInterval(ctx context.Context, grantID, intervalID, requestKey string) error {
	return a.exchange.SettleAlertTime(ctx, grantID, intervalID, requestKey)
}

func (a *sentryAlertSettlement) ReleaseAlertInterval(ctx context.Context, grantID, intervalID, requestKey string) error {
	return a.exchange.ReleaseAlertInterval(ctx, grantID, intervalID, requestKey)
}

func (a *sentryAlertSettlement) RefundAlertInterval(ctx context.Context, grantID, intervalID, requestKey string) error {
	return a.exchange.RefundAlertTime(ctx, grantID, intervalID, requestKey)
}

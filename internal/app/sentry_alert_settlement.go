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

func (a *sentryAlertSettlement) ReserveAlertTimeForSystem(ctx context.Context, grantID, accountID, requestKey, systemID, priceVersion string, unitSeconds, unitPriceMinor, reservedSeconds int64, expiresAt time.Time) error {
	return a.exchange.ReserveAlertTime(ctx, exchange.AlertTimeGrantRequest{ID: grantID, AccountID: accountID, SystemID: systemID, RequestKey: requestKey, PriceVersion: priceVersion, UnitSeconds: unitSeconds, UnitPriceMinor: unitPriceMinor, ReservedSeconds: reservedSeconds, ExpiresAt: expiresAt})
}

func (a *sentryAlertSettlement) ReserveAlertInterval(ctx context.Context, grantID, intervalID, requestKey string, startedAt, endedAt time.Time) error {
	return a.exchange.ReserveAlertInterval(ctx, exchange.AlertTimeIntervalRequest{
		GrantID: grantID, IntervalID: intervalID, RequestKey: requestKey, StartedAt: startedAt, EndedAt: endedAt,
	})
}

func (a *sentryAlertSettlement) SettleAlertInterval(ctx context.Context, grantID, intervalID, requestKey string) error {
	return a.exchange.SettleAlertTime(ctx, grantID, intervalID, requestKey)
}

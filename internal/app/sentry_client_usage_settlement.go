package app

import (
	"context"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/exchange"
	"glorynavy.local/seat/internal/modules/sentry"
)

var _ sentry.CumulativeAlertSettlement = (*sentryAlertSettlement)(nil)

func (a *sentryAlertSettlement) SettleClientUsage(ctx context.Context, c sentry.ClientUsageSettlement) error {
	return a.exchange.SettleClientUsage(ctx, exchange.AlertTimeGrantRequest{
		ID: c.GrantID, AccountID: c.AccountID, SystemID: c.SystemID, RequestKey: c.ReserveKey,
		PriceVersion: c.PriceVersion, UnitSeconds: c.UnitSeconds, UnitPriceMinor: c.UnitPriceMinor,
		ReservedSeconds: c.DurationSeconds, ExpiresAt: c.ExpiresAt,
	}, exchange.AlertTimeIntervalRequest{
		GrantID: c.GrantID, IntervalID: c.UsageID, RequestKey: c.IntervalKey, StartedAt: c.StartedAt, EndedAt: c.EndedAt,
	}, c.SettleKey, func(ctx context.Context, tx pgx.Tx, replayOnly bool) (bool, error) {
		return sentry.GuardClientUsageTx(ctx, tx, c, replayOnly)
	})
}

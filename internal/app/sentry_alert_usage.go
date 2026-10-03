package app

import (
	"context"
	"strconv"
	"time"

	"glorynavy.local/seat/internal/modules/exchange"
	"glorynavy.local/seat/internal/modules/sentry"
)

// sentryAlertUsageReader adapts exchange's read-only financial projection to
// sentry's narrow member-facing boundary without importing exchange's store.
type sentryAlertUsageReader struct {
	exchange *exchange.Service
}

func (r *sentryAlertUsageReader) AlertUsage(ctx context.Context, account string) (sentry.AlertUsageSummary, error) {
	row, err := r.exchange.AlertUsage(ctx, account)
	if err != nil {
		return sentry.AlertUsageSummary{}, err
	}
	return sentry.AlertUsageSummary{
		AvailableMinor: row.AvailableMinor, AlertReservedMinor: row.AlertReservedMinor,
		AlertSettledMinor: row.AlertSettledMinor, AlertReleasedMinor: row.AlertReleasedMinor,
		AlertRefundedMinor: row.AlertRefundedMinor, AsOf: row.AsOf,
		MonitorRewardMinor: row.MonitorRewardMinor,
	}, nil
}

func (r *sentryAlertUsageReader) AlertConsumptions(ctx context.Context, account string, before int64, state string, from, to *time.Time, limit int) (sentry.AlertConsumptionPage, error) {
	page, err := r.exchange.AlertConsumptions(ctx, account, before, state, from, to, limit)
	if err != nil {
		return sentry.AlertConsumptionPage{}, err
	}
	out := sentry.AlertConsumptionPage{Items: make([]sentry.AlertConsumption, 0), NextCursor: "", AsOf: page.AsOf}
	if page.NextCursor > 0 {
		out.NextCursor = formatCursor(page.NextCursor)
	}
	for _, item := range page.Items {
		out.Items = append(out.Items, sentry.AlertConsumption{
			ID: item.ID, GrantID: item.GrantID, IntervalID: item.IntervalID,
			StartedAt: item.StartedAt, EndedAt: item.EndedAt, DurationSeconds: item.DurationSeconds,
			CoinsMinor: item.CoinsMinor, State: item.State, UnitSeconds: item.UnitSeconds,
			UnitPriceMinor: item.UnitPriceMinor, PriceVersion: item.PriceVersion, ExpiresAt: item.ExpiresAt,
		})
	}
	return out, nil
}

func formatCursor(value int64) string {
	return strconv.FormatInt(value, 10)
}

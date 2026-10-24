package sentry

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/sentry/internal/store"
)

// GuardClientUsageTx is injected by the host into exchange's transaction.
// A true result means this usage has durable exclusion evidence and must not
// create a grant, charge, coin entry, or rounding credit.
func GuardClientUsageTx(ctx context.Context, tx pgx.Tx, c ClientUsageSettlement, replayOnly bool) (bool, error) {
	return store.GuardUsage(ctx, tx, store.UsageEvidence{
		ID: strings.TrimSpace(c.UsageID), AccountID: strings.ToLower(strings.TrimSpace(c.AccountID)), SystemID: strings.TrimSpace(c.SystemID),
		StartedAt: c.StartedAt.UTC(), EndedAt: c.EndedAt.UTC(), DurationSeconds: c.DurationSeconds,
	}, c.PriceVersion, c.UnitSeconds, c.UnitPriceMinor, replayOnly)
}

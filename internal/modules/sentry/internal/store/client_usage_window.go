package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrUsageConflict = errors.New("client usage exclusion evidence changed")
var ErrChargingWindow = errors.New("client usage charging window unavailable")
var ErrUsagePolicyChanged = errors.New("client usage pricing changed")

type UsageEvidence struct {
	ID, AccountID, SystemID string
	StartedAt, EndedAt      time.Time
	DurationSeconds         int64
}

func excludedUsage(ctx context.Context, tx pgx.Tx, c UsageEvidence) (bool, error) {
	var old UsageEvidence
	err := tx.QueryRow(ctx, `SELECT account_id::text,system_id,started_at,ended_at,duration_seconds FROM sentry_client_usage_exclusions WHERE usage_id=$1`, c.ID).Scan(&old.AccountID, &old.SystemID, &old.StartedAt, &old.EndedAt, &old.DurationSeconds)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if old.AccountID != c.AccountID || old.SystemID != c.SystemID || !old.StartedAt.Equal(c.StartedAt) || !old.EndedAt.Equal(c.EndedAt) || old.DurationSeconds != c.DurationSeconds {
		return false, ErrUsageConflict
	}
	return true, nil
}

// GuardUsage locks the durable gate until the caller commits either billing or
// exclusion evidence. Only genuine off-to-on transitions begin a new window.
func GuardUsage(ctx context.Context, tx pgx.Tx, c UsageEvidence, priceVersion string, unit, price int64, replayOnly bool) (bool, error) {
	if excluded, err := excludedUsage(ctx, tx, c); excluded || err != nil {
		return excluded, err
	}
	if replayOnly {
		return false, nil
	}
	var enabled bool
	var currentVersion string
	var currentUnit, currentPrice int64
	if err := tx.QueryRow(ctx, `SELECT charging_enabled,price_version,unit_seconds,unit_price_minor FROM sentry_alert_pricing WHERE id=1 FOR SHARE`).Scan(&enabled, &currentVersion, &currentUnit, &currentPrice); err != nil {
		return false, err
	}
	if !enabled {
		return false, ErrChargingWindow
	}
	var auditID int64
	var opened time.Time
	var transitionEnabled bool
	if err := tx.QueryRow(ctx, `SELECT id,created_at,after_snapshot->>'charging_enabled'='true' FROM sentry_alert_pricing_audit
		WHERE coalesce(before_snapshot->>'charging_enabled','false') IS DISTINCT FROM after_snapshot->>'charging_enabled'
		ORDER BY id DESC LIMIT 1`).Scan(&auditID, &opened, &transitionEnabled); err != nil {
		return false, err
	}
	if !transitionEnabled {
		return false, ErrChargingWindow
	}
	if !c.StartedAt.Before(opened) {
		if currentVersion != priceVersion || currentUnit != unit || currentPrice != price {
			return false, ErrUsagePolicyChanged
		}
		return false, nil
	}
	reason := "before_latest_enable"
	if c.EndedAt.After(opened) {
		reason = "crosses_latest_enable"
	}
	if _, err := tx.Exec(ctx, `INSERT INTO sentry_client_usage_exclusions(usage_id,account_id,system_id,started_at,ended_at,duration_seconds,reason,enable_audit_id,enabled_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT(usage_id) DO NOTHING`, c.ID, c.AccountID, c.SystemID, c.StartedAt, c.EndedAt, c.DurationSeconds, reason, auditID, opened); err != nil {
		return false, err
	}
	return excludedUsage(ctx, tx, c)
}

func UsageExclusionMergeSummary(ctx context.Context, tx pgx.Tx, source, target string) (int64, string, error) {
	var count int64
	var fingerprint string
	err := tx.QueryRow(ctx, `SELECT count(*) FILTER(WHERE account_id=$1),md5(coalesce(string_agg(to_jsonb(e)::text,'' ORDER BY usage_id),'')) FROM sentry_client_usage_exclusions e WHERE account_id IN ($1,$2)`, source, target).Scan(&count, &fingerprint)
	return count, fingerprint, err
}

func MergeUsageExclusions(ctx context.Context, tx pgx.Tx, source, target string) error {
	_, err := tx.Exec(ctx, `UPDATE sentry_client_usage_exclusions SET original_account_id=coalesce(original_account_id,account_id),account_id=$2 WHERE account_id=$1`, source, target)
	return err
}

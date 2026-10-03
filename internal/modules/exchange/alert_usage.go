package exchange

import (
	"context"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// AlertUsageSummary is the member-facing financial view of alert consumption.
// Seconds are included only to explain the frozen pricing basis; coins remain
// the accounting unit shown to members.
type AlertUsageSummary struct {
	AvailableMinor     int64
	AlertReservedMinor int64
	AlertSettledMinor  int64
	AlertReleasedMinor int64
	AlertRefundedMinor int64
	MonitorRewardMinor int64
	AsOf               time.Time
}

type AlertConsumption struct {
	ID              int64
	GrantID         string
	IntervalID      string
	StartedAt       time.Time
	EndedAt         time.Time
	DurationSeconds int64
	CoinsMinor      int64
	State           string
	UnitSeconds     int64
	UnitPriceMinor  int64
	PriceVersion    string
	ExpiresAt       time.Time
}

type AlertConsumptionPage struct {
	Items      []AlertConsumption
	NextCursor int64
	AsOf       time.Time
}

// AlertUsage reads the exchange-owned balance and alert ledgers in one
// repeatable-read snapshot. It deliberately keeps alert values separate from
// generic exchange reservation/spend totals.
func (s *Service) AlertUsage(ctx context.Context, account string) (AlertUsageSummary, error) {
	var out AlertUsageSummary
	id, err := uuid(account)
	if err != nil {
		return out, err
	}
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return out, err
	}
	defer tx.Rollback(context.Background())
	const query = `
SELECT
  coalesce((SELECT sum(delta) FROM exchange_coin_ledger WHERE account_id=$1 AND kind='source'),0)
    - coalesce((SELECT sum(coins_minor) FROM exchange_redemptions WHERE account_id=$1 AND state IN ('pending','cancel_requested')),0)
    - coalesce((SELECT sum(reserved_minor-settled_minor-released_minor) FROM exchange_alert_grants WHERE account_id=$1 AND state='active'),0)
    - coalesce((SELECT sum(coins_minor) FROM exchange_redemptions WHERE account_id=$1 AND state='fulfilled'),0)
    - coalesce((SELECT sum(coins_minor) FROM exchange_alert_charges WHERE account_id=$1 AND state='settled'),0) AS available_minor,
  coalesce((SELECT sum(reserved_minor-settled_minor-released_minor) FROM exchange_alert_grants WHERE account_id=$1 AND state='active'),0) AS alert_reserved_minor,
  coalesce((SELECT sum(coins_minor) FROM exchange_alert_charges WHERE account_id=$1 AND state='settled'),0) AS alert_settled_minor,
  coalesce((SELECT sum(released_minor) FROM exchange_alert_grants WHERE account_id=$1),0) AS alert_released_minor,
	coalesce((SELECT sum(coins_minor) FROM exchange_alert_charges WHERE account_id=$1 AND state='refunded'),0) AS alert_refunded_minor,
  coalesce((SELECT sum(delta) FROM exchange_coin_ledger WHERE account_id=$1 AND kind='source' AND reference LIKE 'sentry-monitor:%'),0) AS monitor_reward_minor`
	if err := tx.QueryRow(ctx, query, id).Scan(&out.AvailableMinor, &out.AlertReservedMinor, &out.AlertSettledMinor, &out.AlertReleasedMinor, &out.AlertRefundedMinor, &out.MonitorRewardMinor); err != nil {
		return out, err
	}
	if err := tx.Commit(ctx); err != nil {
		return out, err
	}
	out.AsOf = time.Now().UTC()
	return out, nil
}

// AlertConsumptions returns a stable newest-first page for one account. The
// cursor is the opaque-to-UI database row id used by the existing exchange
// pagination convention.
func (s *Service) AlertConsumptions(ctx context.Context, account string, before int64, state string, from, to *time.Time, limit int) (AlertConsumptionPage, error) {
	out := AlertConsumptionPage{Items: make([]AlertConsumption, 0)}
	id, err := uuid(account)
	if err != nil {
		return out, err
	}
	if before <= 0 {
		before = math.MaxInt64
	}
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	state = strings.TrimSpace(state)
	if state != "" && state != "reserved" && state != "settled" && state != "released" && state != "refunded" {
		return out, ErrInvalid
	}
	const query = `
SELECT c.id,c.grant_id::text,c.interval_id,c.started_at,c.ended_at,c.duration_seconds,c.coins_minor,c.state,
       g.unit_seconds,g.unit_price_minor,coalesce(g.price_version,''),g.expires_at
FROM exchange_alert_charges c
JOIN exchange_alert_grants g ON g.id=c.grant_id
WHERE c.account_id=$1
  AND c.id < $2
  AND ($3='' OR c.state=$3)
  AND ($4::timestamptz IS NULL OR c.started_at >= $4)
  AND ($5::timestamptz IS NULL OR c.started_at < $5)
ORDER BY c.id DESC
LIMIT $6`
	var fromArg, toArg any
	if from != nil {
		fromArg = *from
	}
	if to != nil {
		toArg = *to
	}
	rows, err := s.Pool.Query(ctx, query, id, before, state, fromArg, toArg, limit+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var item AlertConsumption
		if err := rows.Scan(&item.ID, &item.GrantID, &item.IntervalID, &item.StartedAt, &item.EndedAt, &item.DurationSeconds, &item.CoinsMinor, &item.State, &item.UnitSeconds, &item.UnitPriceMinor, &item.PriceVersion, &item.ExpiresAt); err != nil {
			return out, err
		}
		out.Items = append(out.Items, item)
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		out.NextCursor = out.Items[len(out.Items)-1].ID
	}
	out.AsOf = time.Now().UTC()
	return out, nil
}

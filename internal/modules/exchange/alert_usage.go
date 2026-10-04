package exchange

import (
	"context"
	"math"
	"sort"
	"strconv"
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
	SystemID        string
}

type AlertConsumptionPage struct {
	Items      []AlertConsumption
	NextCursor int64
	AsOf       time.Time
}

// mergeAlertConsumptions folds adjacent settled/reserved records that use the
// same frozen price. Exchange keeps each source interval for idempotency and
// audit, while the member read model presents one continuous online run.
func mergeAlertConsumptions(items []AlertConsumption) []AlertConsumption {
	if len(items) < 2 {
		return items
	}
	groups := make(map[string][]AlertConsumption)
	for _, item := range items {
		key := strings.Join([]string{item.State, strconv.FormatInt(item.UnitSeconds, 10), strconv.FormatInt(item.UnitPriceMinor, 10), item.PriceVersion, item.SystemID}, "\x00")
		group := groups[key]
		if len(group) > 0 {
			newer := &group[len(group)-1]
			if newer.StartedAt.Equal(item.EndedAt) {
				item.EndedAt = newer.EndedAt
				item.DurationSeconds += newer.DurationSeconds
				item.CoinsMinor += newer.CoinsMinor
				item.ID = newer.ID
				item.GrantID = newer.GrantID
				item.IntervalID = "merged:" + strconv.FormatInt(item.ID, 10)
				group[len(group)-1] = item
				groups[key] = group
				continue
			}
		}
		groups[key] = append(group, item)
	}
	out := make([]AlertConsumption, 0, len(items))
	for _, group := range groups {
		out = append(out, group...)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
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
  coalesce((SELECT sum(delta) FROM exchange_coin_ledger WHERE account_id=$1 AND kind='source' AND (starts_with(reference,'sentry-monitor:') OR starts_with(reference,'sentry-monitor-batch:'))),0) AS monitor_reward_minor`
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
       g.unit_seconds,g.unit_price_minor,coalesce(g.price_version,''),g.expires_at,c.system_id
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
	// Read enough raw intervals to collapse heartbeat-sized records into a
	// useful page. The source rows remain individually auditable in storage.
	fetchLimit := limit * 100
	if fetchLimit < limit+1 {
		fetchLimit = limit + 1
	}
	if fetchLimit > 10000 {
		fetchLimit = 10000
	}
	rows, err := s.Pool.Query(ctx, query, id, before, state, fromArg, toArg, fetchLimit+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var item AlertConsumption
		if err := rows.Scan(&item.ID, &item.GrantID, &item.IntervalID, &item.StartedAt, &item.EndedAt, &item.DurationSeconds, &item.CoinsMinor, &item.State, &item.UnitSeconds, &item.UnitPriceMinor, &item.PriceVersion, &item.ExpiresAt, &item.SystemID); err != nil {
			return out, err
		}
		out.Items = append(out.Items, item)
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	rawHasMore := len(out.Items) > fetchLimit
	if rawHasMore {
		out.Items = out.Items[:fetchLimit]
	}
	out.Items = mergeAlertConsumptions(out.Items)
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		out.NextCursor = out.Items[len(out.Items)-1].ID
	} else if rawHasMore && len(out.Items) > 0 {
		out.NextCursor = out.Items[len(out.Items)-1].ID
	}
	out.AsOf = time.Now().UTC()
	return out, nil
}

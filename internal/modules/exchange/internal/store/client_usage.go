package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// LegacyAlertUsageCoverage prevents re-reading a page under its original raw
// IDs from charging intervals already settled under a legacy merged batch ID.
func (q *Queries) LegacyAlertUsageCoverage(ctx context.Context, account pgtype.UUID, system string, start, end time.Time) (covered, overlaps bool, err error) {
	err = q.db.QueryRow(ctx, `SELECT coalesce(bool_or(c.started_at<=$3 AND c.ended_at>=$4),false),count(*)>0
		FROM exchange_alert_charges c JOIN exchange_alert_grants g ON g.id=c.grant_id
		WHERE c.account_id=$1 AND c.system_id=$2 AND c.state='settled' AND NOT g.cumulative_rounding AND c.started_at<$4 AND c.ended_at>$3`, account, system, start, end).Scan(&covered, &overlaps)
	return covered, overlaps, err
}

// AlertRoundingCredit is called while holding the exchange account lock.
func (q *Queries) AlertRoundingCredit(ctx context.Context, account pgtype.UUID, system, version string, unit, price int64) (int64, error) {
	_, err := q.db.Exec(ctx, `INSERT INTO exchange_alert_rounding(account_id,system_id,price_version,unit_seconds,unit_price_minor) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, account, system, version, unit, price)
	if err != nil {
		return 0, err
	}
	var credit int64
	err = q.db.QueryRow(ctx, `SELECT credit_numerator FROM exchange_alert_rounding WHERE account_id=$1 AND system_id=$2 AND price_version=$3 AND unit_seconds=$4 AND unit_price_minor=$5 FOR UPDATE`, account, system, version, unit, price).Scan(&credit)
	return credit, err
}

func (q *Queries) SetAlertRoundingCredit(ctx context.Context, account pgtype.UUID, system, version string, unit, price, credit int64) error {
	_, err := q.db.Exec(ctx, `UPDATE exchange_alert_rounding SET credit_numerator=$6 WHERE account_id=$1 AND system_id=$2 AND price_version=$3 AND unit_seconds=$4 AND unit_price_minor=$5`, account, system, version, unit, price, credit)
	return err
}

func (q *Queries) MarkCumulativeAlertGrant(ctx context.Context, grant pgtype.UUID) error {
	_, err := q.db.Exec(ctx, `UPDATE exchange_alert_grants SET cumulative_rounding=true WHERE id=$1`, grant)
	return err
}

func (q *Queries) MergeAlertRoundingAccount(ctx context.Context, source, target string) error {
	// Preserve all prepaid fractions, even if their sum exceeds one minor unit.
	// Original grant/charge/ledger rows retain each contribution's provenance.
	_, err := q.db.Exec(ctx, `INSERT INTO exchange_alert_rounding(account_id,system_id,price_version,unit_seconds,unit_price_minor,credit_numerator,original_account_id)
		SELECT $2,system_id,price_version,unit_seconds,unit_price_minor,credit_numerator,coalesce(original_account_id,account_id) FROM exchange_alert_rounding WHERE account_id=$1
		ON CONFLICT(account_id,system_id,price_version,unit_seconds,unit_price_minor) DO UPDATE SET credit_numerator=exchange_alert_rounding.credit_numerator+EXCLUDED.credit_numerator`, source, target)
	if err != nil {
		return err
	}
	_, err = q.db.Exec(ctx, `DELETE FROM exchange_alert_rounding WHERE account_id=$1`, source)
	return err
}

package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// AlertGrant is the exchange-owned prepaid time allowance. The amount left in
// the grant is reserved from the wallet until it is settled or released.
type AlertGrant struct {
	ID              pgtype.UUID
	AccountID       pgtype.UUID
	SystemID        string
	RequestKey      pgtype.UUID
	PriceVersion    string
	UnitSeconds     int64
	UnitPriceMinor  int64
	ReservedSeconds int64
	ReservedMinor   int64
	ReleasedSeconds int64
	SettledSeconds  int64
	ReleasedMinor   int64
	SettledMinor    int64
	ExpiresAt       time.Time
	State           string
}

type AlertCharge struct {
	ID              int64
	GrantID         pgtype.UUID
	AccountID       pgtype.UUID
	SystemID        string
	IntervalID      string
	StartedAt       time.Time
	EndedAt         time.Time
	DurationSeconds int64
	CoinsMinor      int64
	State           string
}

const findAlertGrantByRequest = `SELECT id,account_id,request_key,price_version,unit_seconds,unit_price_minor,reserved_seconds,reserved_minor,released_seconds,settled_seconds,released_minor,settled_minor,expires_at,state,system_id FROM exchange_alert_grants WHERE account_id=$1 AND request_key=$2`

func (q *Queries) FindAlertGrantByRequest(ctx context.Context, accountID, requestKey pgtype.UUID) (AlertGrant, error) {
	return scanAlertGrant(q.db.QueryRow(ctx, findAlertGrantByRequest, accountID, requestKey))
}

const lockAlertGrant = `SELECT id,account_id,request_key,price_version,unit_seconds,unit_price_minor,reserved_seconds,reserved_minor,released_seconds,settled_seconds,released_minor,settled_minor,expires_at,state,system_id FROM exchange_alert_grants WHERE id=$1 FOR UPDATE`

const findAlertGrant = `SELECT id,account_id,request_key,price_version,unit_seconds,unit_price_minor,reserved_seconds,reserved_minor,released_seconds,settled_seconds,released_minor,settled_minor,expires_at,state,system_id FROM exchange_alert_grants WHERE id=$1`

func (q *Queries) AlertGrant(ctx context.Context, id pgtype.UUID) (AlertGrant, error) {
	return scanAlertGrant(q.db.QueryRow(ctx, findAlertGrant, id))
}

func (q *Queries) LockAlertGrant(ctx context.Context, id pgtype.UUID) (AlertGrant, error) {
	return scanAlertGrant(q.db.QueryRow(ctx, lockAlertGrant, id))
}

func scanAlertGrant(row pgx.Row) (AlertGrant, error) {
	var g AlertGrant
	err := row.Scan(&g.ID, &g.AccountID, &g.RequestKey, &g.PriceVersion, &g.UnitSeconds, &g.UnitPriceMinor, &g.ReservedSeconds, &g.ReservedMinor, &g.ReleasedSeconds, &g.SettledSeconds, &g.ReleasedMinor, &g.SettledMinor, &g.ExpiresAt, &g.State, &g.SystemID)
	return g, err
}

const createAlertGrant = `INSERT INTO exchange_alert_grants(id,account_id,request_key,price_version,unit_seconds,unit_price_minor,reserved_seconds,reserved_minor,expires_at,system_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`

func (q *Queries) CreateAlertGrant(ctx context.Context, id, accountID, requestKey pgtype.UUID, priceVersion string, unitSeconds, unitPriceMinor, reservedSeconds, reservedMinor int64, expiresAt time.Time, systemID string) error {
	_, err := q.db.Exec(ctx, createAlertGrant, id, accountID, requestKey, priceVersion, unitSeconds, unitPriceMinor, reservedSeconds, reservedMinor, expiresAt, systemID)
	return err
}

const countAlertReserved = `SELECT count(*) FROM exchange_alert_charges WHERE grant_id=$1 AND state='reserved'`

func (q *Queries) CountAlertReserved(ctx context.Context, grantID pgtype.UUID) (int64, error) {
	var n int64
	err := q.db.QueryRow(ctx, countAlertReserved, grantID).Scan(&n)
	return n, err
}

// Refunded intervals are corrections and must not block a replacement
// interval for the same service time. Reserved and settled intervals still
// prevent overlapping consumption within one grant.
const alertIntervalOverlap = `SELECT EXISTS(SELECT 1 FROM exchange_alert_charges WHERE grant_id=$1 AND state IN('reserved','settled') AND started_at < $3 AND ended_at > $2)`

func (q *Queries) AlertIntervalOverlap(ctx context.Context, grantID pgtype.UUID, startedAt, endedAt time.Time) (bool, error) {
	var exists bool
	err := q.db.QueryRow(ctx, alertIntervalOverlap, grantID, startedAt, endedAt).Scan(&exists)
	return exists, err
}

const findAlertCharge = `SELECT id,grant_id,account_id,interval_id,started_at,ended_at,duration_seconds,coins_minor,state,system_id FROM exchange_alert_charges WHERE grant_id=$1 AND interval_id=$2 FOR UPDATE`

func (q *Queries) LockAlertCharge(ctx context.Context, grantID pgtype.UUID, intervalID string) (AlertCharge, error) {
	var c AlertCharge
	err := q.db.QueryRow(ctx, findAlertCharge, grantID, intervalID).Scan(&c.ID, &c.GrantID, &c.AccountID, &c.IntervalID, &c.StartedAt, &c.EndedAt, &c.DurationSeconds, &c.CoinsMinor, &c.State, &c.SystemID)
	return c, err
}

const createAlertCharge = `INSERT INTO exchange_alert_charges(grant_id,account_id,interval_id,started_at,ended_at,duration_seconds,coins_minor,reserve_key,system_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`

func (q *Queries) CreateAlertCharge(ctx context.Context, grantID, accountID pgtype.UUID, intervalID string, startedAt, endedAt time.Time, durationSeconds, coinsMinor int64, reserveKey pgtype.UUID, systemID string) error {
	_, err := q.db.Exec(ctx, createAlertCharge, grantID, accountID, intervalID, startedAt, endedAt, durationSeconds, coinsMinor, reserveKey, systemID)
	return err
}

const updateAlertChargeSettled = `UPDATE exchange_alert_charges SET state='settled',settle_key=$2,decided_at=now() WHERE id=$1 AND state='reserved'`
const updateAlertChargeReleased = `UPDATE exchange_alert_charges SET state='released',release_key=$2,decided_at=now() WHERE id=$1 AND state='reserved'`
const updateAlertChargeRefunded = `UPDATE exchange_alert_charges SET state='refunded',refund_key=$2,decided_at=now() WHERE id=$1 AND state='settled'`

func (q *Queries) SettleAlertCharge(ctx context.Context, id int64, key pgtype.UUID) error {
	_, err := q.db.Exec(ctx, updateAlertChargeSettled, id, key)
	return err
}
func (q *Queries) ReleaseAlertCharge(ctx context.Context, id int64, key pgtype.UUID) error {
	_, err := q.db.Exec(ctx, updateAlertChargeReleased, id, key)
	return err
}
func (q *Queries) RefundAlertCharge(ctx context.Context, id int64, key pgtype.UUID) error {
	_, err := q.db.Exec(ctx, updateAlertChargeRefunded, id, key)
	return err
}

const updateAlertGrantSettled = `UPDATE exchange_alert_grants SET settled_seconds=settled_seconds+$2,settled_minor=settled_minor+$3 WHERE id=$1`
const updateAlertGrantReleased = `UPDATE exchange_alert_grants SET released_seconds=released_seconds+$2,released_minor=released_minor+$3 WHERE id=$1`
const reverseAlertGrantSettled = `UPDATE exchange_alert_grants SET settled_seconds=settled_seconds-$2,settled_minor=settled_minor-$3 WHERE id=$1`
const reopenAlertGrant = `UPDATE exchange_alert_grants SET state='active' WHERE id=$1 AND state='closed' AND expires_at>now()`
const closeAlertGrant = `UPDATE exchange_alert_grants SET state='closed',closed_at=now() WHERE id=$1 AND state='active'`

func (q *Queries) AddAlertSettled(ctx context.Context, id pgtype.UUID, seconds, minor int64) error {
	_, err := q.db.Exec(ctx, updateAlertGrantSettled, id, seconds, minor)
	return err
}
func (q *Queries) AddAlertReleased(ctx context.Context, id pgtype.UUID, seconds, minor int64) error {
	_, err := q.db.Exec(ctx, updateAlertGrantReleased, id, seconds, minor)
	return err
}
func (q *Queries) ReverseAlertSettled(ctx context.Context, id pgtype.UUID, seconds, minor int64) error {
	_, err := q.db.Exec(ctx, reverseAlertGrantSettled, id, seconds, minor)
	return err
}
func (q *Queries) ReopenAlertGrant(ctx context.Context, id pgtype.UUID) error {
	_, err := q.db.Exec(ctx, reopenAlertGrant, id)
	return err
}
func (q *Queries) CloseAlertGrant(ctx context.Context, id pgtype.UUID) error {
	_, err := q.db.Exec(ctx, closeAlertGrant, id)
	return err
}

package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

type AlliancePAPSync struct {
	Month        time.Time
	State        string
	Complete     bool
	RecordsTotal int32
	LastSyncedAt pgtype.Timestamptz
	LastError    string
	Version      int64
}

type AlliancePAPSyncHistory struct {
	Month        time.Time
	State        string
	Complete     bool
	RecordsTotal int32
	LastSyncedAt pgtype.Timestamptz
	LastError    string
	Version      int64
}

type AlliancePAPAccountReport struct {
	Month        time.Time
	Points       string
	State        string
	Complete     bool
	RecordsTotal int32
	LastSyncedAt pgtype.Timestamptz
	LastError    string
	Version      int64
}

type AlliancePAPCharacter struct {
	CharacterID   int64
	CharacterName string
	PAP           string
}

type AlliancePAPAward struct {
	CharacterID int64
	AccountID   pgtype.UUID
	PAP         string
}

// AlliancePAPFulfillment counts distinct bound site accounts in the current
// snapshot. PAP is evaluated after aggregating all roles belonging to an
// account, so multi-character accounts are counted once.
func (q *Queries) AlliancePAPFulfillment(ctx context.Context, month time.Time, target int32) (eligible, achieved int32, err error) {
	err = q.db.QueryRow(ctx, `
		SELECT COUNT(*)::int,
		       COUNT(*) FILTER (WHERE x.points >= $2::numeric)::int
		FROM (
			SELECT account_id, SUM(pap) AS points
			FROM attendance_alliance_pap_snapshot
			WHERE month=$1 AND account_id IS NOT NULL
			GROUP BY account_id
		) x`, month, target).Scan(&eligible, &achieved)
	return
}

func (q *Queries) AlliancePAPSync(ctx context.Context) (AlliancePAPSync, error) {
	var r AlliancePAPSync
	err := q.db.QueryRow(ctx, `
		SELECT COALESCE(month, DATE '0001-01-01'), state, complete, records_total,
		       COALESCE(last_synced_at, '0001-01-01'::timestamptz), COALESCE(last_error, ''), version
		FROM attendance_alliance_pap_sync WHERE singleton`).Scan(
		&r.Month, &r.State, &r.Complete, &r.RecordsTotal, &r.LastSyncedAt, &r.LastError, &r.Version,
	)
	return r, err
}

func (q *Queries) AlliancePAPSyncHistories(ctx context.Context) ([]AlliancePAPSyncHistory, error) {
	rows, err := q.db.Query(ctx, `
		SELECT month, state, complete, records_total,
		       COALESCE(last_synced_at, '0001-01-01'::timestamptz),
		       COALESCE(last_error, ''), version
		FROM attendance_alliance_pap_sync_history
		WHERE state='ready' AND complete
		ORDER BY month DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AlliancePAPSyncHistory{}
	for rows.Next() {
		var item AlliancePAPSyncHistory
		if err := rows.Scan(&item.Month, &item.State, &item.Complete, &item.RecordsTotal, &item.LastSyncedAt, &item.LastError, &item.Version); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (q *Queries) LockAlliancePAPSyncHistory(ctx context.Context, month time.Time) (AlliancePAPSyncHistory, error) {
	var r AlliancePAPSyncHistory
	err := q.db.QueryRow(ctx, `
		SELECT month, state, complete, records_total,
		       COALESCE(last_synced_at, '0001-01-01'::timestamptz),
		       COALESCE(last_error, ''), version
		FROM attendance_alliance_pap_sync_history
		WHERE month=$1 FOR UPDATE`, month).Scan(
		&r.Month, &r.State, &r.Complete, &r.RecordsTotal, &r.LastSyncedAt, &r.LastError, &r.Version,
	)
	return r, err
}

func (q *Queries) MarkAlliancePAPHistoryReady(ctx context.Context, month time.Time, records int32) error {
	_, err := q.db.Exec(ctx, `
		INSERT INTO attendance_alliance_pap_sync_history
		  (month, state, complete, records_total, last_synced_at, last_error, version)
		VALUES ($1, 'ready', true, $2, now(), '', 1)
		ON CONFLICT (month) DO UPDATE SET
		  state='ready', complete=true, records_total=EXCLUDED.records_total,
		  last_synced_at=EXCLUDED.last_synced_at, last_error='',
		  version=attendance_alliance_pap_sync_history.version+1`, month, records)
	return err
}

func (q *Queries) MarkAlliancePAPSyncing(ctx context.Context, month time.Time) error {
	_, err := q.db.Exec(ctx, `
		UPDATE attendance_alliance_pap_sync
		SET month=$1, state='syncing', complete=false, last_error='', version=version+1
		WHERE singleton`, month)
	return err
}

func (q *Queries) SaveAlliancePAP(ctx context.Context, month time.Time, characterID int64, name, pap string, accountID *pgtype.UUID) error {
	var account any
	if accountID != nil && accountID.Valid {
		account = *accountID
	}
	_, err := q.db.Exec(ctx, `
		INSERT INTO attendance_alliance_pap_snapshot(month, character_id, character_name, pap, account_id)
		VALUES ($1,$2,$3,$4::numeric,$5)
		ON CONFLICT (month, character_id) DO UPDATE SET
		  character_name=EXCLUDED.character_name,
		  pap=EXCLUDED.pap,
		  account_id=COALESCE(attendance_alliance_pap_snapshot.account_id, EXCLUDED.account_id),
		  synced_at=now()
		WHERE attendance_alliance_pap_snapshot.character_name IS DISTINCT FROM EXCLUDED.character_name
		   OR attendance_alliance_pap_snapshot.pap IS DISTINCT FROM EXCLUDED.pap
		   OR (EXCLUDED.account_id IS NOT NULL AND attendance_alliance_pap_snapshot.account_id IS DISTINCT FROM EXCLUDED.account_id)`, month, characterID, name, pap, account)
	return err
}

// PruneAlliancePAP removes rows that disappeared from a complete upstream
// snapshot. The caller runs this inside the same transaction as the changed
// row upserts, so readers never observe a partially published month.
func (q *Queries) PruneAlliancePAP(ctx context.Context, month time.Time, characterIDs []int64) error {
	_, err := q.db.Exec(ctx, `
		DELETE FROM attendance_alliance_pap_snapshot
		WHERE month=$1 AND NOT (character_id = ANY($2::bigint[]))`, month, characterIDs)
	return err
}

func (q *Queries) MarkAlliancePAPReady(ctx context.Context, month time.Time, total int32) error {
	_, err := q.db.Exec(ctx, `
		UPDATE attendance_alliance_pap_sync
		SET month=$1, state='ready', complete=true, records_total=$2,
		    last_synced_at=now(), last_error='', version=version+1
		WHERE singleton`, month, total)
	return err
}

func (q *Queries) MarkAlliancePAPError(ctx context.Context, month time.Time, reason string) error {
	_, err := q.db.Exec(ctx, `
		UPDATE attendance_alliance_pap_sync
		SET month=$1, state='error', complete=false, last_error=$2, version=version+1
		WHERE singleton`, month, reason)
	return err
}

func (q *Queries) AlliancePAPAccount(ctx context.Context, accountID string) (AlliancePAPAccountReport, error) {
	var r AlliancePAPAccountReport
	err := q.db.QueryRow(ctx, `
		SELECT COALESCE(s.month, DATE '0001-01-01'), COALESCE(SUM(p.pap)::text, '0'),
		       COALESCE(s.state, 'idle'), COALESCE(s.complete, false), COALESCE(s.records_total, 0),
		       COALESCE(s.last_synced_at, '0001-01-01'::timestamptz), COALESCE(s.last_error, ''), COALESCE(s.version, 1)
		FROM attendance_alliance_pap_sync s
		LEFT JOIN attendance_alliance_pap_snapshot p ON p.month=s.month AND p.account_id=$1::uuid
		WHERE s.singleton
		GROUP BY s.month,s.state,s.complete,s.records_total,s.last_synced_at,s.last_error,s.version`, accountID).Scan(
		&r.Month, &r.Points, &r.State, &r.Complete, &r.RecordsTotal, &r.LastSyncedAt, &r.LastError, &r.Version,
	)
	return r, err
}

func (q *Queries) AlliancePAPCharacters(ctx context.Context, accountID string) ([]AlliancePAPCharacter, error) {
	rows, err := q.db.Query(ctx, `
		SELECT p.character_id, p.character_name, p.pap::text
		FROM attendance_alliance_pap_sync s
		JOIN attendance_alliance_pap_snapshot p ON p.month=s.month
		WHERE s.singleton AND s.state='ready' AND s.complete AND p.account_id=$1::uuid
		ORDER BY p.pap DESC, p.character_name, p.character_id`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AlliancePAPCharacter{}
	for rows.Next() {
		var item AlliancePAPCharacter
		if err := rows.Scan(&item.CharacterID, &item.CharacterName, &item.PAP); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (q *Queries) LockAlliancePAPSync(ctx context.Context) (AlliancePAPSync, error) {
	var r AlliancePAPSync
	err := q.db.QueryRow(ctx, `
		SELECT COALESCE(month, DATE '0001-01-01'), state, complete, records_total,
		       COALESCE(last_synced_at, '0001-01-01'::timestamptz), COALESCE(last_error, ''), version
		FROM attendance_alliance_pap_sync WHERE singleton FOR UPDATE`).Scan(
		&r.Month, &r.State, &r.Complete, &r.RecordsTotal, &r.LastSyncedAt, &r.LastError, &r.Version,
	)
	return r, err
}

func (q *Queries) AlliancePAPAwards(ctx context.Context) ([]AlliancePAPAward, error) {
	rows, err := q.db.Query(ctx, `
		SELECT p.character_id, p.account_id, p.pap::text
		FROM attendance_alliance_pap_sync s
		JOIN attendance_alliance_pap_snapshot p ON p.month=s.month
		WHERE s.singleton AND s.state='ready' AND s.complete AND p.account_id IS NOT NULL
		ORDER BY p.character_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AlliancePAPAward{}
	for rows.Next() {
		var item AlliancePAPAward
		if err := rows.Scan(&item.CharacterID, &item.AccountID, &item.PAP); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// Reads retained source totals even after a transient sync failure.
func (q *Queries) AllianceMonthAwards(ctx context.Context, month time.Time) ([]AlliancePAPAward, error) {
	rows, err := q.db.Query(ctx, `SELECT character_id,account_id,pap::text FROM attendance_alliance_pap_snapshot WHERE month=$1 ORDER BY character_id`, month)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AlliancePAPAward{}
	for rows.Next() {
		var v AlliancePAPAward
		if err = rows.Scan(&v.CharacterID, &v.AccountID, &v.PAP); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

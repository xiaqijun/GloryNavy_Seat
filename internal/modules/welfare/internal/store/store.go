// Package store owns welfare persistence; callers use the welfare service.
package store

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"time"
)

type DB interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}
type Case struct {
	ID            int64           `json:"id,string"`
	Reference     string          `json:"reference"`
	AccountID     string          `json:"account_id"`
	CorporationID int64           `json:"corporation_id,string"`
	Kind          string          `json:"kind"`
	State         string          `json:"state"`
	Version       int64           `json:"version,string"`
	Detail        json.RawMessage `json:"detail"`
	Award         int64           `json:"award_minor"`
	Keys          []string        `json:"claim_keys"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}
type ApprovalSnapshotRow struct {
	Case
	OccurredAt  time.Time
	ProcessedBy []string
	History     bool
}
type Policy struct {
	CorporationID int64           `json:"corporation_id,string"`
	Kind          string          `json:"kind"`
	Version       int64           `json:"version,string"`
	Config        json.RawMessage `json:"config"`
}
type Member struct {
	AccountID string            `json:"account_id"`
	Verified  bool              `json:"verified"`
	History   map[string]string `json:"history"`
	Months    []string          `json:"months"`
	Version   int64             `json:"version,string"`
}

const cols = "id,account_id::text,corporation_id,kind,state,version,detail,award_minor,claim_keys,created_at,updated_at,settlement_reference"

func scan(r pgx.Row) (c Case, e error) {
	e = r.Scan(&c.ID, &c.AccountID, &c.CorporationID, &c.Kind, &c.State, &c.Version, &c.Detail, &c.Award, &c.Keys, &c.CreatedAt, &c.UpdatedAt, &c.Reference)
	return
}
func Lock(ctx context.Context, db DB) error {
	_, e := db.Exec(ctx, `SELECT pg_advisory_xact_lock(740031,1)`)
	return e
}

// LockActivity serializes activity applications without waiting behind the
// delivery/review lock. Activity applications only publish a new submitted
// case and their idempotency audit; they do not claim a delivery entitlement
// or recalculate an existing welfare case.
func LockActivity(ctx context.Context, db DB) error {
	_, e := db.Exec(ctx, `SELECT pg_advisory_xact_lock(740031,2)`)
	return e
}
func Read(ctx context.Context, db DB, id int64) (Case, error) {
	return scan(db.QueryRow(ctx, "SELECT "+cols+" FROM welfare_cases WHERE id=$1", id))
}
func List(ctx context.Context, db DB, corp int64, owner, kind string, before int64, allowed []string) ([]Case, error) {
	rows, e := db.Query(ctx, "SELECT "+cols+" FROM welfare_cases WHERE corporation_id=$1 AND ($2='' OR account_id::text=$2) AND ($3='' OR kind=$3 OR ($3='growth' AND starts_with(kind,'growth_')) OR ($3='activity' AND starts_with(kind,'activity_'))) AND id<$4 AND ($2<>'' OR account_id=ANY($5::uuid[])) ORDER BY id DESC LIMIT 31", corp, owner, kind, before, allowed)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Case{}
	for rows.Next() {
		c, e := scan(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Snapshot returns the compact source-owned rows used by the approval index.
// Authorization is deliberately absent here; the approval service applies the
// actor's corporation scope when reading the central index.
func Snapshot(ctx context.Context, db DB) ([]ApprovalSnapshotRow, error) {
	rows, e := db.Query(ctx, "SELECT "+cols+",coalesce((SELECT array_agg(DISTINCT actor_id::text) FROM welfare_audit WHERE case_id=c.id AND action IN ('approve','reject','information','approve_cancel','reject_cancel','complete','release_coins','cancel','void')),'{}'::text[]),coalesce((a.actor_id IS NOT NULL OR c.state IN ('completed','cancelled','rejected','reversed') OR c.detail->>'payment_status'='awaiting_acceptance'),false),CASE WHEN a.actor_id IS NOT NULL OR c.state IN ('completed','cancelled','rejected','reversed') OR c.detail->>'payment_status'='awaiting_acceptance' THEN coalesce(a.audit_at,c.updated_at,c.created_at) ELSE coalesce((SELECT max(created_at) FROM welfare_audit WHERE case_id=c.id AND action IN ('apply','resubmit')),c.created_at) END FROM welfare_cases c LEFT JOIN LATERAL (SELECT actor_id,created_at AS audit_at FROM welfare_audit WHERE case_id=c.id AND (action IN ('approve','reject','information','approve_cancel','reject_cancel','complete','release_coins','cancel','void') OR action='delivery_check' AND result->>'state'='completed') ORDER BY created_at DESC,id DESC LIMIT 1) a ON true WHERE c.kind<>'grant' ORDER BY c.id")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []ApprovalSnapshotRow{}
	for rows.Next() {
		var c ApprovalSnapshotRow
		if e := rows.Scan(&c.ID, &c.AccountID, &c.CorporationID, &c.Kind, &c.State, &c.Version, &c.Detail, &c.Award, &c.Keys, &c.CreatedAt, &c.UpdatedAt, &c.Reference, &c.ProcessedBy, &c.History, &c.OccurredAt); e != nil {
			return nil, e
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func Save(ctx context.Context, db DB, c Case) (Case, error) {
	if c.ID == 0 {
		return scan(db.QueryRow(ctx, "INSERT INTO welfare_cases(account_id,corporation_id,kind,state,detail,award_minor,claim_keys) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING "+cols, c.AccountID, c.CorporationID, c.Kind, c.State, c.Detail, c.Award, c.Keys))
	}
	return scan(db.QueryRow(ctx, "UPDATE welfare_cases SET state=$2,detail=$3,award_minor=$4,claim_keys=$5,version=version+1,updated_at=now() WHERE id=$1 AND version=$6 RETURNING "+cols, c.ID, c.State, c.Detail, c.Award, c.Keys, c.Version))
}

// ApprovedLossTotal uses the immutable approval audit timestamp. A case's
// updated_at changes during delivery and must never move an award to a new period.
func ApprovedLossTotal(ctx context.Context, db DB, account string, corp int64, kind string, start, end time.Time) (int64, error) {
	var total int64
	err := db.QueryRow(ctx, `SELECT COALESCE(SUM(c.award_minor),0)
		FROM welfare_cases c
		WHERE c.account_id=$1 AND c.corporation_id=$2 AND c.kind=$3
		AND c.state IN ('approved','executing','cancel_requested','completed')
		AND EXISTS (SELECT 1 FROM welfare_audit a WHERE a.case_id=c.id AND a.action='approve'
			AND a.created_at >= $4 AND a.created_at < $5)`, account, corp, kind, start, end).Scan(&total)
	return total, err
}

// LossQuotaUsage returns the approved amount for both cash-loss kinds in the
// requested periods. It uses the same approval timestamp and states as the
// atomic quota check, so the member-facing summary cannot drift from
// enforcement.
type LossQuotaUsage struct {
	Kind    string
	Daily   int64
	Weekly  int64
	Monthly int64
}

type PendingLoss struct {
	ID             int64
	Kind           string
	Amount         int64
	BaseMinor      int64
	ValuationMinor int64
	CreatedAt      time.Time
}

func PendingLosses(ctx context.Context, db DB, account string, corp int64) ([]PendingLoss, error) {
	rows, err := db.Query(ctx, `
		SELECT id, kind, COALESCE(award_minor,0),
			COALESCE(NULLIF(detail->>'base_minor','')::bigint,0),
			COALESCE(NULLIF(detail #>> '{valuation,amount_minor}','')::bigint,0), created_at
		FROM welfare_cases
		WHERE account_id=$1 AND corporation_id=$2 AND kind IN ('srp','solo')
		  AND state IN ('submitted','information','external')
		ORDER BY id`, account, corp)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PendingLoss{}
	for rows.Next() {
		var v PendingLoss
		if err = rows.Scan(&v.ID, &v.Kind, &v.Amount, &v.BaseMinor, &v.ValuationMinor, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func ApprovedLossTotals(ctx context.Context, db DB, account string, corp int64, dayStart, dayEnd, weekStart, weekEnd, monthStart, monthEnd time.Time) (map[string]LossQuotaUsage, error) {
	rows, err := db.Query(ctx, `
		SELECT c.kind,
			COALESCE(SUM(c.award_minor) FILTER (WHERE EXISTS (
				SELECT 1 FROM welfare_audit a WHERE a.case_id=c.id AND a.action='approve' AND a.created_at >= $3 AND a.created_at < $4
			)),0),
			COALESCE(SUM(c.award_minor) FILTER (WHERE EXISTS (
				SELECT 1 FROM welfare_audit a WHERE a.case_id=c.id AND a.action='approve' AND a.created_at >= $5 AND a.created_at < $6
			)),0),
			COALESCE(SUM(c.award_minor) FILTER (WHERE EXISTS (
				SELECT 1 FROM welfare_audit a WHERE a.case_id=c.id AND a.action='approve' AND a.created_at >= $7 AND a.created_at < $8
			)),0)
		FROM welfare_cases c
		WHERE c.account_id=$1 AND c.corporation_id=$2 AND c.kind IN ('srp','solo')
		  AND c.state IN ('approved','executing','cancel_requested','completed')
		GROUP BY c.kind`, account, corp, dayStart, dayEnd, weekStart, weekEnd, monthStart, monthEnd)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]LossQuotaUsage{}
	for rows.Next() {
		var v LossQuotaUsage
		if err = rows.Scan(&v.Kind, &v.Daily, &v.Weekly, &v.Monthly); err != nil {
			return nil, err
		}
		out[v.Kind] = v
	}
	return out, rows.Err()
}
func Policies(ctx context.Context, db DB, corp int64) ([]Policy, error) {
	rows, e := db.Query(ctx, `SELECT corporation_id,kind,version,config FROM welfare_policies WHERE corporation_id=$1 ORDER BY kind`, corp)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Policy{}
	for rows.Next() {
		var p Policy
		if e = rows.Scan(&p.CorporationID, &p.Kind, &p.Version, &p.Config); e != nil {
			return nil, e
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
func SavePolicy(ctx context.Context, db DB, p Policy) (Policy, error) {
	e := db.QueryRow(ctx, `INSERT INTO welfare_policies(corporation_id,kind,config) SELECT $1,$2,$3 WHERE $4::bigint=0 ON CONFLICT DO NOTHING RETURNING version`, p.CorporationID, p.Kind, p.Config, p.Version).Scan(&p.Version)
	if e == pgx.ErrNoRows {
		e = db.QueryRow(ctx, `UPDATE welfare_policies SET config=$3,version=version+1 WHERE corporation_id=$1 AND kind=$2 AND version=$4 RETURNING version`, p.CorporationID, p.Kind, p.Config, p.Version).Scan(&p.Version)
	}
	return p, e
}
func Profile(ctx context.Context, db DB, account string) (Member, error) {
	m := Member{AccountID: account, History: map[string]string{}, Months: []string{}}
	e := db.QueryRow(ctx, `SELECT verified,history,months,version FROM welfare_members WHERE account_id=$1`, account).Scan(&m.Verified, &m.History, &m.Months, &m.Version)
	if e == pgx.ErrNoRows {
		e = nil
	}
	return m, e
}
func SaveProfile(ctx context.Context, db DB, m Member) error {
	_, e := db.Exec(ctx, `INSERT INTO welfare_members(account_id,verified,history,months) VALUES($1,$2,$3,$4) ON CONFLICT(account_id) DO UPDATE SET verified=$2,history=$3,months=$4,version=welfare_members.version+1`, m.AccountID, m.Verified, m.History, m.Months)
	return e
}
func Claim(ctx context.Context, db DB, key string, id int64) error {
	_, e := db.Exec(ctx, `INSERT INTO welfare_claims(claim_key,case_id) VALUES($1,$2)`, key, id)
	return e
}
func Claimed(ctx context.Context, db DB, key string) (bool, error) {
	var yes bool
	e := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM welfare_claims WHERE claim_key=$1)`, key).Scan(&yes)
	return yes, e
}
func Release(ctx context.Context, db DB, id int64) error {
	_, e := db.Exec(ctx, `DELETE FROM welfare_claims WHERE case_id=$1`, id)
	return e
}
func Replay(ctx context.Context, db DB, actor, key string) (string, json.RawMessage, error) {
	var fp string
	var out json.RawMessage
	e := db.QueryRow(ctx, `SELECT fingerprint,result FROM welfare_audit WHERE actor_id=$1 AND request_key=$2`, actor, key).Scan(&fp, &out)
	return fp, out, e
}
func Audit(ctx context.Context, db DB, actor, key, fp, action, note string, id int64, result any) error {
	data, e := json.Marshal(result)
	if e != nil {
		return e
	}
	_, e = db.Exec(ctx, `INSERT INTO welfare_audit(actor_id,request_key,fingerprint,case_id,action,note,result) VALUES($1,$2,$3,NULLIF($4,0),$5,$6,$7)`, actor, key, fp, id, action, note, data)
	return e
}
func History(ctx context.Context, db DB, id int64) ([]json.RawMessage, error) {
	rows, e := db.Query(ctx, `SELECT jsonb_build_object('action',action,'note',note,'actor_id',actor_id,'created_at',created_at,'snapshot',result) FROM welfare_audit WHERE case_id=$1 ORDER BY id`, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []json.RawMessage{}
	for rows.Next() {
		var v json.RawMessage
		if e = rows.Scan(&v); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func GrantAudit(ctx context.Context, db DB, actor string, id int64, note string, data json.RawMessage) error {
	_, e := db.Exec(ctx, `INSERT INTO welfare_audit(actor_id,request_key,fingerprint,case_id,action,note,result) VALUES($1,gen_random_uuid(),'grant-line',$2,'grant',$3,$4)`, actor, id, note, data)
	return e
}

// Release only the loss entitlement; delivery contract claims remain reserved.
func ReleaseLoss(ctx context.Context, db DB, id int64) error {
	_, e := db.Exec(ctx, `DELETE FROM welfare_claims WHERE case_id=$1 AND claim_key LIKE 'km:%'`, id)
	return e
}

// Keep delivery reservations even when an unfulfilled entitlement is cancelled.
func ReleaseEntitlements(ctx context.Context, db DB, id int64) error {
	_, e := db.Exec(ctx, `DELETE FROM welfare_claims WHERE case_id=$1 AND claim_key NOT LIKE 'delivery:%'`, id)
	return e
}

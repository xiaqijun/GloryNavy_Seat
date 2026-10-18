// Package store owns reminder configuration and delivery receipts, never approval state.
package store

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"time"
)

type DBTX interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}
type Settings struct {
	Events   []string             `json:"events"`
	Started  map[string]time.Time `json:"-"`
	Enabled  bool                 `json:"enabled"`
	SenderID int64                `json:"sender_id,string"`
	Revision int64                `json:"revision,string"`
	Account  string               `json:"-"`
	Owner    []byte               `json:"-"`
}

func ReadSettings(ctx context.Context, db DBTX, lock bool) (Settings, error) {
	query := `SELECT enabled,sender_id,revision,coalesce(sender_account::text,''),sender_owner,events,event_started FROM reminder_settings WHERE singleton`
	if lock {
		query += " FOR UPDATE"
	}
	var r Settings
	var started []byte
	err := db.QueryRow(ctx, query).Scan(&r.Enabled, &r.SenderID, &r.Revision, &r.Account, &r.Owner, &r.Events, &started)
	if err == nil {
		err = json.Unmarshal(started, &r.Started)
	}
	return r, err
}
func SaveSettings(ctx context.Context, tx pgx.Tx, actor string, r Settings) error {
	started, err := json.Marshal(r.Started)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE reminder_settings SET enabled=$1,sender_id=$2,revision=revision+1,sender_account=nullif($3,'')::uuid,sender_owner=$4,events=$5,event_started=$6`, r.Enabled, r.SenderID, r.Account, r.Owner, r.Events, started)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO reminder_audit(actor,action,revision) VALUES($1,'settings',$2)`, actor, r.Revision+1)
	return err
}

// Only call after a successful complete read of current permissions and pending phases.
func Reconcile(ctx context.Context, tx pgx.Tx, account string, keys []string, event ...string) ([]string, error) {
	kind := "approval_pending"
	if len(event) > 0 {
		kind = event[0]
	}
	if keys == nil {
		keys = []string{}
	}
	_, err := tx.Exec(ctx, `UPDATE reminder_receipts SET active=false,updated_at=now() WHERE account_id=$1 AND event=$3 AND active AND NOT(reference=ANY($2::text[]))`, account, keys, kind)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO reminder_receipts(account_id,reference,event) SELECT $1,unnest($2::text[]),$3 ON CONFLICT(account_id,reference) DO UPDATE SET active=true,generation=reminder_receipts.generation+CASE WHEN reminder_receipts.active OR reminder_receipts.event<>'approval_pending' THEN 0 ELSE 1 END,updated_at=now()`, account, keys, kind)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT reference FROM reminder_receipts WHERE account_id=$1 AND event=$2 AND active AND attempted_generation<generation ORDER BY reference`, account, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var k string
		if err = rows.Scan(&k); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

type Delivery struct {
	ID            string    `json:"id"`
	Channel       string    `json:"channel"`
	Event         string    `json:"event"`
	SenderID      int64     `json:"sender_id,string"`
	RecipientID   int64     `json:"recipient_id,string"`
	RecipientName string    `json:"recipient_name"`
	Count         int       `json:"notice_count"`
	State         string    `json:"state"`
	Reason        string    `json:"reason"`
	MessageID     int64     `json:"message_id,string"`
	CreatedAt     time.Time `json:"created_at"`
}

func Claim(ctx context.Context, tx pgx.Tx, account string, sender, recipient int64, name string, count int, keys []string, event ...string) (string, error) {
	kind := "approval_pending"
	if len(event) > 0 {
		kind = event[0]
	}
	var id string
	err := tx.QueryRow(ctx, `INSERT INTO reminder_deliveries(account_id,sender_id,recipient_id,recipient_name,notice_count,state,event) VALUES($1,$2,$3,$4,$5,'sending',$6) RETURNING id::text`, account, sender, recipient, name, count, kind).Scan(&id)
	if err != nil {
		return "", err
	}
	_, err = tx.Exec(ctx, `UPDATE reminder_receipts SET attempted_generation=generation,delivery_id=$3,updated_at=now() WHERE account_id=$1 AND active AND reference=ANY($2::text[])`, account, keys, id)
	return id, err
}
func Finish(ctx context.Context, db DBTX, id, state, reason string, message int64) error {
	_, err := db.Exec(ctx, `UPDATE reminder_deliveries SET state=$2,reason=$3,message_id=$4,finished_at=now() WHERE id=$1 AND state='sending'`, id, state, reason, message)
	return err
}
func List(ctx context.Context, db DBTX, before string) ([]Delivery, error) {
	rows, err := db.Query(ctx, `SELECT id::text,channel,event,sender_id,recipient_id,recipient_name,notice_count,state,reason,message_id,created_at FROM reminder_deliveries WHERE ($1='' OR (created_at,id)<(SELECT created_at,id FROM reminder_deliveries WHERE id=nullif($1,'')::uuid)) ORDER BY created_at DESC,id DESC LIMIT 50`, before)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Delivery{}
	for rows.Next() {
		var r Delivery
		if err = rows.Scan(&r.ID, &r.Channel, &r.Event, &r.SenderID, &r.RecipientID, &r.RecipientName, &r.Count, &r.State, &r.Reason, &r.MessageID, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func Retry(ctx context.Context, tx pgx.Tx, id, actor string) error {
	var account string
	err := tx.QueryRow(ctx, `UPDATE reminder_deliveries SET state='retry_requested' WHERE id=$1 AND state='failed' RETURNING account_id::text`, id).Scan(&account)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE reminder_receipts SET attempted_generation=0 WHERE account_id=$1 AND delivery_id=$2 AND active`, account, id)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO reminder_audit(actor,action,revision) SELECT $1,'retry:'||$2::text,revision FROM reminder_settings`, actor, id)
	return err
}
func Maintain(ctx context.Context, db DBTX) error {
	_, err := db.Exec(ctx, `UPDATE reminder_deliveries SET state='unknown',reason='interrupted',finished_at=now() WHERE state='sending' AND created_at<now()-interval '5 minutes'`)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, `DELETE FROM reminder_receipts WHERE NOT active AND updated_at<now()-interval '90 days'`)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, `DELETE FROM reminder_deliveries WHERE state<>'sending' AND created_at<now()-interval '90 days'`)
	return err
}
func RateLimited(ctx context.Context, db DBTX, account string) (bool, error) {
	var b bool
	err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM reminder_deliveries WHERE account_id=$1 AND created_at>now()-interval '5 minutes')`, account).Scan(&b)
	return b, err
}
func Lock(ctx context.Context, db DBTX, account string) (bool, error) {
	var ok bool
	err := db.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtextextended('reminder:'||$1::text,0))`, account).Scan(&ok)
	return ok, err
}
func Unlock(ctx context.Context, db DBTX, account string) error {
	_, err := db.Exec(ctx, `SELECT pg_advisory_unlock(hashtextextended('reminder:'||$1::text,0))`, account)
	return err
}

func Merge(ctx context.Context, tx pgx.Tx, source, target string, apply bool) (int, error) {
	var n int
	err := tx.QueryRow(ctx, `SELECT count(*) FROM reminder_deliveries WHERE account_id=$1`, source).Scan(&n)
	if err != nil || !apply {
		return n, err
	}
	// Reconcile has no source business data. Transfer receipts conservatively to prevent replay.
	_, err = tx.Exec(ctx, `INSERT INTO reminder_receipts(account_id,reference,active,generation,attempted_generation,delivery_id,event) SELECT $2,reference,active,1,CASE WHEN attempted_generation>=generation THEN 1 ELSE 0 END,delivery_id,event FROM reminder_receipts WHERE account_id=$1 ON CONFLICT(account_id,reference) DO UPDATE SET attempted_generation=CASE WHEN EXCLUDED.attempted_generation>0 THEN reminder_receipts.generation ELSE reminder_receipts.attempted_generation END`, source, target)
	if err != nil {
		return n, err
	}
	_, err = tx.Exec(ctx, `DELETE FROM reminder_receipts WHERE account_id=$1`, source)
	if err != nil {
		return n, err
	}
	_, err = tx.Exec(ctx, `UPDATE reminder_deliveries SET account_id=$2,original_account_id=coalesce(original_account_id,$1) WHERE account_id=$1`, source, target)
	if err != nil {
		return n, err
	}
	// Sender consent belongs to its original account; never transfer site privileges.
	_, err = tx.Exec(ctx, `UPDATE reminder_settings SET enabled=false,revision=revision+1 WHERE sender_account=$1`, source)
	return n, err
}

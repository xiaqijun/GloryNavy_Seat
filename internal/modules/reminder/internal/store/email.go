package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

type EmailSettings struct {
	Enabled, ImplicitTLS   bool
	Host, Username, Sender string
	Port                   int
	Secret                 []byte
	Revision               int64
	Events                 []string
	Started                map[string]time.Time
}

func ReadEmailSettings(ctx context.Context, db DBTX, lock bool) (EmailSettings, error) {
	q := `SELECT enabled,host,port,implicit_tls,username,sender,secret_ciphertext,revision,events,event_started FROM reminder_email_settings WHERE singleton`
	if lock {
		q += " FOR UPDATE"
	}
	var r EmailSettings
	var raw []byte
	err := db.QueryRow(ctx, q).Scan(&r.Enabled, &r.Host, &r.Port, &r.ImplicitTLS, &r.Username, &r.Sender, &r.Secret, &r.Revision, &r.Events, &raw)
	if err == nil {
		err = json.Unmarshal(raw, &r.Started)
	}
	return r, err
}

func SaveEmailSettings(ctx context.Context, tx pgx.Tx, actor string, r EmailSettings) error {
	started, err := json.Marshal(r.Started)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE reminder_email_settings SET enabled=$1,host=$2,port=$3,implicit_tls=$4,username=$5,sender=$6,secret_ciphertext=$7,revision=revision+1,events=$8,event_started=$9`, r.Enabled, r.Host, r.Port, r.ImplicitTLS, r.Username, r.Sender, r.Secret, r.Events, started)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO reminder_audit(actor,action,revision) VALUES($1,'email-settings',$2)`, actor, r.Revision+1)
	return err
}

type EmailAccount struct {
	Email              string     `json:"email"`
	Verified           bool       `json:"verified"`
	Enabled            bool       `json:"enabled"`
	Revision           int64      `json:"revision,string"`
	Pending            string     `json:"pending_email"`
	CodeHash, CodeSalt []byte     `json:"-"`
	Expires, LastCode  *time.Time `json:"-"`
	Attempts           int        `json:"-"`
}

func ReadEmailAccount(ctx context.Context, db DBTX, account string, lock bool) (EmailAccount, error) {
	q := `SELECT email,verified,enabled,revision,pending_email,code_hash,code_salt,code_expires,last_code_at,code_attempts FROM reminder_email_accounts WHERE account_id=$1`
	if lock {
		q += " FOR UPDATE"
	}
	var a EmailAccount
	err := db.QueryRow(ctx, q, account).Scan(&a.Email, &a.Verified, &a.Enabled, &a.Revision, &a.Pending, &a.CodeHash, &a.CodeSalt, &a.Expires, &a.LastCode, &a.Attempts)
	return a, err
}

func EnsureEmailAccount(ctx context.Context, db DBTX, account string) error {
	_, err := db.Exec(ctx, `INSERT INTO reminder_email_accounts(account_id) VALUES($1) ON CONFLICT DO NOTHING`, account)
	return err
}

func UpdateEmailChallenge(ctx context.Context, tx pgx.Tx, account, address string, hash, salt []byte, expires time.Time) error {
	_, err := tx.Exec(ctx, `UPDATE reminder_email_accounts SET pending_email=$2,code_hash=$3,code_salt=$4,code_expires=$5,code_attempts=0,last_code_at=now(),revision=revision+1 WHERE account_id=$1`, account, address, hash, salt, expires)
	return err
}

func VerifyEmailAccount(ctx context.Context, tx pgx.Tx, account string) error {
	_, err := tx.Exec(ctx, `UPDATE reminder_email_accounts SET email=pending_email,verified=true,enabled=true,pending_email='',code_hash='',code_salt='',code_expires=NULL,code_attempts=0,revision=revision+1 WHERE account_id=$1`, account)
	return err
}

func FailEmailChallenge(ctx context.Context, tx pgx.Tx, account string) error {
	_, err := tx.Exec(ctx, `UPDATE reminder_email_accounts SET code_attempts=code_attempts+1 WHERE account_id=$1`, account)
	return err
}

func SetEmailEnabled(ctx context.Context, tx pgx.Tx, account string, enabled bool) error {
	_, err := tx.Exec(ctx, `UPDATE reminder_email_accounts SET enabled=$2,revision=revision+1 WHERE account_id=$1`, account, enabled)
	return err
}

func EmailUsers(ctx context.Context, db DBTX) ([]string, error) {
	rows, err := db.Query(ctx, `SELECT account_id::text FROM reminder_email_accounts WHERE verified AND enabled ORDER BY account_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func EmailEventOrder(ctx context.Context, db DBTX, account string, events []string) ([]string, error) {
	rows, err := db.Query(ctx, `SELECT e FROM unnest($2::text[]) e LEFT JOIN LATERAL (SELECT max(created_at) t FROM reminder_email_deliveries WHERE account_id=$1 AND event=e) d ON true ORDER BY d.t NULLS FIRST,e`, account, events)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var e string
		if err = rows.Scan(&e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func ReconcileEmail(ctx context.Context, tx pgx.Tx, account string, keys []string, kind string) ([]string, error) {
	if keys == nil {
		keys = []string{}
	}
	if _, err := tx.Exec(ctx, `UPDATE reminder_email_receipts SET active=false,updated_at=now() WHERE account_id=$1 AND event=$3 AND active AND NOT(reference=ANY($2::text[]))`, account, keys, kind); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO reminder_email_receipts(account_id,reference,event) SELECT $1,unnest($2::text[]),$3 ON CONFLICT(account_id,reference) DO UPDATE SET active=true,generation=reminder_email_receipts.generation+CASE WHEN reminder_email_receipts.active OR reminder_email_receipts.event<>'approval_pending' THEN 0 ELSE 1 END,updated_at=now()`, account, keys, kind); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT reference FROM reminder_email_receipts WHERE account_id=$1 AND event=$2 AND active AND attempted_generation<generation ORDER BY reference`, account, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var key string
		if err = rows.Scan(&key); err != nil {
			return nil, err
		}
		out = append(out, key)
	}
	return out, rows.Err()
}

func ClaimEmail(ctx context.Context, tx pgx.Tx, account, recipient, event string, keys []string) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `INSERT INTO reminder_email_deliveries(account_id,recipient,event,notice_count,state) VALUES($1,$2,$3,$4,'sending') RETURNING id::text`, account, recipient, event, len(keys)).Scan(&id)
	if err != nil {
		return "", err
	}
	_, err = tx.Exec(ctx, `UPDATE reminder_email_receipts SET attempted_generation=generation,delivery_id=$3,updated_at=now() WHERE account_id=$1 AND active AND reference=ANY($2::text[])`, account, keys, id)
	return id, err
}

func FinishEmail(ctx context.Context, db DBTX, id, state, reason string) error {
	_, err := db.Exec(ctx, `UPDATE reminder_email_deliveries SET state=$2,reason=$3,finished_at=now() WHERE id=$1 AND state='sending'`, id, state, reason)
	return err
}

func EmailRateLimited(ctx context.Context, db DBTX, account string) (bool, error) {
	var b bool
	err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM reminder_email_deliveries WHERE account_id=$1 AND created_at>now()-interval '5 minutes')`, account).Scan(&b)
	return b, err
}

func MaintainEmail(ctx context.Context, db DBTX) error {
	_, err := db.Exec(ctx, `UPDATE reminder_email_deliveries SET state='unknown',reason='interrupted',finished_at=now() WHERE state='sending' AND created_at<now()-interval '5 minutes'`)
	return err
}

func MergeEmail(ctx context.Context, tx pgx.Tx, source, target string, apply bool) (int, error) {
	var n int
	err := tx.QueryRow(ctx, `SELECT count(*) FROM reminder_email_deliveries WHERE account_id=$1`, source).Scan(&n)
	if err != nil || !apply {
		return n, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO reminder_email_receipts(account_id,reference,event,active,generation,attempted_generation,delivery_id) SELECT $2,reference,event,active,1,CASE WHEN attempted_generation>=generation THEN 1 ELSE 0 END,delivery_id FROM reminder_email_receipts WHERE account_id=$1 ON CONFLICT(account_id,reference) DO UPDATE SET attempted_generation=CASE WHEN EXCLUDED.attempted_generation>0 THEN reminder_email_receipts.generation ELSE reminder_email_receipts.attempted_generation END`, source, target); err != nil {
		return 0, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM reminder_email_receipts WHERE account_id=$1`, source); err != nil {
		return 0, err
	}
	if _, err = tx.Exec(ctx, `UPDATE reminder_email_deliveries SET original_account_id=coalesce(original_account_id,account_id),account_id=$2 WHERE account_id=$1`, source, target); err != nil {
		return 0, err
	}
	_, err = tx.Exec(ctx, `UPDATE reminder_email_accounts SET enabled=false,pending_email='',code_hash='',code_salt='',code_expires=NULL,revision=revision+1 WHERE account_id=$1`, source)
	return n, err
}

type EmailDelivery struct {
	ID        string    `json:"id"`
	Recipient string    `json:"recipient"`
	Event     string    `json:"event"`
	Count     int       `json:"notice_count"`
	State     string    `json:"state"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

func ListEmail(ctx context.Context, db DBTX) ([]EmailDelivery, error) {
	rows, err := db.Query(ctx, `SELECT id::text,recipient,event,notice_count,state,reason,created_at FROM reminder_email_deliveries ORDER BY created_at DESC,id DESC LIMIT 50`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EmailDelivery{}
	for rows.Next() {
		var d EmailDelivery
		if err = rows.Scan(&d.ID, &d.Recipient, &d.Event, &d.Count, &d.State, &d.Reason, &d.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func RetryEmail(ctx context.Context, tx pgx.Tx, id, actor string) error {
	var account string
	err := tx.QueryRow(ctx, `UPDATE reminder_email_deliveries SET state='retry_requested' WHERE id=$1 AND state='failed' RETURNING account_id::text`, id).Scan(&account)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE reminder_email_receipts SET attempted_generation=0 WHERE account_id=$1 AND delivery_id=$2 AND active`, account, id); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO reminder_audit(actor,action,revision) SELECT $1,'email-retry:'||$2::text,revision FROM reminder_email_settings`, actor, id)
	return err
}

package store

import (
	"context"
	"github.com/jackc/pgx/v5"
)

type ApprovalRecipient struct {
	UserID, QQ string
	Version    int64
}

func ApprovalRecipients(ctx context.Context, db DBTX) ([]ApprovalRecipient, error) {
	rows, err := db.Query(ctx, `SELECT b.user_id::text,b.qq_number,b.qq_version FROM community_onebot_bindings b
 JOIN community_profiles p ON p.user_id=b.user_id AND p.qq_number=b.qq_number AND p.qq_version=b.qq_version
 WHERE EXISTS(SELECT 1 FROM community_confirmations c WHERE c.user_id=b.user_id AND c.platform='qq'
 AND c.field_version=b.qq_version AND c.source='onebot' AND c.invalidated_at IS NULL) ORDER BY b.user_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ApprovalRecipient{}
	for rows.Next() {
		var r ApprovalRecipient
		if err = rows.Scan(&r.UserID, &r.QQ, &r.Version); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ReconcileApprovalNotices must only follow a complete, successful source read.
func ReconcileApprovalNotices(ctx context.Context, tx pgx.Tx, group, user string, keys []string) ([]string, error) {
	if keys == nil {
		keys = []string{}
	}
	_, err := tx.Exec(ctx, `UPDATE community_approval_notices SET active=false,updated_at=now()
 WHERE group_id=$1 AND user_id=$2 AND active AND NOT(reference=ANY($3::text[]))`, group, user, keys)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO community_approval_notices(group_id,user_id,reference)
 SELECT $1,$2,unnest($3::text[]) ON CONFLICT(group_id,user_id,reference) DO UPDATE
 SET active=true,generation=community_approval_notices.generation+CASE WHEN community_approval_notices.active THEN 0 ELSE 1 END,updated_at=now()`, group, user, keys)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT reference FROM community_approval_notices
 WHERE group_id=$1 AND user_id=$2 AND active AND attempted_generation<generation ORDER BY reference`, group, user)
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

func ApprovalGroupLock(ctx context.Context, db DBTX, group string) (bool, error) {
	var ok bool
	err := db.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtextextended('community.approval:'||$1::text,0))`, group).Scan(&ok)
	return ok, err
}
func ApprovalGroupUnlock(ctx context.Context, db DBTX, group string) error {
	_, err := db.Exec(ctx, `SELECT pg_advisory_unlock(hashtextextended('community.approval:'||$1::text,0))`, group)
	return err
}
func ApprovalRateLimited(ctx context.Context, db DBTX, group string) (bool, error) {
	var limited bool
	err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM community_approval_deliveries WHERE group_id=$1 AND created_at>now()-interval '1 minute')`, group).Scan(&limited)
	return limited, err
}
func BeginApprovalDelivery(ctx context.Context, tx pgx.Tx, group string, recipients, notices int) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `INSERT INTO community_approval_deliveries(group_id,state,recipient_count,notice_count)
 VALUES($1,'sending',$2,$3) RETURNING id::text`, group, recipients, notices).Scan(&id)
	return id, err
}
func ClaimApprovalNotices(ctx context.Context, tx pgx.Tx, group, user, batch string, keys []string) error {
	_, err := tx.Exec(ctx, `UPDATE community_approval_notices SET attempted_generation=generation,batch_id=$3,updated_at=now()
 WHERE group_id=$1 AND user_id=$2 AND active AND reference=ANY($4::text[])`, group, user, batch, keys)
	return err
}
func FinishApprovalDelivery(ctx context.Context, db DBTX, batch, message string) error {
	state := "sent"
	if message == "" {
		state = "unknown"
	}
	_, err := db.Exec(ctx, `UPDATE community_approval_deliveries SET state=$2,message_id=$3,finished_at=now() WHERE id=$1 AND state='sending'`, batch, state, message)
	return err
}
func ExpireApprovalNotices(ctx context.Context, db DBTX) error {
	_, err := db.Exec(ctx, `UPDATE community_approval_deliveries SET state='unknown',finished_at=now() WHERE state='sending' AND created_at<now()-interval '5 minutes'`)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, `UPDATE community_approval_notices n SET active=false,updated_at=now() WHERE active
 AND NOT EXISTS(SELECT 1 FROM community_onebot_bindings b JOIN community_profiles p ON p.user_id=b.user_id AND p.qq_number=b.qq_number AND p.qq_version=b.qq_version
 WHERE b.user_id=n.user_id AND EXISTS(SELECT 1 FROM community_confirmations c WHERE c.user_id=b.user_id AND c.platform='qq' AND c.field_version=b.qq_version AND c.source='onebot' AND c.invalidated_at IS NULL))`)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, `DELETE FROM community_approval_notices WHERE NOT active AND updated_at<now()-interval '90 days'`)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, `DELETE FROM community_approval_deliveries WHERE state<>'sending' AND created_at<now()-interval '90 days'`)
	return err
}
func CancelAccountApprovalNotices(ctx context.Context, tx pgx.Tx, user string, apply bool) (int64, error) {
	var count int64
	err := tx.QueryRow(ctx, `SELECT count(*) FROM community_approval_notices WHERE user_id=$1 AND active`, user).Scan(&count)
	if err == nil && apply {
		_, err = tx.Exec(ctx, `UPDATE community_approval_notices SET active=false,updated_at=now() WHERE user_id=$1 AND active`, user)
	}
	return count, err
}

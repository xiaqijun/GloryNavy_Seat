package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

// QQGroupApprovalOwner lets the caller take the account lock before locking the
// pending application. The application must be read again once that lock is held.
func (q *Queries) QQGroupApprovalOwner(ctx context.Context, hash []byte) (pgtype.UUID, error) {
	var owner pgtype.UUID
	err := q.db.QueryRow(ctx, `SELECT user_id FROM community_qq_group_applications
		WHERE code_hash=$1 AND status='pending' AND expires_at>now()`, hash).Scan(&owner)
	return owner, err
}

func (q *Queries) QQBindingOpenID(ctx context.Context, user pgtype.UUID, version int64) (string, error) {
	var openID string
	err := q.db.QueryRow(ctx, `SELECT openid FROM community_qq_bindings
		WHERE user_id=$1 AND qq_version=$2 FOR SHARE`, user, version).Scan(&openID)
	return openID, err
}

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

// Callers hold the official identity lock until the reservation or binding is saved.
func (q *Queries) QQGroupIdentityReserved(ctx context.Context, user pgtype.UUID, member string) (bool, error) {
	var reserved bool
	err := q.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM community_qq_group_applications a
		JOIN community_profiles p ON p.user_id=a.user_id AND p.qq_version=a.qq_version
		WHERE a.status IN ('approving','approved')
		AND ((a.member_openid=$2 AND a.user_id<>$1) OR (a.user_id=$1 AND a.member_openid<>$2)))`, user, member).Scan(&reserved)
	return reserved, err
}

func (q *Queries) ExpireStaleQQGroupApplications(ctx context.Context, user pgtype.UUID, group string, version int64) error {
	_, err := q.db.Exec(ctx, `UPDATE community_qq_group_applications SET status='expired',updated_at=now()
		WHERE user_id=$1 AND group_openid=$2 AND qq_version<>$3 AND status IN ('pending','approving','approved')`, user, group, version)
	return err
}

func (q *Queries) QQGroupMemberApplication(ctx context.Context, group, member string) (int64, pgtype.UUID, error) {
	var id int64
	var user pgtype.UUID
	err := q.db.QueryRow(ctx, `SELECT id,user_id FROM community_qq_group_applications
		WHERE group_openid=$1 AND member_openid=$2 AND status IN ('approving','approved')
		ORDER BY id DESC LIMIT 1`, group, member).Scan(&id, &user)
	return id, user, err
}

func (q *Queries) CompleteQQGroupAdmission(ctx context.Context, id int64) error {
	_, err := q.db.Exec(ctx, `UPDATE community_qq_group_applications
		SET status='bound',approved_at=COALESCE(approved_at,now()),bound_at=now(),updated_at=now(),failure_reason=NULL
		WHERE id=$1 AND status IN ('approving','approved')`, id)
	return err
}

func (q *Queries) ExpireQQGroupApplicationsOnAppChange(ctx context.Context) error {
	_, err := q.db.Exec(ctx, `UPDATE community_qq_group_applications SET status='expired',updated_at=now()
		WHERE status IN ('pending','approving','approved')`)
	return err
}

func (q *Queries) LatestQQGroupApplicationForGroup(ctx context.Context, user pgtype.UUID, group string) (CommunityQqGroupApplication, error) {
	var app CommunityQqGroupApplication
	err := q.db.QueryRow(ctx, `SELECT id,user_id,group_openid,qq_number,qq_version,code_hash,status,member_openid,join_request_id,
		failure_reason,created_at,expires_at,approved_at,bound_at,updated_at
		FROM community_qq_group_applications WHERE user_id=$1 AND group_openid=$2
		ORDER BY id DESC LIMIT 1`, user, group).Scan(
		&app.ID, &app.UserID, &app.GroupOpenid, &app.QqNumber, &app.QqVersion, &app.CodeHash, &app.Status,
		&app.MemberOpenid, &app.JoinRequestID, &app.FailureReason, &app.CreatedAt, &app.ExpiresAt,
		&app.ApprovedAt, &app.BoundAt, &app.UpdatedAt,
	)
	return app, err
}

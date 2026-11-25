package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// QQGroupApprovalCandidate lets the caller lock the account before rereading the
// application row. After intake, only the exact QQ request can advance it.
func (q *Queries) QQGroupApprovalCandidate(ctx context.Context, group, member, request string, hash []byte) (int64, pgtype.UUID, error) {
	var id int64
	var owner pgtype.UUID
	err := q.db.QueryRow(ctx, `SELECT id,user_id FROM community_qq_group_applications WHERE group_openid=$1
		AND ((status='waiting_membership' AND member_openid=$2 AND join_request_id=$3)
		  OR (status='pending' AND code_hash=$4 AND expires_at>now()))
		ORDER BY (status='waiting_membership') DESC, id DESC LIMIT 1`, group, member, request, hash).Scan(&id, &owner)
	return id, owner, err
}

func (q *Queries) WaitForQQGroupMembership(ctx context.Context, id int64, member, request string) error {
	_, err := q.db.Exec(ctx, `UPDATE community_qq_group_applications SET status='waiting_membership',
		member_openid=$2,join_request_id=$3,updated_at=now() WHERE id=$1 AND status IN ('pending','approving')`, id, member, request)
	return err
}

func (q *Queries) QQGroupsAwaitingApplications(ctx context.Context) ([]string, error) {
	rows, err := q.db.Query(ctx, `SELECT DISTINCT group_openid FROM community_qq_group_applications
		WHERE status='waiting_membership' OR (status='pending' AND expires_at>now()) ORDER BY group_openid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var groups []string
	for rows.Next() {
		var group string
		if err := rows.Scan(&group); err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	return groups, rows.Err()
}

func (q *Queries) ExpireMissingQQGroupRequests(ctx context.Context, group string, cutoff time.Time, observed []string) error {
	_, err := q.db.Exec(ctx, `UPDATE community_qq_group_applications SET status='expired',updated_at=now()
		WHERE group_openid=$1 AND status='waiting_membership' AND updated_at<$2
		  AND NOT (join_request_id=ANY($3::text[]))`, group, cutoff, observed)
	return err
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
		WHERE user_id=$1 AND group_openid=$2 AND qq_version<>$3 AND status IN ('pending','waiting_membership','approving','approved')`, user, group, version)
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
		WHERE status IN ('pending','waiting_membership','approving','approved')`)
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

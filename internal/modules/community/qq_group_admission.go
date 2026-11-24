package community

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/community/internal/store"
)

func (s *Service) requireQQGroupMember(ctx context.Context, user string) error {
	if s.QQGroupMember == nil {
		return ErrQQGroupMemberRequired
	}
	allowed, err := s.QQGroupMember(ctx, user)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrQQGroupMemberRequired
	}
	return nil
}

func (s *Service) lockQQGroupScope(ctx context.Context, q *store.Queries, bot *OfficialQQBot, group string) error {
	if err := s.verifyQQGatewayConfig(ctx, q, bot); err != nil {
		return err
	}
	enabled, err := q.QQGatewayGroupEnabled(ctx, group)
	if err != nil {
		return err
	}
	if !enabled {
		return ErrQQGroupNotConfigured
	}
	return nil
}

func (s *Service) checkQQGroupApplicant(ctx context.Context, q *store.Queries, app store.CommunityQqGroupApplication, member string) error {
	profile, err := q.LockProfile(ctx, app.UserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrQQNotBound
	}
	if err != nil {
		return err
	}
	if profile.QqNumber != app.QqNumber || profile.QqVersion != app.QqVersion {
		return ErrQQNotBound
	}
	openID, err := q.QQBindingOpenID(ctx, app.UserID, app.QqVersion)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if openID != "" && openID != member {
		return ErrQQOfficialConflict
	}
	other, err := q.FindQQBinding(ctx, member)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if err == nil && other.UserID != app.UserID {
		return ErrQQOfficialConflict
	}
	reserved, err := q.QQGroupIdentityReserved(ctx, app.UserID, member)
	if err != nil {
		return err
	}
	if reserved {
		return ErrQQOfficialConflict
	}
	return s.requireQQGroupMember(ctx, app.UserID.String())
}

func (s *Service) approveQQGroupApplicant(ctx context.Context, bot *OfficialQQBot, app store.CommunityQqGroupApplication, request OfficialQQGroupJoinRequest) error {
	guard, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer guard.Rollback(context.Background())
	q := store.New(guard)
	if err = s.lockQQGroupScope(ctx, q, bot, request.GroupOpenID); err != nil {
		return err
	}
	if err = q.LockOfficialQQIdentity(ctx, request.MemberOpenID); err != nil {
		return err
	}
	if err = s.LockAccounts(ctx, guard, []string{app.UserID.String()}); err != nil {
		return err
	}
	current, err := q.LockQQGroupApplication(ctx, app.ID)
	if err != nil {
		return err
	}
	if current.UserID != app.UserID ||
		current.GroupOpenid != request.GroupOpenID || current.MemberOpenid.String != request.MemberOpenID ||
		current.JoinRequestID.String != request.JoinRequestID {
		return errors.New("QQ group application is no longer current")
	}
	if current.Status == "bound" {
		return nil
	}
	if current.Status != "approving" || !current.ExpiresAt.Time.After(time.Now()) {
		return errors.New("QQ group application is no longer current")
	}
	if err = s.checkQQGroupApplicant(ctx, q, app, request.MemberOpenID); err != nil {
		return err
	}
	outbound := bot.callbackSnapshot()
	outbound.SetGroups([]string{request.GroupOpenID})
	return outbound.ApproveGroupJoinRequest(ctx, request.GroupOpenID, request.MemberOpenID, request.JoinRequestID)
}

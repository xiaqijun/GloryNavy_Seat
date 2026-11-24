package community

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"glorynavy.local/seat/internal/modules/community/internal/store"
)

const qqGroupApplicationTTL = 15 * time.Minute

var qqGroupCodePattern = regexp.MustCompile(`[A-Z2-9]{8}`)

type QQGroupApplication struct {
	ID            int64      `json:"id"`
	GroupOpenID   string     `json:"group_openid"`
	QQNumber      string     `json:"qq_number"`
	Status        string     `json:"status"`
	MemberOpenID  string     `json:"member_openid,omitempty"`
	JoinRequestID string     `json:"join_request_id,omitempty"`
	FailureReason string     `json:"failure_reason,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	ExpiresAt     time.Time  `json:"expires_at"`
	ApprovedAt    *time.Time `json:"approved_at,omitempty"`
	BoundAt       *time.Time `json:"bound_at,omitempty"`
}

type QQGroupApplicationChallenge struct {
	QQGroupApplication
	Code string `json:"code"`
}

func validQQGroupOpenID(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && len(value) <= 256 && !strings.ContainsAny(value, "\r\n") && !numericQQGroupID(value)
}

func numericQQGroupID(value string) bool {
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return value != ""
}

func qqGroupCodeFromVerifyInfo(info officialQQVerifyInfo) string {
	values := []string{info.VerifyMessage}
	for _, item := range info.ReviewQAList {
		values = append(values, item.Answer)
	}
	for _, value := range values {
		if code := qqGroupCodePattern.FindString(strings.ToUpper(value)); code != "" {
			return code
		}
	}
	return ""
}

func applicationFromRow(row store.CommunityQqGroupApplication) QQGroupApplication {
	return QQGroupApplication{
		ID: row.ID, GroupOpenID: row.GroupOpenid, QQNumber: row.QqNumber, Status: row.Status,
		MemberOpenID: row.MemberOpenid.String, JoinRequestID: row.JoinRequestID.String,
		FailureReason: row.FailureReason.String, CreatedAt: row.CreatedAt.Time, ExpiresAt: row.ExpiresAt.Time,
		ApprovedAt: optionalTime(row.ApprovedAt), BoundAt: optionalTime(row.BoundAt),
	}
}

func applicationFromListRow(row store.ListQQGroupApplicationsRow) QQGroupApplication {
	return QQGroupApplication{
		ID: row.ID, GroupOpenID: row.GroupOpenid, QQNumber: row.QqNumber, Status: row.Status,
		MemberOpenID: row.MemberOpenid.String, JoinRequestID: row.JoinRequestID.String,
		FailureReason: row.FailureReason.String, CreatedAt: row.CreatedAt.Time, ExpiresAt: row.ExpiresAt.Time,
		ApprovedAt: optionalTime(row.ApprovedAt), BoundAt: optionalTime(row.BoundAt),
	}
}

func optionalTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	v := value.Time
	return &v
}

func (s *Service) CreateQQGroupApplication(ctx context.Context, userID, groupOpenID string) (QQGroupApplicationChallenge, error) {
	groupOpenID = strings.TrimSpace(groupOpenID)
	if groupOpenID == "" && s.qqOfficial != nil && len(s.qqOfficial.ConfiguredGroups()) == 1 {
		groupOpenID = s.qqOfficial.ConfiguredGroups()[0]
	}
	if s.qqOfficial == nil || !s.qqOfficial.GroupAllowed(groupOpenID) {
		return QQGroupApplicationChallenge{}, ErrQQGroupNotConfigured
	}
	if err := s.requireQQGroupMember(ctx, userID); err != nil {
		return QQGroupApplicationChallenge{}, err
	}
	bot := s.qqOfficial.callbackSnapshot()
	if bot == nil || s.LockAccounts == nil {
		return QQGroupApplicationChallenge{}, ErrQQOfficialDisabled
	}
	var user pgtype.UUID
	if err := user.Scan(userID); err != nil {
		return QQGroupApplicationChallenge{}, err
	}
	code, hash, err := newQQChallenge()
	if err != nil {
		return QQGroupApplicationChallenge{}, err
	}
	expires := time.Now().UTC().Add(qqGroupApplicationTTL)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return QQGroupApplicationChallenge{}, err
	}
	defer tx.Rollback(context.Background())
	q := store.New(tx)
	if err = s.lockQQGroupScope(ctx, q, bot, groupOpenID); err != nil {
		return QQGroupApplicationChallenge{}, err
	}
	if err = s.LockAccounts(ctx, tx, []string{userID}); err != nil {
		return QQGroupApplicationChallenge{}, err
	}
	if err = q.EnsureProfile(ctx, user); err != nil {
		return QQGroupApplicationChallenge{}, err
	}
	profileRow, err := q.LockProfile(ctx, user)
	if err != nil {
		return QQGroupApplicationChallenge{}, err
	}
	if profileRow.QqNumber == "" {
		return QQGroupApplicationChallenge{}, ErrQQNotBound
	}
	if err = q.ExpireQQGroupApplications(ctx); err != nil {
		return QQGroupApplicationChallenge{}, err
	}
	if err = q.ExpireStaleQQGroupApplications(ctx, user, groupOpenID, profileRow.QqVersion); err != nil {
		return QQGroupApplicationChallenge{}, err
	}
	if _, err = q.LockActiveQQGroupApplication(ctx, store.LockActiveQQGroupApplicationParams{UserID: user, GroupOpenid: groupOpenID}); err == nil {
		return QQGroupApplicationChallenge{}, ErrQQGroupApplicationExists
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return QQGroupApplicationChallenge{}, err
	}
	row, err := q.InsertQQGroupApplication(ctx, store.InsertQQGroupApplicationParams{UserID: user, GroupOpenid: groupOpenID, QqNumber: profileRow.QqNumber, QqVersion: profileRow.QqVersion, CodeHash: hash, ExpiresAt: pgtype.Timestamptz{Time: expires, Valid: true}})
	if err != nil {
		return QQGroupApplicationChallenge{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return QQGroupApplicationChallenge{}, err
	}
	return QQGroupApplicationChallenge{QQGroupApplication: QQGroupApplication{ID: row.ID, GroupOpenID: row.GroupOpenid, QQNumber: row.QqNumber, Status: row.Status, ExpiresAt: row.ExpiresAt.Time}, Code: code}, nil
}

func (s *Service) QQGroupApplication(ctx context.Context, userID string) (QQGroupApplication, error) {
	return s.qqGroupApplication(ctx, userID, "")
}

func (s *Service) qqGroupApplication(ctx context.Context, userID, group string) (QQGroupApplication, error) {
	var user pgtype.UUID
	if err := user.Scan(userID); err != nil {
		return QQGroupApplication{}, err
	}
	q := store.New(s.pool)
	var row store.CommunityQqGroupApplication
	var err error
	if group != "" {
		row, err = q.LatestQQGroupApplicationForGroup(ctx, user, group)
	} else {
		row, err = q.LatestQQGroupApplication(ctx, user)
	}
	if err != nil {
		return QQGroupApplication{}, err
	}
	if row.Status == "pending" && !row.ExpiresAt.Time.After(time.Now()) {
		row.Status = "expired"
	}
	return applicationFromRow(row), nil
}

func (s *Service) ListQQGroupApplications(ctx context.Context, groupOpenID, status string, limit int32) ([]QQGroupApplication, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	rows, err := store.New(s.pool).ListQQGroupApplications(ctx, store.ListQQGroupApplicationsParams{Column1: strings.TrimSpace(groupOpenID), Column2: strings.TrimSpace(status), Limit: limit})
	if err != nil {
		return nil, err
	}
	items := make([]QQGroupApplication, 0, len(rows))
	for _, row := range rows {
		items = append(items, applicationFromListRow(row))
	}
	return items, nil
}

func (s *Service) ProcessQQGroupJoinRequest(ctx context.Context, eventID string, request OfficialQQGroupJoinRequest, payload []byte, verified ...*OfficialQQBot) error {
	if s.qqOfficial == nil || (len(verified) == 0 && !s.qqOfficial.GroupAllowed(request.GroupOpenID)) || request.MemberOpenID == "" || request.JoinRequestID == "" {
		return nil
	}
	bot := s.qqOfficial.callbackSnapshot()
	if len(verified) > 0 {
		bot = verified[0]
	}
	if bot == nil || s.LockAccounts == nil {
		return ErrQQOfficialDisabled
	}
	code := qqGroupCodeFromVerifyInfo(request.VerifyInfo)
	hash := sha256.Sum256(payload)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	q := store.New(tx)
	if err = s.lockQQGroupScope(ctx, q, bot, request.GroupOpenID); errors.Is(err, ErrQQGroupNotConfigured) {
		return nil
	} else if err != nil {
		return err
	}
	if eventID != "" {
		if err = q.LockOfficialQQEvent(ctx, eventID); err != nil {
			return err
		}
		if existing, lookupErr := q.GetBotEvent(ctx, store.GetBotEventParams{Source: "qqgroup-join", EventID: eventID}); lookupErr == nil {
			if !bytesEqual(existing.PayloadHash, hash[:]) {
				return ErrQQBotReplay
			}
			return nil
		} else if !errors.Is(lookupErr, pgx.ErrNoRows) {
			return lookupErr
		}
	}
	if code == "" {
		if eventID != "" {
			if err = q.InsertBotEvent(ctx, store.InsertBotEventParams{Source: "qqgroup-join", EventID: eventID, PayloadHash: hash[:]}); err != nil {
				return err
			}
		}
		return tx.Commit(ctx)
	}
	codeHash := officialChallengeHash(code)
	owner, err := q.QQGroupApprovalOwner(ctx, codeHash)
	if errors.Is(err, pgx.ErrNoRows) {
		if eventID != "" {
			_ = q.InsertBotEvent(ctx, store.InsertBotEventParams{Source: "qqgroup-join", EventID: eventID, PayloadHash: hash[:]})
		}
		return tx.Commit(ctx)
	}
	if err != nil {
		return err
	}
	if err = q.LockOfficialQQIdentity(ctx, request.MemberOpenID); err != nil {
		return err
	}
	if err = s.LockAccounts(ctx, tx, []string{owner.String()}); err != nil {
		return err
	}
	app, err := q.FindQQGroupApplicationByCode(ctx, codeHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if app.UserID != owner || app.GroupOpenid != request.GroupOpenID {
		return nil
	}
	if err = s.checkQQGroupApplicant(ctx, q, app, request.MemberOpenID); errors.Is(err, ErrQQNotBound) || errors.Is(err, ErrQQOfficialConflict) || errors.Is(err, ErrQQGroupMemberRequired) {
		return nil
	} else if err != nil {
		return err
	}
	if err = q.MarkQQGroupApplicationApproving(ctx, store.MarkQQGroupApplicationApprovingParams{ID: app.ID, MemberOpenid: pgtype.Text{String: request.MemberOpenID, Valid: true}, JoinRequestID: pgtype.Text{String: request.JoinRequestID, Valid: true}}); err != nil {
		return err
	}
	if eventID != "" {
		if err = q.InsertBotEvent(ctx, store.InsertBotEventParams{Source: "qqgroup-join", EventID: eventID, PayloadHash: hash[:]}); err != nil {
			return err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	err = s.approveQQGroupApplicant(ctx, bot, app, request)
	if err != nil {
		failureTx, beginErr := s.pool.Begin(ctx)
		if beginErr != nil {
			return fmt.Errorf("approve group request: %w; record failure: %v", err, beginErr)
		}
		defer failureTx.Rollback(context.Background())
		failureQ := store.New(failureTx)
		_ = failureQ.MarkQQGroupApplicationFailed(ctx, store.MarkQQGroupApplicationFailedParams{ID: app.ID, FailureReason: pgtype.Text{String: err.Error(), Valid: true}})
		_ = failureTx.Commit(ctx)
		return err
	}
	approvedTx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer approvedTx.Rollback(context.Background())
	if err = store.New(approvedTx).MarkQQGroupApplicationApproved(ctx, app.ID); err != nil {
		return err
	}
	return approvedTx.Commit(ctx)
}

func bytesEqual(a, b []byte) bool {
	return hmac.Equal(a, b)
}

func (s *Service) ProcessQQGroupMemberAdded(ctx context.Context, eventID string, event OfficialQQGroupMemberEvent, payload []byte, verified ...*OfficialQQBot) error {
	if s.qqOfficial == nil || eventID == "" || event.MemberOpenID == "" {
		return nil
	}
	bot := s.qqOfficial.callbackSnapshot()
	if len(verified) > 0 {
		bot = verified[0]
	}
	if bot == nil || s.LockAccounts == nil {
		return ErrQQOfficialDisabled
	}
	hash := sha256.Sum256(payload)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	q := store.New(tx)
	if err = s.lockQQGroupScope(ctx, q, bot, event.GroupOpenID); errors.Is(err, ErrQQGroupNotConfigured) {
		return nil
	} else if err != nil {
		return err
	}
	if err = q.LockOfficialQQEvent(ctx, eventID); err != nil {
		return err
	}
	if existing, lookupErr := q.GetBotEvent(ctx, store.GetBotEventParams{Source: "qqgroup-member", EventID: eventID}); lookupErr == nil {
		if !bytesEqual(existing.PayloadHash, hash[:]) {
			return ErrQQBotReplay
		}
		return nil
	} else if !errors.Is(lookupErr, pgx.ErrNoRows) {
		return lookupErr
	}
	if err = q.LockOfficialQQIdentity(ctx, event.MemberOpenID); err != nil {
		return err
	}
	id, owner, err := q.QQGroupMemberApplication(ctx, event.GroupOpenID, event.MemberOpenID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if err = s.LockAccounts(ctx, tx, []string{owner.String()}); err != nil {
		return err
	}
	app, err := q.LockQQGroupApplication(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if app.UserID != owner || app.GroupOpenid != event.GroupOpenID || app.MemberOpenid.String != event.MemberOpenID ||
		(app.Status != "approving" && app.Status != "approved") {
		return nil
	}
	if err = s.checkQQGroupApplicant(ctx, q, app, event.MemberOpenID); errors.Is(err, ErrQQNotBound) || errors.Is(err, ErrQQOfficialConflict) || errors.Is(err, ErrQQGroupMemberRequired) {
		return nil
	} else if err != nil {
		return err
	}
	if err = q.UpsertQQBinding(ctx, store.UpsertQQBindingParams{UserID: app.UserID, Openid: event.MemberOpenID, QqVersion: app.QqVersion}); err != nil {
		return err
	}
	if err = q.InvalidateQQChallenges(ctx, app.UserID); err != nil {
		return err
	}
	if err = q.InsertQQGroupBinding(ctx, store.InsertQQGroupBindingParams{UserID: app.UserID, GroupOpenid: app.GroupOpenid, MemberOpenid: event.MemberOpenID, QqNumber: app.QqNumber, ApplicationID: app.ID}); err != nil {
		return err
	}
	if err = q.InvalidateQQConfirmations(ctx, app.UserID); err != nil {
		return err
	}
	if err = q.InsertConfirmation(ctx, store.InsertConfirmationParams{UserID: app.UserID, FieldVersion: app.QqVersion, Source: "qqgroup", Actor: event.MemberOpenID, EventID: eventID}); err != nil {
		return err
	}
	if err = q.CompleteQQGroupAdmission(ctx, app.ID); err != nil {
		return err
	}
	if err = q.InsertBotEvent(ctx, store.InsertBotEventParams{Source: "qqgroup-member", EventID: eventID, PayloadHash: hash[:]}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) SyncQQGroupApplications(ctx context.Context) error {
	if s.qqOfficial == nil {
		return ErrQQOfficialDisabled
	}
	for _, group := range s.qqOfficial.ConfiguredGroups() {
		cursor := ""
		for {
			page, err := s.qqOfficial.GroupJoinRequests(ctx, group, cursor, 100)
			if err != nil {
				return err
			}
			for _, item := range page.List {
				// QQ scopes these entries by the GET path and omits the group from the list item.
				if item.GroupOpenID != "" && item.GroupOpenID != group {
					continue
				}
				item.GroupOpenID = group
				eventID := "official-group-sync:" + group + ":" + item.JoinRequestID
				if err = s.ProcessQQGroupJoinRequest(ctx, eventID, item, []byte(eventID)); err != nil {
					return err
				}
			}
			if page.NextCursor == "" || page.NextCursor == cursor {
				break
			}
			cursor = page.NextCursor
		}
	}
	return nil
}

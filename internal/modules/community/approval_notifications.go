package community

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"glorynavy.local/seat/internal/modules/community/internal/store"
)

type approvalNoticeRecipient struct {
	store.ApprovalRecipient
	Keys  []string
	Count int
}

func (r *ApprovalJobs) deliver(ctx context.Context, group string) error {
	if !r.Enabled || r.Service == nil || r.Pending == nil || r.Service.pool == nil {
		return nil
	}
	origin, err := url.Parse(r.Origin)
	if err != nil || origin.Host == "" || (origin.Scheme != "https" && origin.Scheme != "http") {
		return errors.New("approval notification origin missing")
	}
	s := r.Service
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	locked, err := store.ApprovalGroupLock(ctx, conn, group)
	if err != nil || !locked {
		return err
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := store.ApprovalGroupUnlock(unlockCtx, conn, group); err != nil {
			_ = conn.Conn().Close(unlockCtx)
		}
	}()
	limited, err := store.ApprovalRateLimited(ctx, conn, group)
	if err != nil || limited {
		return err
	}
	settings, err := s.ListOneBotSettings(ctx)
	if err != nil {
		return err
	}
	if !settings.Enabled || !slices.ContainsFunc(settings.Groups, func(g OneBotGroup) bool { return g.ID == group && g.NotifyApprovals }) {
		return nil
	}
	recipients, err := store.ApprovalRecipients(ctx, conn)
	if err != nil || len(recipients) == 0 {
		return err
	}
	row, client, err := s.oneBotConnection(ctx)
	if err != nil {
		return err
	}
	if strconv.FormatInt(row.Revision, 10) != settings.Revision {
		return ErrVersion
	}
	if _, err = client.login(ctx, row.BotQq); err != nil {
		return err
	}
	groupID, err := strconv.ParseInt(group, 10, 64)
	if err != nil {
		return ErrQQGroupNotConfigured
	}
	var members []struct {
		UserID json.Number `json:"user_id"`
	}
	// NapCat 4.18.37 refreshes the full list asynchronously and can replace a
	// populated cache with an empty result. Use it only for candidate discovery;
	// a forced individual lookup below is the final membership check.
	if err = client.call(ctx, "get_group_member_list", map[string]any{"group_id": groupID, "no_cache": false}, &members); err != nil {
		return err
	}
	if len(members) == 0 {
		return errors.New("approval notification group members unavailable")
	}
	memberSet := map[string]bool{}
	for _, m := range members {
		memberSet[m.UserID.String()] = true
	}
	selected := []approvalNoticeRecipient{}
	var sourceErr error
	for _, recipient := range recipients {
		if !memberSet[recipient.QQ] || recipient.QQ == row.BotQq {
			continue
		}
		keys, e := r.Pending(ctx, recipient.UserID)
		if e != nil {
			sourceErr = errors.New("approval notification source unavailable")
			continue
		}
		tx, e := conn.Begin(ctx)
		if e != nil {
			return e
		}
		due, e := store.ReconcileApprovalNotices(ctx, tx, group, recipient.UserID, keys)
		if e == nil {
			e = tx.Commit(ctx)
		} else {
			_ = tx.Rollback(context.Background())
		}
		if e != nil {
			return e
		}
		if len(due) > 0 {
			selected = append(selected, approvalNoticeRecipient{ApprovalRecipient: recipient, Keys: due, Count: len(keys)})
		}
		// Remaining recipients stay unclaimed and are picked up by the next scan.
		if len(selected) == 20 {
			break
		}
	}
	// Re-read live permissions and business state after the network calls and
	// recipient scan. Never send counts from the materialized approval index.
	ready := selected[:0]
	for _, recipient := range selected {
		var member struct {
			UserID  json.Number `json:"user_id"`
			GroupID json.Number `json:"group_id"`
		}
		if e := client.call(ctx, "get_group_member_info", map[string]any{"group_id": groupID, "user_id": recipient.QQ, "no_cache": true}, &member); e != nil || member.UserID.String() != recipient.QQ || member.GroupID.String() != group {
			sourceErr = errors.New("approval notification member verification unavailable")
			continue
		}
		keys, e := r.Pending(ctx, recipient.UserID)
		if e != nil {
			sourceErr = errors.New("approval notification source unavailable")
			continue
		}
		tx, e := conn.Begin(ctx)
		if e != nil {
			return e
		}
		due, e := store.ReconcileApprovalNotices(ctx, tx, group, recipient.UserID, keys)
		if e == nil {
			e = tx.Commit(ctx)
		} else {
			_ = tx.Rollback(context.Background())
		}
		if e != nil {
			return e
		}
		if len(due) > 0 {
			recipient.Keys = due
			recipient.Count = len(keys)
			ready = append(ready, recipient)
		}
	}
	if len(ready) == 0 {
		return sourceErr
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	q := store.New(tx)
	current, err := q.LockOneBotSettings(ctx)
	if err != nil || !current.Enabled || current.Revision != row.Revision {
		return ErrOneBotUnavailable
	}
	accounts := make([]string, 0, len(ready))
	for _, recipient := range ready {
		accounts = append(accounts, recipient.UserID)
	}
	if s.LockAccounts == nil {
		return errors.New("approval notification account guard missing")
	}
	if err = s.LockAccounts(ctx, tx, accounts); err != nil {
		return err
	}
	count := 0
	for _, recipient := range ready {
		var id pgtype.UUID
		if err = id.Scan(recipient.UserID); err != nil {
			return err
		}
		binding, e := q.GetOneBotBinding(ctx, id)
		if e != nil || binding.QqNumber != recipient.QQ || binding.QqVersion != recipient.Version {
			return errors.New("approval notification binding changed")
		}
		count += len(recipient.Keys)
	}
	batch, err := store.BeginApprovalDelivery(ctx, tx, group, len(ready), count)
	if err != nil {
		return err
	}
	for _, recipient := range ready {
		if err = store.ClaimApprovalNotices(ctx, tx, group, recipient.UserID, batch, recipient.Keys); err != nil {
			return err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	// Durable claim precedes the external side effect. An ambiguous response or
	// a crash here must never cause an automatic duplicate @ on retry.
	segments := []oneBotSegment{}
	for _, recipient := range ready {
		segments = append(segments, oneBotSegment{Type: "at", Data: map[string]string{"qq": recipient.QQ}}, oneBotSegment{Type: "text", Data: map[string]string{"text": fmt.Sprintf(" 有 %d 项待审批\n", recipient.Count)}})
	}
	segments = append(segments, oneBotSegment{Type: "text", Data: map[string]string{"text": strings.TrimRight(r.Origin, "/") + "/approvals"}})
	var result struct {
		MessageID json.Number `json:"message_id"`
	}
	sendErr := client.call(ctx, "send_group_msg", map[string]any{"group_id": groupID, "message": segments}, &result)
	message := ""
	if sendErr == nil && result.MessageID.String() != "" && result.MessageID.String() != "0" {
		message = result.MessageID.String()
	}
	finishCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = store.FinishApprovalDelivery(finishCtx, conn, batch, message); err != nil {
		return err
	}
	if message == "" {
		slog.Warn("approval notification delivery unknown; automatic retry suppressed", "batch", batch, "recipients", len(ready))
	} else {
		slog.Info("approval notification sent", "batch", batch, "recipients", len(ready), "notices", count)
	}
	return sourceErr
}

package community

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"time"

	"glorynavy.local/seat/internal/modules/community/internal/store"
)

const qqGroupRecheckInterval = time.Minute

// RunQQGroupAdmission also reconciles requests when the gateway is disconnected.
func (s *Service) RunQQGroupAdmission(ctx context.Context) {
	ticker := time.NewTicker(qqGroupRecheckInterval)
	defer ticker.Stop()
	for ctx.Err() == nil {
		check, cancel := context.WithTimeout(ctx, 45*time.Second)
		err := s.syncPendingQQGroupApplications(check)
		cancel()
		if err != nil && ctx.Err() == nil {
			slog.Warn("QQ group admission reconciliation incomplete")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) syncPendingQQGroupApplications(ctx context.Context) error {
	if !s.qqOfficial.Configured() || len(s.qqOfficial.ConfiguredGroups()) == 0 {
		return nil
	}
	groups, err := store.New(s.pool).QQGroupsAwaitingApplications(ctx)
	if err != nil {
		return err
	}
	return s.syncQQGroups(ctx, groups)
}

func (s *Service) SyncQQGroupApplications(ctx context.Context) error {
	if !s.qqOfficial.Configured() {
		return ErrQQOfficialDisabled
	}
	return s.syncQQGroups(ctx, s.qqOfficial.ConfiguredGroups())
}

func (s *Service) syncQQGroups(ctx context.Context, groups []string) error {
	if !slices.ContainsFunc(groups, s.qqOfficial.GroupAllowed) {
		return nil
	}
	// Refresh the shared token before pinning the configuration for this scan.
	if _, err := s.qqOfficial.accessToken(ctx); err != nil {
		return err
	}
	bot := s.qqOfficial.callbackSnapshot()
	bot.SetGroups(s.qqOfficial.ConfiguredGroups())
	var syncErr error
	for _, group := range groups {
		if bot.GroupAllowed(group) {
			syncErr = errors.Join(syncErr, s.syncQQGroup(ctx, bot, group))
		}
	}
	return syncErr
}

func (s *Service) syncQQGroup(ctx context.Context, bot *OfficialQQBot, group string) error {
	// Newly received events may not appear in QQ's list immediately.
	cutoff := time.Now().Add(-2 * qqGroupRecheckInterval)
	observed := []string{}
	cursors := map[string]bool{"": true}
	complete := true
	var syncErr error
	cursor := ""
	for {
		page, err := bot.GroupJoinRequests(ctx, group, cursor, 100)
		if err != nil {
			return errors.Join(syncErr, err)
		}
		if !page.listReported {
			complete = false
		}
		for _, item := range page.List {
			// QQ scopes these entries by the GET path and can omit the group.
			if item.JoinRequestID == "" || (item.GroupOpenID != "" && item.GroupOpenID != group) {
				complete = false
				continue
			}
			observed = append(observed, item.JoinRequestID)
			item.GroupOpenID = group
			// The application row guards repeat approvals. Poll results may be
			// revisited when the applicant's EVE membership changes.
			syncErr = errors.Join(syncErr, s.ProcessQQGroupJoinRequest(ctx, "", item, nil, bot))
		}
		if page.NextCursor == "" {
			break
		}
		if cursors[page.NextCursor] {
			return errors.Join(syncErr, errors.New("QQ group request list repeated a page"))
		}
		cursor = page.NextCursor
		cursors[cursor] = true
	}
	if !complete || syncErr != nil {
		return syncErr
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	q := store.New(tx)
	if err = s.lockQQGroupScope(ctx, q, bot, group); err != nil {
		return err
	}
	if err = q.ExpireMissingQQGroupRequests(ctx, group, cutoff, observed); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

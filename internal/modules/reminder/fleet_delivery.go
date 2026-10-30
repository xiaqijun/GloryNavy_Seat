package reminder

import (
	"context"
	"errors"
	"strconv"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/reminder/internal/store"
)

func (s *Service) FleetTarget(ctx context.Context, channel string) (string, error) {
	if s == nil || !s.Enabled || s.Pool == nil {
		return "", ErrSender
	}
	if channel == "email" {
		c, err := store.ReadEmailSettings(ctx, s.Pool, false)
		if err != nil || !c.Enabled {
			return "", ErrEmailConfig
		}
		return strconv.FormatInt(c.Revision, 10), nil
	}
	if channel == "eve_mail" {
		c, err := store.ReadSettings(ctx, s.Pool, false)
		if err != nil || !c.Enabled {
			return "", ErrSender
		}
		if _, err = s.sender(ctx, c); err != nil {
			return "", err
		}
		return strconv.FormatInt(c.Revision, 10), nil
	}
	return "", ErrEvents
}

// FleetEmail never exposes an address or credentials to the originating module.
// The caller records delivery before invoking it and must not retry unknown results.
func (s *Service) FleetEmail(ctx context.Context, account, expected, subject, body string) Result {
	fail := func(reason string) Result { return Result{State: "failed", Reason: reason} }
	if s == nil || !s.Enabled || s.Pool == nil {
		return fail("service_unavailable")
	}
	c, err := store.ReadEmailSettings(ctx, s.Pool, false)
	if err != nil || !c.Enabled || strconv.FormatInt(c.Revision, 10) != expected {
		return fail("context_changed")
	}
	a, err := store.ReadEmailAccount(ctx, s.Pool, account, false)
	if errors.Is(err, pgx.ErrNoRows) {
		return Result{State: "skipped", Reason: "email_unavailable"}
	}
	if err != nil {
		return fail("service_unavailable")
	}
	if !a.Verified || !a.Enabled || a.Email == "" {
		return Result{State: "skipped", Reason: "email_unavailable"}
	}
	secret, err := s.openEmailSecret(c.Secret)
	if err != nil {
		return fail("service_unavailable")
	}
	if s.LockAccounts == nil {
		return fail("service_unavailable")
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return fail("service_unavailable")
	}
	defer tx.Rollback(context.Background())
	if err = s.LockAccounts(ctx, tx, []string{account}); err != nil {
		return Result{State: "skipped", Reason: "identity_changed"}
	}
	current, err := store.ReadEmailSettings(ctx, tx, true)
	if err != nil || !current.Enabled || current.Revision != c.Revision {
		return fail("context_changed")
	}
	member, err := store.ReadEmailAccount(ctx, tx, account, true)
	if err != nil || !member.Verified || !member.Enabled || member.Revision != a.Revision || member.Email != a.Email {
		return Result{State: "skipped", Reason: "email_unavailable"}
	}
	if err = tx.Commit(ctx); err != nil {
		return fail("service_unavailable")
	}
	current, err = store.ReadEmailSettings(ctx, s.Pool, false)
	if err != nil || !current.Enabled || current.Revision != c.Revision {
		return fail("context_changed")
	}
	member, err = store.ReadEmailAccount(ctx, s.Pool, account, false)
	if err != nil || !member.Verified || !member.Enabled || member.Revision != a.Revision || member.Email != a.Email {
		return Result{State: "skipped", Reason: "email_unavailable"}
	}
	return s.sendEmail(ctx, smtpConfig{Host: c.Host, Port: c.Port, ImplicitTLS: c.ImplicitTLS, Username: c.Username, Password: secret, From: c.Sender}, a.Email, subject, body)
}

func (s *Service) FleetCorporationMail(ctx context.Context, corporation int64, expected, subject, body string) Result {
	fail := func(reason string) Result { return Result{State: "failed", Reason: reason} }
	if s == nil || !s.Enabled || s.Pool == nil || s.SendCorporation == nil || corporation <= 0 {
		return fail("service_unavailable")
	}
	c, err := store.ReadSettings(ctx, s.Pool, false)
	if err != nil || !c.Enabled || strconv.FormatInt(c.Revision, 10) != expected {
		return fail("context_changed")
	}
	sender, err := s.sender(ctx, c)
	if err != nil {
		return fail("sender_unavailable")
	}
	current, err := store.ReadSettings(ctx, s.Pool, false)
	if err != nil || !current.Enabled || current.Revision != c.Revision {
		return fail("context_changed")
	}
	return s.SendCorporation(ctx, sender, corporation, subject, body)
}

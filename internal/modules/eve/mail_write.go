package eve

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
	"net/url"
	"strings"
	"unicode/utf8"
)

const MailSendScope = "esi-mail.send_mail.v1"

type mailAuthorizationKey struct{}

// The extra scope is server-selected, never supplied by the browser.
func mailAuthorizationURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	q := u.Query()
	scopes := strings.Fields(q.Get("scope"))
	if !hasScopes(scopes, []string{MailSendScope}) {
		scopes = append(scopes, MailSendScope)
	}
	q.Set("scope", strings.Join(scopes, " "))
	u.RawQuery = q.Encode()
	return u.String(), nil
}

type MailWriteResult struct {
	ID            int64
	State, Reason string
}

// MailGrant exposes no token; generation pins a later send to this authorization.
func (s *AuthorizationService) MailGrant(ctx context.Context, id int64, owner []byte) (int64, bool, error) {
	if s == nil || s.pool == nil {
		return 0, false, nil
	}
	c, err := store.New(s.pool).GetCredential(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return c.GrantGeneration, c.State != "reauthorize" && subtle.ConstantTimeCompare(owner, c.OwnerHash) == 1 && hasScopes(c.Scopes, []string{MailSendScope}), nil
}

// Sends one character mail, never authorizes a CSPA charge and never retries an uncertain POST.
func (s *AuthorizationService) SendMail(ctx context.Context, sender int64, owner []byte, generation, recipient int64, subject, body string) MailWriteResult {
	fail := func(reason string) MailWriteResult { return MailWriteResult{State: "failed", Reason: reason} }
	if s == nil || s.esi == nil {
		return fail("service_unavailable")
	}
	if sender <= 0 || recipient <= 0 || subject == "" || body == "" || utf8.RuneCountInString(subject) > 1000 || utf8.RuneCountInString(body) > 10000 {
		return fail("invalid_mail")
	}
	c, err := store.New(s.pool).GetCredential(ctx, sender)
	if err != nil {
		return fail("credential_unavailable")
	}
	if subtle.ConstantTimeCompare(owner, c.OwnerHash) != 1 || c.GrantGeneration != generation {
		return fail("identity_changed")
	}
	if c.State == "reauthorize" {
		return fail("reauthorize")
	}
	if !hasScopes(c.Scopes, []string{MailSendScope}) {
		return fail("missing_scope")
	}
	payload, _ := json.Marshal(map[string]any{"approved_cost": 0, "subject": subject, "body": body, "recipients": []map[string]any{{"recipient_id": recipient, "recipient_type": "character"}}})
	var id int64
	_, err = s.esi.Request(ctx, ESIRequest{Method: "POST", Path: fmt.Sprintf("/characters/%d/mail/", sender), Body: payload, CharacterID: sender, Generation: generation, Scopes: []string{MailSendScope}, Mutation: true}, &id)
	if err == nil && id > 0 {
		return MailWriteResult{ID: id, State: "sent"}
	}
	if errors.Is(err, ErrReauthorize) {
		return fail("reauthorize")
	}
	var limited retryError
	if errors.As(err, &limited) {
		return fail("rate_limited")
	}
	var fault syncFault
	if errors.As(err, &fault) {
		if fault.Status >= 400 && fault.Status < 500 {
			return fail("mail_rejected")
		}
		if fault.Reason == "missing_scope" || fault.Reason == "identity_changed" || fault.Reason == "credential_context_missing" {
			return fail(fault.Reason)
		}
	}
	return MailWriteResult{State: "unknown", Reason: "confirmation_required"}
}

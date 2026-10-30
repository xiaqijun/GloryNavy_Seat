package eve

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"

	"glorynavy.local/seat/internal/modules/eve/internal/store"
)

// SendCorporationMail sends to the EVE corporation address, not a list of
// characters. The authorized sender and zero approved cost are rechecked.
func (s *AuthorizationService) SendCorporationMail(ctx context.Context, sender int64, owner []byte, generation, corporation int64, subject, body string) MailWriteResult {
	fail := func(reason string) MailWriteResult { return MailWriteResult{State: "failed", Reason: reason} }
	if s == nil || s.esi == nil || sender <= 0 || corporation <= 0 || subject == "" || body == "" || utf8.RuneCountInString(subject) > 1000 || utf8.RuneCountInString(body) > 10000 {
		return fail("invalid_mail")
	}
	c, err := store.New(s.pool).GetCredential(ctx, sender)
	if err != nil {
		return fail("credential_unavailable")
	}
	if len(owner) == 0 || subtle.ConstantTimeCompare(owner, c.OwnerHash) != 1 || c.GrantGeneration != generation {
		return fail("identity_changed")
	}
	if c.State == "reauthorize" {
		return fail("reauthorize")
	}
	if !hasScopes(c.Scopes, []string{MailSendScope}) {
		return fail("missing_scope")
	}
	payload, _ := json.Marshal(map[string]any{"approved_cost": 0, "subject": subject, "body": body, "recipients": []map[string]any{{"recipient_id": corporation, "recipient_type": "corporation"}}})
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
	if errors.As(err, &fault) && ((fault.Status >= 400 && fault.Status < 500) || fault.Reason == "missing_scope" || fault.Reason == "identity_changed" || fault.Reason == "credential_context_missing") {
		return fail("mail_rejected")
	}
	return MailWriteResult{State: "unknown", Reason: "confirmation_required"}
}

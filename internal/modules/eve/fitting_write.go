package eve

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
)

const FittingsWriteScope = "esi-fittings.write_fittings.v1"

type GameFitting struct {
	Name        string             `json:"name"`
	Description string             `json:"description"`
	ShipTypeID  int64              `json:"ship_type_id"`
	Items       []SavedFittingItem `json:"items"`
}
type FittingWriteResult struct {
	ID            int64
	State, Reason string
}

// A network failure may have occurred after ESI created the fitting; the caller must never retry it blindly.
func (s *AuthorizationService) SaveGameFitting(ctx context.Context, id int64, owner []byte, fit GameFitting) FittingWriteResult {
	fail := func(reason string) FittingWriteResult { return FittingWriteResult{State: "failed", Reason: reason} }
	if s == nil || s.esi == nil {
		return fail("service_unavailable")
	}
	c, err := store.New(s.pool).GetCredential(ctx, id)
	if err != nil {
		return fail("credential_unavailable")
	}
	if subtle.ConstantTimeCompare(owner, c.OwnerHash) != 1 {
		return fail("identity_changed")
	}
	if c.State == "reauthorize" {
		return fail("reauthorize")
	}
	if !hasScopes(c.Scopes, []string{FittingsWriteScope}) {
		return fail("missing_scope")
	}
	body, err := json.Marshal(fit)
	if err != nil {
		return fail("invalid_fitting")
	}
	var saved struct {
		ID int64 `json:"fitting_id"`
	}
	_, err = s.esi.Request(ctx, ESIRequest{Method: "POST", Path: fmt.Sprintf("/characters/%d/fittings/", id), Body: body, CharacterID: id, Generation: c.GrantGeneration, Scopes: []string{FittingsWriteScope}, Mutation: true}, &saved)
	if err == nil && saved.ID > 0 {
		return FittingWriteResult{ID: saved.ID, State: "saved"}
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
			return fail(fault.Reason)
		}
		if fault.Reason == "missing_scope" || fault.Reason == "identity_changed" || fault.Reason == "credential_context_missing" {
			return fail(fault.Reason)
		}
	}
	return FittingWriteResult{State: "unknown", Reason: "confirmation_required"}
}

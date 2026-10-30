package eve

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
)

const FleetWriteScope = "esi-fleets.write_fleet.v1"

type fleetAuthorizationKey struct{}

func fleetAuthorizationURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	q := u.Query()
	scopes := strings.Fields(q.Get("scope"))
	if !hasScopes(scopes, []string{FleetWriteScope}) {
		scopes = append(scopes, FleetWriteScope)
	}
	q.Set("scope", strings.Join(scopes, " "))
	u.RawQuery = q.Encode()
	return u.String(), nil
}

type FleetMOTDResult struct{ State, Reason string }

// FleetWriteGrant returns no credential and pins subsequent reads and writes
// to the observed grant generation and identity owner.
func (s *AuthorizationService) FleetWriteGrant(ctx context.Context, id int64, owner []byte) (int64, bool, error) {
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
	return c.GrantGeneration, len(owner) > 0 && c.State != "reauthorize" && subtle.ConstantTimeCompare(owner, c.OwnerHash) == 1 && hasScopes(c.Scopes, []string{FleetReadScope, FleetWriteScope}), nil
}

// ReadFleetMOTD is an observation, not permission to overwrite a later change.
func (s *AuthorizationService) ReadFleetMOTD(ctx context.Context, characterID int64, owner []byte) (FleetSnapshot, string, error) {
	var empty FleetSnapshot
	generation, ok, err := s.FleetWriteGrant(ctx, characterID, owner)
	if err != nil {
		return empty, "", err
	}
	if !ok {
		return empty, "", ErrReauthorize
	}
	fleet, err := s.Fleet(ctx, characterID, owner)
	if err != nil {
		return empty, "", err
	}
	if fleet.Generation != generation {
		return empty, "", ErrReauthorize
	}
	var data struct {
		MOTD string `json:"motd"`
	}
	_, err = s.esi.Request(ctx, ESIRequest{Method: "GET", Path: fmt.Sprintf("/fleets/%d/", fleet.FleetID), CharacterID: characterID, Generation: generation, Scopes: []string{FleetReadScope}, ForceRefresh: true}, &data)
	return fleet, data.MOTD, err
}

// UpdateFleetMOTD refuses a changed fleet or MOTD; an uncertain mutation is
// never retried automatically. ESI has no atomic compare-and-swap for MOTD.
func (s *AuthorizationService) UpdateFleetMOTD(ctx context.Context, characterID int64, owner []byte, generation, fleetID int64, expected, motd string) FleetMOTDResult {
	fail := func(reason string) FleetMOTDResult { return FleetMOTDResult{State: "failed", Reason: reason} }
	if fleetID <= 0 || motd == "" || utf8.RuneCountInString(motd) > 10000 {
		return fail("invalid_motd")
	}
	fleet, current, err := s.ReadFleetMOTD(ctx, characterID, owner)
	if err != nil {
		return fail("fleet_unavailable")
	}
	if fleet.Generation != generation || fleet.FleetID != fleetID || current != expected {
		return fail("fleet_changed")
	}
	payload, _ := json.Marshal(map[string]string{"motd": motd})
	_, err = s.esi.Request(ctx, ESIRequest{Method: "PUT", Path: fmt.Sprintf("/fleets/%d/", fleetID), Body: payload, CharacterID: characterID, Generation: generation, Scopes: []string{FleetWriteScope}, Mutation: true}, nil)
	if err == nil {
		return FleetMOTDResult{State: "sent"}
	}
	if errors.Is(err, ErrReauthorize) {
		return fail("reauthorize")
	}
	var limited retryError
	if errors.As(err, &limited) {
		return fail("rate_limited")
	}
	var fault syncFault
	if errors.As(err, &fault) && fault.Status >= 400 && fault.Status < 500 {
		return fail("fleet_rejected")
	}
	return FleetMOTDResult{State: "unknown", Reason: "confirmation_required"}
}

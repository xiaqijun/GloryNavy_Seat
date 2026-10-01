package eve

import (
	"context"
	"crypto/subtle"
	"errors"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
	"slices"
	"time"
)

// The credential row lock covers only bounded SSO refresh, never ESI collection.
// rejected is the access token rejected by ESI; a newer stored token is reused.
func (s *AuthorizationService) currentToken(parent context.Context, id, generation int64, rejected string, required ...string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(context.Background())
	q := store.New(tx)
	c, err := q.GetCredentialForUpdate(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", syncFault{Reason: "identity_changed", Status: 0, Temporary: false}
	}
	if err != nil {
		return "", err
	}
	if c.GrantGeneration != generation {
		return "", syncFault{Reason: "identity_changed", Status: 0, Temporary: false}
	}
	if c.State == "reauthorize" {
		return "", ErrReauthorize
	}
	a, err := s.open(id, c.OwnerHash, c.Sealed)
	if err != nil {
		return "", err
	}
	if len(required) == 0 {
		required = []string{CorporationRolesScope}
	}
	for _, scope := range required {
		if !slices.Contains(a.Scopes, scope) {
			return "", syncFault{Reason: "missing_scope", Status: 0, Temporary: false}
		}
	}
	var attempted time.Time
	committed := false
	reason := "storage_error"
	defer func() {
		if !attempted.IsZero() && !committed {
			// A timeout may already have aborted the refresh transaction. Record
			// the failure separately, fenced to the grant that actually attempted it.
			_ = tx.Rollback(context.Background())
			s.observeRefreshFailure(parent, id, generation, attempted, reason)
		}
	}()
	reused := int64(1)
	if a.ExpiresAt.Before(time.Now().Add(time.Minute)) || (rejected != "" && rejected == a.AccessToken) {
		attempted = time.Now()
		reused = 0
		ch, err := s.client.refresh(ctx, a.RefreshToken)
		if err != nil {
			reason = refreshFailureReason(err)
			if ctx.Err() != nil {
				reason = "sso_timeout"
			}
			return "", err
		}
		if ch.ID != id || subtle.ConstantTimeCompare(httpapi.Hash(ch.Owner), c.OwnerHash) != 1 {
			reason = "identity_mismatch"
			return "", ErrReauthorize
		}
		if !hasScopes(ch.authorization.Scopes, a.Scopes) {
			reason = "scope_lost"
			return "", ErrReauthorize
		}
		a = ch.authorization
		sealed, err := s.seal(id, c.OwnerHash, a)
		if err != nil {
			return "", err
		}
		if err = q.PersistRotation(ctx, store.PersistRotationParams{CharacterID: id, Sealed: sealed}); err != nil {
			return "", err
		}
		if err = recordRefresh(ctx, q, id, generation, attempted, ""); err != nil {
			return "", err
		}
	}
	if err = q.ObserveTokenUse(ctx, store.ObserveTokenUseParams{CharacterID: id, Generation: generation, ExpiresAt: timestamp(a.ExpiresAt), Reused: reused}); err != nil {
		return "", err
	}
	if err = tx.Commit(ctx); err != nil {
		reason = "rotation_commit_uncertain"
		return "", err
	}
	committed = true
	return a.AccessToken, nil
}

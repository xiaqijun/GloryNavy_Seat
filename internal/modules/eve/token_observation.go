package eve

import (
	"context"
	"errors"
	"glorynavy.local/seat/internal/modules/eve/internal/esiclient"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
	"time"
)

func refreshFailureReason(err error) string {
	var retry retryError
	switch {
	case errors.Is(err, ErrReauthorize):
		return "reauthorize"
	case errors.As(err, &retry):
		return "sso_rate_limited"
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return "sso_timeout"
	default:
		return "sso_unavailable"
	}
}

func recordRefresh(ctx context.Context, q *store.Queries, id, generation int64, started time.Time, reason string) error {
	success := reason == ""
	if err := q.ObserveTokenRefresh(ctx, store.ObserveTokenRefreshParams{CharacterID: id, Generation: generation, AttemptedAt: timestamp(started), Success: success, Reason: reason}); err != nil {
		return err
	}
	outcome := "refresh_failed"
	if success {
		outcome = "refresh_success"
	}
	return q.InsertTokenEvent(ctx, store.InsertTokenEventParams{CharacterID: id, Generation: generation, Outcome: outcome, Reason: reason, DurationMs: max(0, time.Since(started).Milliseconds())})
}

func (s *AuthorizationService) observeRefreshFailure(parent context.Context, id, generation int64, started time.Time, reason string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 2*time.Second)
	defer cancel()
	err := func() error {
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(context.Background())
		q := store.New(tx)
		c, err := q.GetCredentialForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if c.GrantGeneration != generation {
			return nil
		}
		if err = recordRefresh(ctx, q, id, generation, started, reason); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}()
	if s.logger != nil {
		s.logger.Warn("EVE token refresh failed", "character_id", id, "generation", generation, "reason", reason, "observation_saved", err == nil)
	}
}

func (s *AuthorizationService) observeESI(parent context.Context, credential esiclient.Credential, e esiclient.RequestEvent) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 2*time.Second)
	defer cancel()
	count := func(b bool) int64 {
		if b {
			return 1
		}
		return 0
	}
	err := store.New(s.pool).ObserveTokenRequest(ctx, store.ObserveTokenRequestParams{
		CharacterID: credential.ID, Generation: credential.Generation, HttpStatus: int32(e.Status), Reason: e.Reason,
		Network: count(e.Network), CacheHit: count(e.CacheHit), Limited: count(e.Limited), Failed: count(e.Failed),
	})
	if err != nil && s.logger != nil {
		s.logger.Warn("ESI observation could not be saved", "character_id", credential.ID, "generation", credential.Generation)
	}
}

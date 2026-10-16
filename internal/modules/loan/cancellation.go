package loan

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/loan/internal/store"
)

// CancelApplication withdraws an unreviewed application. It never reverses a
// funding entry or releases the immutable claim on a delivered game contract.
func (s *Service) CancelApplication(ctx context.Context, actor string, id, version int64) (store.Case, error) {
	if id <= 0 || version <= 0 {
		return store.Case{}, ErrInvalid
	}
	c, err := store.GetCase(ctx, s.db(), id)
	if err != nil {
		return store.Case{}, err
	}
	if c.BorrowerAccountID != actor {
		return store.Case{}, ErrForbidden
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return store.Case{}, err
	}
	defer tx.Rollback(ctx)
	if err = s.guardAccounts(ctx, tx, actor); err != nil {
		return store.Case{}, err
	}
	c, err = store.LockCase(ctx, tx, id)
	if err != nil {
		return store.Case{}, err
	}
	if c.BorrowerAccountID != actor {
		return store.Case{}, ErrForbidden
	}
	if c.State != "submitted" || c.Version != version {
		return store.Case{}, ErrConflict
	}
	out, err := store.CancelApplication(ctx, tx, id, version, actor)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.Case{}, ErrConflict
	}
	if err != nil {
		return store.Case{}, err
	}
	if err = store.ReleaseApplicationSecurity(ctx, tx, id); err != nil {
		return store.Case{}, err
	}
	if err = store.Audit(ctx, tx, id, actor, "application_cancelled", c, out); err != nil {
		return store.Case{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return store.Case{}, err
	}
	return out, nil
}

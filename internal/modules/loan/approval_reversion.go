package loan

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/loan/internal/store"
)

type RevertReviewInput struct {
	Version               int64  `json:"version"`
	Note                  string `json:"note"`
	ConfirmedNotDisbursed bool   `json:"confirmed_not_disbursed"`
}

func (s *Service) RevertReview(ctx context.Context, actor string, id int64, in RevertReviewInput) (store.Case, error) {
	in.Note = strings.TrimSpace(in.Note)
	if id <= 0 || in.Version <= 0 || len([]rune(in.Note)) == 0 || len([]rune(in.Note)) > 1000 {
		return store.Case{}, ErrInvalid
	}
	previous, err := store.GetCase(ctx, s.db(), id)
	if err != nil {
		return store.Case{}, err
	}
	pool, err := store.GetPool(ctx, s.db(), previous.PoolID)
	if err != nil {
		return store.Case{}, err
	}
	allowed, err := s.canManage(ctx, actor, pool)
	if err != nil {
		return store.Case{}, err
	}
	if !allowed || actor == previous.BorrowerAccountID {
		return store.Case{}, ErrForbidden
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return store.Case{}, err
	}
	defer tx.Rollback(ctx)
	if err = s.guardAccounts(ctx, tx, actor, previous.BorrowerAccountID); err != nil {
		return store.Case{}, err
	}
	if err = store.LockCreditAccounts(ctx, tx, previous.BorrowerAccountID); err != nil {
		return store.Case{}, err
	}
	if pool.IsShared {
		lockedPool, e := store.SharedPool(ctx, tx, true)
		if e != nil {
			return store.Case{}, e
		}
		if lockedPool.ID != pool.ID {
			return store.Case{}, ErrConflict
		}
	}
	current, err := store.LockCase(ctx, tx, id)
	if err != nil {
		return store.Case{}, err
	}
	if current.Version != in.Version || current.PoolID != previous.PoolID || current.BorrowerAccountID != previous.BorrowerAccountID || (current.State != "approved" && current.State != "rejected") {
		return store.Case{}, ErrConflict
	}
	allowed, err = s.canManage(ctx, actor, pool)
	if err != nil {
		return store.Case{}, err
	}
	if !allowed || actor == current.BorrowerAccountID {
		return store.Case{}, ErrForbidden
	}
	if current.State == "approved" && !in.ConfirmedNotDisbursed {
		return store.Case{}, ErrInvalid
	}
	exists, err := store.HasPaymentIntents(ctx, tx, id)
	if err != nil {
		return store.Case{}, err
	}
	if exists || current.FundedAt != nil {
		return store.Case{}, ErrConflict
	}
	out, err := store.RevertReview(ctx, tx, id, in.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.Case{}, ErrConflict
	}
	if err != nil {
		return store.Case{}, err
	}
	if err = store.Audit(ctx, tx, id, actor, "revert_review", current, map[string]any{"case": out, "note": in.Note, "confirmed_not_disbursed": in.ConfirmedNotDisbursed}); err != nil {
		return store.Case{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return store.Case{}, err
	}
	return out, nil
}

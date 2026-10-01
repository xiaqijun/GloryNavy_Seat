package exchange

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/exchange/internal/store"
	"strconv"
)

// WelfareGrantTx accepts a confirmed welfare record's absolute remaining entitlement.
// Both administrator grants and configured growth rewards use the globally unique case ID.
// Only the host injects this into welfare; no public arbitrary-credit endpoint exists.
func (s *Service) WelfareGrantTx(ctx context.Context, tx pgx.Tx, account string, id, previous, amount int64, key, reason string) error {
	if id <= 0 || previous < 0 || amount < 0 || amount > 1000000000000 {
		return ErrInvalid
	}
	if amount > previous && !s.AllowNew {
		return ErrUnavailable
	}
	actor, e := uuid(account)
	if e != nil {
		return e
	}
	q := store.New(tx)
	if e = q.LockAccount(ctx, account); e != nil {
		return e
	}
	ref := strconv.FormatInt(id, 10)
	old, e := q.SourceAward(ctx, store.SourceAwardParams{SourceID: "welfare_grant", Reference: ref})
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return e
	}
	if e == nil && (old.AccountID != actor || old.Coins != previous) {
		return ErrConflict
	}
	if e == pgx.ErrNoRows && previous != 0 {
		return ErrConflict
	}
	return saveCoins(ctx, tx, "welfare_grant", key, reason, []coinPlan{{Award: Award{Reference: ref, AccountID: account}, Units: amount, Rate: 1, PreviousCoins: previous, Coins: amount}})
}

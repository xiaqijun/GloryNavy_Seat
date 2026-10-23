package exchange

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/exchange/internal/store"
	"strconv"
)

// FeedbackGrantTx is injected only by the host. The feedback transaction owns
// adoption and the once-only ticket claim; no client controls its amount.
func (s *Service) FeedbackGrantTx(ctx context.Context, tx pgx.Tx, actor, account string, id, amount int64) error {
	if id <= 0 || amount <= 0 || amount > 1000000000000 || actor == account {
		return ErrInvalid
	}
	if !s.AllowNew {
		return ErrUnavailable
	}
	if err := s.shopAdmin(ctx, actor); err != nil {
		return err
	}
	owner, err := uuid(account)
	if err != nil {
		return err
	}
	q := store.New(tx)
	if err = q.LockAccount(ctx, account); err != nil {
		return err
	}
	ref := strconv.FormatInt(id, 10)
	old, err := q.SourceAward(ctx, store.SourceAwardParams{SourceID: "feedback_reward", Reference: ref})
	if err == nil {
		if old.AccountID != owner || old.Coins != amount {
			return ErrConflict
		}
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	// The ticket identity is a stable source key even after an account merge.
	return saveCoins(ctx, tx, "feedback_reward", "00000000-0000-4000-8000-000000000104", "反馈采纳奖励 #"+ref, []coinPlan{{Award: Award{Reference: ref, AccountID: account}, Units: amount, Rate: 1, Coins: amount}})
}

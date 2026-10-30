package exchange

import (
	"context"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/exchange/internal/store"
)

func (s *Service) guardRevertCancelReview(ctx context.Context, tx pgx.Tx, id int64) error {
	ok, err := store.LastDecisionWasRejectedCancellation(ctx, tx, id)
	if err != nil {
		return err
	}
	if !ok {
		return ErrConflict
	}
	delivery, err := store.ReadDelivery(ctx, tx, id)
	if err != nil {
		return err
	}
	if !withdrawable(delivery) {
		return ErrDeliveryPending
	}
	return nil
}

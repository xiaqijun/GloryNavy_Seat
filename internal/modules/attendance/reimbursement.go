package attendance

import (
	"context"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/attendance/internal/store"
)

// ConfirmedReimbursementLosses is a host-only contract. The caller authorizes
// the raw character first; attendance checks historical ownership and presence.
func (s *Service) ConfirmedReimbursementLosses(ctx context.Context, account string, corp, char int64, ids []int64) (map[int64]int64, error) {
	return store.ConfirmedLossEvents(ctx, s.Pool, account, corp, char, ids, false)
}
func (s *Service) LockReimbursementLoss(ctx context.Context, tx pgx.Tx, account string, corp, char, km int64) (int64, error) {
	rows, err := store.ConfirmedLossEvents(ctx, tx, account, corp, char, []int64{km}, true)
	if err != nil {
		return 0, err
	}
	if id := rows[km]; id > 0 {
		return id, nil
	}
	return 0, pgx.ErrNoRows
}

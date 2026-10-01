package identity

import (
	"context"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/identity/internal/store"
)

// LockActiveAccounts fences new account-owned writes against concurrent merges.
// Call after any character locks and before module or wallet locks.
func (s *Service) LockActiveAccounts(ctx context.Context, tx pgx.Tx, ids []string) error {
	return store.LockActiveAccounts(ctx, tx, ids)
}

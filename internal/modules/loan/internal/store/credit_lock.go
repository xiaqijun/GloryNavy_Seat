package store

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"
)

// LockCreditAccounts serializes new loan and guarantee commitments. Identity's
// account guard fences merges but permits concurrent business transactions.
// Acquire these locks after that guard and before pool or case row locks.
func LockCreditAccounts(ctx context.Context, tx pgx.Tx, accounts ...string) error {
	ordered := slices.Clone(accounts)
	slices.Sort(ordered)
	for _, account := range slices.Compact(ordered) {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "loan-credit:"+account); err != nil {
			return err
		}
	}
	return nil
}

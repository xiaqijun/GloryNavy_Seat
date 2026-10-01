package store

import (
	"context"
	"github.com/jackc/pgx/v5"
	"slices"
)

func LockActiveAccounts(ctx context.Context, tx pgx.Tx, ids []string) error {
	ids = slices.Clone(ids)
	slices.Sort(ids)
	ids = slices.Compact(ids)
	for _, id := range ids {
		var found string
		if err := tx.QueryRow(ctx, `SELECT id::text FROM identity_users WHERE id=$1 FOR KEY SHARE`, id).Scan(&found); err != nil {
			return err
		}
		var retired bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM identity_account_merges WHERE source_id=$1)`, id).Scan(&retired); err != nil {
			return err
		}
		if retired {
			return pgx.ErrNoRows
		}
	}
	return nil
}

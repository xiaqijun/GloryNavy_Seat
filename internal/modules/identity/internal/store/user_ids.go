package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

// AllUserIDs is a narrow module-private query used by scheduled account-level
// jobs. It intentionally returns identifiers only; callers cannot infer or
// mutate identity data from this helper.
func AllUserIDs(ctx context.Context, db DBTX) ([]pgtype.UUID, error) {
	rows, err := db.Query(ctx, `SELECT id FROM identity_users WHERE NOT EXISTS (SELECT 1 FROM identity_account_merges WHERE source_id=identity_users.id) ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []pgtype.UUID{}
	for rows.Next() {
		var id pgtype.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

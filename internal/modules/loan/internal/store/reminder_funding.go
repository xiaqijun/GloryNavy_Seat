package store

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
)

func FundingReminderKeys(ctx context.Context, db *pgxpool.Pool, account string, pools []int64) ([]string, error) {
	rows, err := db.Query(ctx, `SELECT id::text FROM loan_cases WHERE pool_id=ANY($2::bigint[]) AND borrower_account_id<>$1 AND state IN ('approved','funding')`, account, pools)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var k string
		if err = rows.Scan(&k); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

package store

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
)

func MergeAccount(ctx context.Context, tx pgx.Tx, source, target string, apply bool) (json.RawMessage, error) {
	if apply {
		if _, err := tx.Exec(ctx, `DELETE FROM eve_login_flows WHERE target_user_id IN($1::uuid,$2::uuid)`, source, target); err != nil {
			return nil, err
		}
	}
	return json.RawMessage(`{}`), nil
}

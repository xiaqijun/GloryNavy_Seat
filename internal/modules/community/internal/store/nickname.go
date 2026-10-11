package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

// ListNicknames returns account-scoped nicknames in one query for host
// composition roots that need to decorate member directories.
func (q *Queries) ListNicknames(ctx context.Context, userIDs []string) (map[string]string, error) {
	out := make(map[string]string, len(userIDs))
	if len(userIDs) == 0 {
		return out, nil
	}
	rows, err := q.db.Query(ctx, `
		SELECT user_id::text, trim(kook_name)
		FROM community_profiles
		WHERE user_id::text = ANY($1::text[])
		  AND nullif(trim(kook_name), '') IS NOT NULL`, userIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out[id] = name
	}
	return out, rows.Err()
}

// LockNickname serializes writes for the normalized nickname. The lock is
// transaction-scoped, so the subsequent ownership check is race-safe without
// changing the legacy schema or blocking unrelated nicknames.
func (q *Queries) LockNickname(ctx context.Context, value string) error {
	_, err := q.db.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext(lower(trim($1))))`, value)
	return err
}

// NicknameTaken reports whether another site account already owns value. Empty
// legacy rows are ignored; callers validate non-empty nicknames first.
func (q *Queries) NicknameTaken(ctx context.Context, value string, userID pgtype.UUID) (bool, error) {
	var taken bool
	err := q.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM community_profiles
			WHERE user_id<>$1
			  AND nullif(trim(kook_name), '') IS NOT NULL
			  AND lower(trim(kook_name))=lower(trim($2))
		)`, userID, value).Scan(&taken)
	return taken, err
}

package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

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

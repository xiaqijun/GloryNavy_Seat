package store

import (
	"context"
	"time"
)

// Update only an existing, unexpired session with the same active identity.
// Never insert a session: logout, merge and revocation cannot be undone by renewal.
func RenewSession(ctx context.Context, db DBTX, hash []byte, seconds int64) (time.Time, error) {
	var expires time.Time
	err := db.QueryRow(ctx, `UPDATE identity_sessions s
 SET expires_at=greatest(s.expires_at,statement_timestamp()+$2::bigint*interval '1 second')
 FROM identity_characters c,identity_users u,identity_characters m
 WHERE s.token_hash=$1 AND s.expires_at>statement_timestamp()
 AND c.character_id=s.character_id AND c.user_id=s.user_id AND c.status='active'
 AND u.id=s.user_id AND m.character_id=u.main_character_id AND m.user_id=u.id
 AND NOT EXISTS(SELECT 1 FROM identity_account_merges a WHERE a.source_id=s.user_id)
 RETURNING s.expires_at`, hash, seconds).Scan(&expires)
	return expires, err
}

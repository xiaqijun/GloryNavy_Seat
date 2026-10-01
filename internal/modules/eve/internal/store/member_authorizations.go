package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// MemberAuthorization is the token-free subset needed to verify current
// character ownership and corporation membership in a single database read.
type MemberAuthorization struct {
	CharacterID   int64
	State         string
	OwnerHash     []byte
	CorporationID int64
	ValidUntil    time.Time
}

func (q *Queries) MemberAuthorizations(ctx context.Context, ids []int64) ([]MemberAuthorization, error) {
	if len(ids) == 0 {
		return []MemberAuthorization{}, nil
	}
	rows, err := q.db.Query(ctx, `SELECT c.character_id,c.state,c.owner_hash,
		s.corporation_id,s.valid_until
		FROM eve_credentials c LEFT JOIN eve_role_snapshots s USING(character_id)
		WHERE c.character_id=ANY($1::bigint[])`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]MemberAuthorization, 0, len(ids))
	for rows.Next() {
		var a MemberAuthorization
		var corp pgtype.Int8
		var validUntil pgtype.Timestamptz
		if err := rows.Scan(&a.CharacterID, &a.State, &a.OwnerHash, &corp, &validUntil); err != nil {
			return nil, err
		}
		if corp.Valid {
			a.CorporationID = corp.Int64
		}
		if validUntil.Valid {
			a.ValidUntil = validUntil.Time
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

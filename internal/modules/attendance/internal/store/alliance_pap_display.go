package store

import (
	"context"
	"time"
)

// AlliancePAPAccountMonthCharacters is a read-only projection. The caller must
// verify the month's complete publication and the current character bindings.
func (q *Queries) AlliancePAPAccountMonthCharacters(ctx context.Context, account string, month time.Time) ([]AlliancePAPCharacter, error) {
	rows, err := q.db.Query(ctx, `
		SELECT character_id, character_name, pap::text
		FROM attendance_alliance_pap_snapshot
		WHERE month=$1 AND account_id=$2::uuid
		ORDER BY pap DESC, character_name, character_id`, month, account)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AlliancePAPCharacter{}
	for rows.Next() {
		var item AlliancePAPCharacter
		if err := rows.Scan(&item.CharacterID, &item.CharacterName, &item.PAP); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

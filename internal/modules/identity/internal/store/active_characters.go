package store

import "context"

type ActiveCharacter struct {
	CharacterID int64
	Name        string
}

// AllActiveCharacters is called only after the host service has checked its
// administrator or business scope.
func (q *Queries) AllActiveCharacters(ctx context.Context) ([]ActiveCharacter, error) {
	rows, err := q.db.Query(ctx, `SELECT character_id,name FROM identity_characters WHERE status='active' ORDER BY name,character_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ActiveCharacter, 0)
	for rows.Next() {
		var item ActiveCharacter
		if err := rows.Scan(&item.CharacterID, &item.Name); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

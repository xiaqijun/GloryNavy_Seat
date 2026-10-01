package store

import "context"

// MainCharacterNames returns names only for explicitly supplied active accounts.
func (q *Queries) MainCharacterNames(ctx context.Context, accounts []string) (map[string]string, error) {
	rows, err := q.db.Query(ctx, `SELECT u.id::text,c.name
		FROM identity_users u JOIN identity_characters c
		ON c.character_id=u.main_character_id AND c.user_id=u.id AND c.status='active'
		WHERE u.id=ANY($1::uuid[]) AND NOT EXISTS
		(SELECT 1 FROM identity_account_merges m WHERE m.source_id=u.id)`, accounts)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]string)
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out[id] = name
	}
	return out, rows.Err()
}

// MainCharacterIDs returns main-character IDs only for explicitly supplied
// active accounts. Source accounts that have already been merged are excluded.
func (q *Queries) MainCharacterIDs(ctx context.Context, accounts []string) (map[string]int64, error) {
	rows, err := q.db.Query(ctx, `SELECT u.id::text,u.main_character_id
		FROM identity_users u JOIN identity_characters c
		ON c.character_id=u.main_character_id AND c.user_id=u.id AND c.status='active'
		WHERE u.id=ANY($1::uuid[]) AND NOT EXISTS
		(SELECT 1 FROM identity_account_merges m WHERE m.source_id=u.id)`, accounts)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]int64)
	for rows.Next() {
		var id string
		var characterID int64
		if err := rows.Scan(&id, &characterID); err != nil {
			return nil, err
		}
		out[id] = characterID
	}
	return out, rows.Err()
}

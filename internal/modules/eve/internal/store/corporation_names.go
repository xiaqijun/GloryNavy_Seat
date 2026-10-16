package store

import "context"

type CorporationName struct {
	ID   int64
	Name string
}

// These labels are only for configuration, never authorization facts. Keep
// expired labels usable so an administrator can select a pending corporation.
func KnownCorporationNames(ctx context.Context, db DBTX) ([]CorporationName, error) {
	rows, err := db.Query(ctx, `SELECT DISTINCT ON (corporation_id) corporation_id, corporation_name
		FROM eve_role_snapshots WHERE corporation_id>0 AND corporation_name<>''
		ORDER BY corporation_id, synced_at DESC, character_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CorporationName{}
	for rows.Next() {
		var r CorporationName
		if err := rows.Scan(&r.ID, &r.Name); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

package store

import "context"

type ExactType struct {
	Input string
	ID    int64
	Name  string
}

func ResolveTypeNames(ctx context.Context, db DBTX, names []string) ([]ExactType, error) {
	rows, err := db.Query(ctx, `SELECT i.name,n.type_id,coalesce(nullif(n.name_zh,''),n.name_en) FROM unnest($1::text[]) i(name) JOIN eve_sde_type_names n ON lower(n.name_en)=i.name OR lower(n.name_zh)=i.name JOIN eve_sde_active_names a ON a.release_id=n.release_id ORDER BY i.name,n.type_id`, names)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ExactType{}
	for rows.Next() {
		var v ExactType
		if err = rows.Scan(&v.Input, &v.ID, &v.Name); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type SystemSearchResult struct {
	ID     int64
	NameZh string
	NameEn string
}

// SearchSDESystems only reads the active published SDE release. The term is a
// literal substring, so wildcard characters do not expand the search.
func SearchSDESystems(ctx context.Context, pool *pgxpool.Pool, term string) ([]SystemSearchResult, error) {
	rows, err := pool.Query(ctx, `SELECT n.solar_system_id,n.name_zh,n.name_en
FROM eve_sde_solar_system_names n
JOIN eve_sde_active_names a ON a.release_id=n.release_id
WHERE strpos(lower(n.name_zh),lower($1))>0 OR strpos(lower(n.name_en),lower($1))>0 OR n.solar_system_id::text=$1
ORDER BY n.name_en,n.solar_system_id LIMIT 30`, term)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []SystemSearchResult{}
	for rows.Next() {
		var item SystemSearchResult
		if err := rows.Scan(&item.ID, &item.NameZh, &item.NameEn); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

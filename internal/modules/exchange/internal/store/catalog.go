package store

import (
	"context"
	"encoding/json"
)

func (q *Queries) ReadReward(ctx context.Context, id int64) (ExchangeReward, error) {
	var r ExchangeReward
	err := q.db.QueryRow(ctx, `SELECT id,version,content,archived FROM exchange_rewards WHERE id=$1`, id).Scan(&r.ID, &r.Version, &r.Content, &r.Archived)
	return r, err
}

type Catalog struct {
	ID       int64           `json:"id,string"`
	Version  int64           `json:"version,string"`
	Name     string          `json:"name"`
	Content  json.RawMessage `json:"content"`
	Archived bool            `json:"archived"`
}

func (q *Queries) CatalogList(ctx context.Context, after int64) ([]Catalog, error) {
	rows, err := q.db.Query(ctx, `SELECT id,catalog_version,name,content,archived FROM exchange_rewards WHERE id>$1 ORDER BY id LIMIT 51`, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Catalog{}
	for rows.Next() {
		var r Catalog
		if err = rows.Scan(&r.ID, &r.Version, &r.Name, &r.Content, &r.Archived); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (q *Queries) CatalogRead(ctx context.Context, id int64) (Catalog, error) {
	var r Catalog
	err := q.db.QueryRow(ctx, `SELECT id,catalog_version,name,content,archived FROM exchange_rewards WHERE id=$1`, id).Scan(&r.ID, &r.Version, &r.Name, &r.Content, &r.Archived)
	return r, err
}
func (q *Queries) CatalogSave(ctx context.Context, r Catalog, cover int64) (int64, error) {
	if r.ID == 0 {
		err := q.db.QueryRow(ctx, `INSERT INTO exchange_rewards(type_id,quantity,isk_value,stock,name,content,archived) VALUES($1,1,1,0,$2,$3,$4) RETURNING id`, cover, r.Name, r.Content, r.Archived).Scan(&r.ID)
		return r.ID, err
	}
	_, err := q.db.Exec(ctx, `UPDATE exchange_rewards SET type_id=$2,quantity=1,name=$3,content=$4,archived=$5,catalog_version=catalog_version+1,version=version+1,enabled=false,stock=0 WHERE id=$1`, r.ID, cover, r.Name, r.Content, r.Archived)
	return r.ID, err
}
func (q *Queries) SeedRewardContent(ctx context.Context, id int64) error {
	_, err := q.db.Exec(ctx, `UPDATE exchange_rewards SET content=jsonb_build_object('fittings','[]'::jsonb,'items',jsonb_build_array(jsonb_build_object('type_id',type_id::text,'quantity',quantity))) WHERE id=$1`, id)
	return err
}
func (q *Queries) SnapshotRedemption(ctx context.Context, id int64, r Catalog) error {
	_, err := q.db.Exec(ctx, `UPDATE exchange_redemptions SET reward_name=$2,reward_content=$3,catalog_version=$4 WHERE id=$1`, id, r.Name, r.Content, r.Version)
	return err
}

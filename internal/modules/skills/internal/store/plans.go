// Package store owns skills SQL; other modules access the skills service instead.
package store

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type Plan struct {
	ID            int64           `json:"id,string"`
	CorporationID int64           `json:"corporation_id,string"`
	Name          string          `json:"name"`
	Requirements  json.RawMessage `json:"requirements"`
	Version       int64           `json:"version,string"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

func scan(row pgx.Row) (p Plan, err error) {
	err = row.Scan(&p.ID, &p.CorporationID, &p.Name, &p.Requirements, &p.Version, &p.UpdatedAt)
	return
}

const cols = "id,corporation_id,name,requirements,version,updated_at"

func List(ctx context.Context, pool *pgxpool.Pool, corp int64) ([]Plan, error) {
	rows, err := pool.Query(ctx, "SELECT "+cols+" FROM skills_plans WHERE corporation_id=$1 ORDER BY id DESC", corp)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Plan{}
	for rows.Next() {
		p, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
func Read(ctx context.Context, pool *pgxpool.Pool, id int64) (Plan, error) {
	return scan(pool.QueryRow(ctx, "SELECT "+cols+" FROM skills_plans WHERE id=$1", id))
}

// Change holds the row lock through version checks and audit publication.
func Change(ctx context.Context, pool *pgxpool.Pool, user, key string, p Plan, remove bool) (Plan, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Plan{}, err
	}
	defer tx.Rollback(context.Background())
	var before, after []byte
	action := "create"
	if p.ID > 0 {
		old, e := scan(tx.QueryRow(ctx, "SELECT "+cols+" FROM skills_plans WHERE id=$1 AND corporation_id=$2 AND version=$3 FOR UPDATE", p.ID, p.CorporationID, p.Version))
		if e != nil {
			return Plan{}, e
		}
		before, _ = json.Marshal(old)
		if remove {
			p = old
			action = "delete"
			_, err = tx.Exec(ctx, "DELETE FROM skills_plans WHERE id=$1", p.ID)
		} else {
			action = "update"
			p, err = scan(tx.QueryRow(ctx, "UPDATE skills_plans SET name=$2,requirements=$3,version=version+1,updated_at=now() WHERE id=$1 RETURNING "+cols, p.ID, p.Name, p.Requirements))
		}
	} else {
		// Serialise retries of a create and cap unbounded plan growth per corporation.
		_, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", p.CorporationID)
		if err != nil {
			return Plan{}, err
		}
		old, e := scan(tx.QueryRow(ctx, "SELECT "+cols+" FROM skills_plans WHERE created_by=$1 AND request_key=$2", user, key))
		if e == nil {
			if old.CorporationID != p.CorporationID || old.Name != p.Name || string(old.Requirements) != string(p.Requirements) {
				// JSONB whitespace is normalized before semantic comparison.
				var a, b any
				_ = json.Unmarshal(old.Requirements, &a)
				_ = json.Unmarshal(p.Requirements, &b)
				aa, _ := json.Marshal(a)
				bb, _ := json.Marshal(b)
				if old.CorporationID != p.CorporationID || old.Name != p.Name || string(aa) != string(bb) {
					return Plan{}, pgx.ErrNoRows
				}
			}
			return old, tx.Commit(ctx)
		}
		if e != pgx.ErrNoRows {
			return Plan{}, e
		}
		p, err = scan(tx.QueryRow(ctx, "INSERT INTO skills_plans(corporation_id,name,requirements,created_by,request_key) SELECT $1,$2,$3,$4,$5 WHERE (SELECT count(*) FROM skills_plans WHERE corporation_id=$1)<100 RETURNING "+cols, p.CorporationID, p.Name, p.Requirements, user, key))
	}
	if err != nil {
		return Plan{}, err
	}
	if !remove {
		after, _ = json.Marshal(p)
	}
	_, err = tx.Exec(ctx, "INSERT INTO skills_plan_audit(plan_id,corporation_id,actor_id,action,before_value,after_value) VALUES($1,$2,$3,$4,$5,$6)", p.ID, p.CorporationID, user, action, before, after)
	if err != nil {
		return Plan{}, err
	}
	return p, tx.Commit(ctx)
}

func GuardVersion(ctx context.Context, tx pgx.Tx, corp, id, version int64) error {
	var actual int64
	err := tx.QueryRow(ctx, "SELECT version FROM skills_plans WHERE id=$1 AND corporation_id=$2 FOR SHARE", id, corp).Scan(&actual)
	if err != nil {
		return err
	}
	if actual != version {
		return pgx.ErrNoRows
	}
	return nil
}

package store

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type Library struct {
	ID            int64           `json:"id,string"`
	CorporationID int64           `json:"corporation_id,string"`
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	Fit           json.RawMessage `json:"fit"`
	Version       int64           `json:"version,string"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

const libraryColumns = "id,corporation_id,name,description,fit,version,updated_at"

func scanLibrary(r pgx.Row) (v Library, err error) {
	err = r.Scan(&v.ID, &v.CorporationID, &v.Name, &v.Description, &v.Fit, &v.Version, &v.UpdatedAt)
	return
}
func LibraryList(ctx context.Context, pool *pgxpool.Pool, corp int64) ([]Library, error) {
	rows, err := pool.Query(ctx, "SELECT "+libraryColumns+" FROM fittings_library WHERE corporation_id=$1 ORDER BY id DESC", corp)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Library{}
	for rows.Next() {
		v, e := scanLibrary(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func LibraryRead(ctx context.Context, pool *pgxpool.Pool, id int64) (Library, error) {
	return scanLibrary(pool.QueryRow(ctx, "SELECT "+libraryColumns+" FROM fittings_library WHERE id=$1", id))
}
func LibrarySave(ctx context.Context, pool *pgxpool.Pool, actor, key string, v Library, remove bool) (Library, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return v, err
	}
	defer tx.Rollback(context.Background())
	action := "import"
	var before, after []byte
	if v.ID > 0 {
		old, e := scanLibrary(tx.QueryRow(ctx, "SELECT "+libraryColumns+" FROM fittings_library WHERE id=$1 AND corporation_id=$2 AND version=$3 FOR UPDATE", v.ID, v.CorporationID, v.Version))
		if e != nil {
			return v, e
		}
		before, _ = json.Marshal(old)
		if remove {
			action = "delete"
			v = old
			_, err = tx.Exec(ctx, "DELETE FROM fittings_library WHERE id=$1", v.ID)
		} else {
			action = "update"
			v, err = scanLibrary(tx.QueryRow(ctx, "UPDATE fittings_library SET name=$2,description=$3,fit=$4,version=version+1,updated_at=now() WHERE id=$1 RETURNING "+libraryColumns, v.ID, v.Name, v.Description, v.Fit))
		}
	} else {
		_, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", v.CorporationID)
		if err != nil {
			return v, err
		}
		old, e := scanLibrary(tx.QueryRow(ctx, "SELECT "+libraryColumns+" FROM fittings_library WHERE created_by=$1 AND request_key=$2", actor, key))
		if e == nil {
			var a, b any
			_ = json.Unmarshal(old.Fit, &a)
			_ = json.Unmarshal(v.Fit, &b)
			aa, _ := json.Marshal(a)
			bb, _ := json.Marshal(b)
			if old.CorporationID != v.CorporationID || old.Name != v.Name || old.Description != v.Description || string(aa) != string(bb) {
				return v, pgx.ErrNoRows
			}
			return old, tx.Commit(ctx)
		}
		if e != pgx.ErrNoRows {
			return v, e
		}
		v, err = scanLibrary(tx.QueryRow(ctx, "INSERT INTO fittings_library(corporation_id,name,description,fit,created_by,request_key) SELECT $1,$2,$3,$4,$5,$6 WHERE (SELECT count(*) FROM fittings_library WHERE corporation_id=$1)<300 RETURNING "+libraryColumns, v.CorporationID, v.Name, v.Description, v.Fit, actor, key))
	}
	if err != nil {
		return v, err
	}
	if !remove {
		after, _ = json.Marshal(v)
	}
	_, err = tx.Exec(ctx, "INSERT INTO fittings_library_audit(library_id,actor_id,action,before_value,after_value) VALUES($1,$2,$3,$4,$5)", v.ID, actor, action, before, after)
	if err != nil {
		return v, err
	}
	return v, tx.Commit(ctx)
}

type GameSave struct {
	ID        int64  `json:"id,string"`
	State     string `json:"state"`
	FittingID *int64 `json:"fitting_id,string"`
	Reason    string `json:"reason"`
}

// ReserveSend serialises duplicates without keeping a transaction open during network I/O.
func ReserveSend(ctx context.Context, pool *pgxpool.Pool, actor, key string, char, lib, version int64) (GameSave, bool, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return GameSave{}, false, err
	}
	defer tx.Rollback(context.Background())
	_, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", char)
	if err != nil {
		return GameSave{}, false, err
	}
	var oldChar, oldLib, oldVersion int64
	err = tx.QueryRow(ctx, "SELECT character_id,library_id,library_version FROM fittings_game_saves WHERE account_id=$1 AND request_key=$2", actor, key).Scan(&oldChar, &oldLib, &oldVersion)
	if err == nil && (oldChar != char || oldLib != lib || oldVersion != version) {
		return GameSave{}, false, pgx.ErrNoRows
	}
	if err != nil && err != pgx.ErrNoRows {
		return GameSave{}, false, err
	}
	v := GameSave{}
	var at time.Time
	err = tx.QueryRow(ctx, "SELECT id,state,fitting_id,reason,created_at FROM fittings_game_saves WHERE account_id=$1 AND character_id=$2 AND library_id=$3 AND library_version=$4 FOR UPDATE", actor, char, lib, version).Scan(&v.ID, &v.State, &v.FittingID, &v.Reason, &at)
	if err == nil {
		if v.State == "failed" {
			_, err = tx.Exec(ctx, "UPDATE fittings_game_saves SET state='sending',reason='',updated_at=now(),created_at=now() WHERE id=$1", v.ID)
			v.State = "sending"
			v.Reason = ""
			if err != nil {
				return v, false, err
			}
			return v, true, tx.Commit(ctx)
		}
		if v.State == "sending" && time.Since(at) > time.Minute {
			v.State = "unknown"
			v.Reason = "confirmation_required"
		}
		return v, false, tx.Commit(ctx)
	}
	if err != pgx.ErrNoRows {
		return v, false, err
	}
	err = tx.QueryRow(ctx, "INSERT INTO fittings_game_saves(account_id,character_id,library_id,library_version,request_key,state) VALUES($1,$2,$3,$4,$5,'sending') RETURNING id,state,reason", actor, char, lib, version, key).Scan(&v.ID, &v.State, &v.Reason)
	if err != nil {
		return v, false, err
	}
	return v, true, tx.Commit(ctx)
}
func FinishSend(ctx context.Context, pool *pgxpool.Pool, id int64, state, reason string, fitID int64) error {
	_, err := pool.Exec(ctx, "UPDATE fittings_game_saves SET state=$2,reason=$3,fitting_id=nullif($4,0),updated_at=now() WHERE id=$1 AND state='sending'", id, state, reason, fitID)
	return err
}

func GuardLibraryVersion(ctx context.Context, tx pgx.Tx, corp, id, version int64) error {
	var actual int64
	err := tx.QueryRow(ctx, "SELECT version FROM fittings_library WHERE id=$1 AND corporation_id=$2 FOR SHARE", id, corp).Scan(&actual)
	if err != nil {
		return err
	}
	if actual != version {
		return pgx.ErrNoRows
	}
	return nil
}

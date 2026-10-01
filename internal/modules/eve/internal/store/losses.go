package store

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
)

func SeedLossTargets(ctx context.Context, tx pgx.Tx) error {
	_, e := tx.Exec(ctx, `INSERT INTO eve_sync_targets(character_id,resource,generation,display_name,state,reason)
 SELECT c.character_id,'killmails',c.grant_generation,coalesce(p.name,''),
 CASE WHEN c.state='reauthorize' OR NOT 'esi-killmails.read_killmails.v1'=ANY(c.scopes) THEN 'blocked' ELSE 'idle' END,
 CASE WHEN c.state='reauthorize' THEN 'reauthorize' WHEN NOT 'esi-killmails.read_killmails.v1'=ANY(c.scopes) THEN 'missing_scope' ELSE '' END
 FROM eve_credentials c LEFT JOIN eve_character_profiles p USING(character_id)
 ON CONFLICT(character_id,resource) DO UPDATE SET generation=excluded.generation,state=excluded.state,reason=excluded.reason,active_job_id=NULL,lease_until=NULL,fence=eve_sync_targets.fence+1,next_due_at=now(),failures=0
 WHERE eve_sync_targets.generation<>excluded.generation OR eve_sync_targets.reason='module_disabled'`)
	return e
}
func LossCursor(ctx context.Context, db DBTX, target, generation int64) (json.RawMessage, error) {
	var b json.RawMessage
	e := db.QueryRow(ctx, `SELECT payload FROM eve_loss_cursors WHERE target_id=$1 AND generation=$2`, target, generation).Scan(&b)
	return b, e
}
func SaveLossCursor(ctx context.Context, db DBTX, target, generation int64, b []byte) error {
	_, e := db.Exec(ctx, `INSERT INTO eve_loss_cursors(target_id,generation,payload) VALUES($1,$2,$3) ON CONFLICT(target_id) DO UPDATE SET generation=excluded.generation,payload=excluded.payload`, target, generation, b)
	return e
}
func DeleteLossCursor(ctx context.Context, db DBTX, target int64) error {
	_, e := db.Exec(ctx, `DELETE FROM eve_loss_cursors WHERE target_id=$1`, target)
	return e
}
func KillmailSeen(ctx context.Context, db DBTX, character, id int64, owner []byte, hash string) (bool, error) {
	var b bool
	e := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM eve_character_killmails WHERE character_id=$1 AND killmail_id=$2 AND owner_hash=$3 AND killmail_hash=$4 AND (payload IS NULL OR payload ? 'attackers'))`, character, id, owner, hash).Scan(&b)
	return b, e
}

// A saved hash can still retrieve a detail after the killmail leaves the recent list.
// Select one per completed pass so archived backfill cannot delay current reports.
func ArchivedLossMissingAttackers(ctx context.Context, db DBTX, character int64, owner []byte) (int64, string, error) {
	var id int64
	var hash string
	e := db.QueryRow(ctx, `SELECT killmail_id,killmail_hash FROM eve_character_killmails
 WHERE character_id=$1 AND owner_hash=$2 AND victim_id=character_id AND payload IS NOT NULL
 AND NOT payload ? 'attackers' ORDER BY occurred_at ASC,killmail_id ASC LIMIT 1`, character, owner).Scan(&id, &hash)
	return id, hash, e
}
func SaveKillmail(ctx context.Context, db DBTX, character, id, victim int64, owner []byte, hash string, at, observed any, payload []byte) error {
	_, e := db.Exec(ctx, `INSERT INTO eve_character_killmails(character_id,killmail_id,owner_hash,killmail_hash,victim_id,occurred_at,observed_at,payload) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(character_id,killmail_id) DO UPDATE SET owner_hash=excluded.owner_hash,killmail_hash=excluded.killmail_hash,victim_id=excluded.victim_id,occurred_at=excluded.occurred_at,observed_at=excluded.observed_at,payload=excluded.payload`, character, id, owner, hash, victim, at, observed, payload)
	return e
}

// Caller enforces identity/object access; live credential and owner binding also fence stored evidence.
func CharacterLosses(ctx context.Context, db DBTX, character, before, id int64) ([]json.RawMessage, error) {
	rows, e := db.Query(ctx, `SELECT k.payload FROM eve_character_killmails k JOIN eve_credentials c USING(character_id)
 WHERE k.character_id=$1 AND k.owner_hash=c.owner_hash AND c.state<>'reauthorize' AND 'esi-killmails.read_killmails.v1'=ANY(c.scopes)
 AND k.victim_id=k.character_id AND k.payload IS NOT NULL AND ($2::bigint=0 OR k.killmail_id<$2) AND ($3::bigint=0 OR k.killmail_id=$3)
 ORDER BY k.killmail_id DESC LIMIT 31`, character, before, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []json.RawMessage{}
	for rows.Next() {
		var b json.RawMessage
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

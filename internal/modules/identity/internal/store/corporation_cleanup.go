package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

type CleanupPolicy struct {
	Enabled      bool       `json:"enabled"`
	Corporations []int64    `json:"-"`
	Version      int64      `json:"version,string"`
	NextRun      *time.Time `json:"next_run_at"`
	Started      *time.Time `json:"run_started_at"`
	After        *string    `json:"-"`
	Removed      int        `json:"removed_count"`
	Skipped      int        `json:"skipped_count"`
	LastRun      *time.Time `json:"last_run_at"`
	LastRemoved  int        `json:"last_removed_count"`
	LastSkipped  int        `json:"last_skipped_count"`
}

func ReadCleanupPolicy(ctx context.Context, db DBTX, lock bool) (CleanupPolicy, error) {
	q := `SELECT enabled,corporation_ids,version,next_run_at,run_started_at,after_user_id::text,removed_count,skipped_count,last_run_at,last_removed_count,last_skipped_count FROM identity_corporation_cleanup WHERE id`
	if lock {
		q += ` FOR UPDATE SKIP LOCKED`
	}
	var p CleanupPolicy
	err := db.QueryRow(ctx, q).Scan(&p.Enabled, &p.Corporations, &p.Version, &p.NextRun, &p.Started, &p.After, &p.Removed, &p.Skipped, &p.LastRun, &p.LastRemoved, &p.LastSkipped)
	return p, err
}

func SaveCleanupPolicy(ctx context.Context, tx pgx.Tx, actor string, enabled bool, ids []int64, version int64) error {
	tag, err := tx.Exec(ctx, `UPDATE identity_corporation_cleanup SET enabled=$1,corporation_ids=$2,version=version+1,
 next_run_at=CASE WHEN NOT $1 THEN NULL ELSE now()+interval '7 days' END,
 run_started_at=NULL,after_user_id=NULL,removed_count=0,skipped_count=0,updated_at=now() WHERE id AND version=$3`, enabled, ids, version)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return pgx.ErrNoRows
	}
	return CleanupAudit(ctx, tx, actor, "configured", map[string]any{"enabled": enabled, "corporation_ids": ids, "version": version + 1})
}

type CleanupCharacter struct {
	ID     int64  `json:"character_id,string"`
	User   string `json:"account_id"`
	Name   string `json:"name"`
	Owner  []byte `json:"-"`
	Status string `json:"-"`
}

func CleanupCharacters(ctx context.Context, db DBTX, user string) ([]CleanupCharacter, error) {
	rows, err := db.Query(ctx, `SELECT character_id,user_id::text,name,owner_hash,status FROM identity_characters WHERE ($1='' OR user_id::text=$1) ORDER BY character_id`, user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CleanupCharacter{}
	for rows.Next() {
		var c CleanupCharacter
		if err = rows.Scan(&c.ID, &c.User, &c.Name, &c.Owner, &c.Status); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func NextCleanupUser(ctx context.Context, db DBTX, after *string) (string, error) {
	var user string
	err := db.QueryRow(ctx, `SELECT DISTINCT user_id::text FROM identity_characters WHERE ($1::uuid IS NULL OR user_id>$1::uuid) ORDER BY user_id::text LIMIT 1`, after).Scan(&user)
	return user, err
}

func CleanupAudit(ctx context.Context, db DBTX, actor, action string, detail any) error {
	raw, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, `INSERT INTO identity_cleanup_audit(actor,action,detail) VALUES($1,$2,$3)`, actor, action, raw)
	return err
}

func RemoveCleanupCharacter(ctx context.Context, tx pgx.Tx, c CleanupCharacter, corp int64) error {
	_, err := tx.Exec(ctx, `INSERT INTO identity_cleanup_removed(character_id,user_id,original_user_id,owner_hash,name,corporation_id)
 VALUES($1,$2,$2,$3,$4,$5) ON CONFLICT(character_id) DO UPDATE SET user_id=excluded.user_id,owner_hash=excluded.owner_hash,name=excluded.name,corporation_id=excluded.corporation_id,removed_at=now()`, c.ID, c.User, c.Owner, c.Name, corp)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM identity_sessions WHERE character_id=$1`, c.ID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM identity_characters WHERE character_id=$1 AND user_id=$2`, c.ID, c.User); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO identity_character_events(user_id,character_id,action) VALUES($1,$2,'unlinked')`, c.User, c.ID); err != nil {
		return err
	}
	return CleanupAudit(ctx, tx, "weekly-cleanup", "removed", map[string]any{"character_id": c.ID, "name": c.Name, "account_id": c.User, "corporation_id": corp})
}

func FinishCleanupAccount(ctx context.Context, tx pgx.Tx, user string, removed, skipped int) error {
	var err error
	if removed > 0 {
		// Only removals may change the main or account sessions; uncertain-only
		// accounts must remain untouched. The composite FK is deferred.
		_, err = tx.Exec(ctx, `WITH changed AS (
 UPDATE identity_users u SET main_character_id=(SELECT character_id FROM identity_characters c WHERE c.user_id=u.id AND c.status='active' ORDER BY character_id LIMIT 1)
 WHERE id=$1 AND NOT EXISTS(SELECT 1 FROM identity_characters c WHERE c.user_id=u.id AND c.character_id=u.main_character_id AND c.status='active')
 RETURNING id,main_character_id)
 INSERT INTO identity_character_events(user_id,character_id,action) SELECT id,main_character_id,'main_changed' FROM changed WHERE main_character_id IS NOT NULL`, user)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `DELETE FROM identity_sessions WHERE user_id=$1 AND NOT EXISTS(SELECT 1 FROM identity_characters WHERE user_id=$1 AND status='active')`, user); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `DELETE FROM identity_merge_requests WHERE completed_at IS NULL AND (source_id=$1 OR target_id=$1)`, user); err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `UPDATE identity_corporation_cleanup SET after_user_id=$1,removed_count=removed_count+$2,skipped_count=skipped_count+$3 WHERE id`, user, removed, skipped)
	return err
}

func StartCleanup(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `UPDATE identity_corporation_cleanup SET run_started_at=now(),after_user_id=NULL,removed_count=0,skipped_count=0 WHERE id AND run_started_at IS NULL`)
	return err
}
func FinishCleanup(ctx context.Context, tx pgx.Tx, p CleanupPolicy) error {
	if err := CleanupAudit(ctx, tx, "weekly-cleanup", "completed", map[string]any{"removed": p.Removed, "skipped": p.Skipped, "version": p.Version}); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE identity_corporation_cleanup SET last_run_at=now(),last_removed_count=removed_count,last_skipped_count=skipped_count,next_run_at=now()+interval '7 days',run_started_at=NULL,after_user_id=NULL WHERE id`)
	return err
}

func CleanupPreviousOwner(ctx context.Context, tx pgx.Tx, id int64) (string, []byte, error) {
	var user string
	var owner []byte
	err := tx.QueryRow(ctx, `SELECT user_id::text,owner_hash FROM identity_cleanup_removed WHERE character_id=$1`, id).Scan(&user, &owner)
	return user, owner, err
}

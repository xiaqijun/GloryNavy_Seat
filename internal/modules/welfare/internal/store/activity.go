package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

func NextActivityProject(ctx context.Context, db DB) (int64, error) {
	var id int64
	err := db.QueryRow(ctx, `SELECT nextval('welfare_activity_project_ids')`).Scan(&id)
	return id, err
}

func SaveActivityImage(ctx context.Context, db DB, caseID int64, ordinal int, mime string, content []byte) error {
	_, err := db.Exec(ctx, `INSERT INTO welfare_activity_images(case_id,ordinal,mime,content) VALUES($1,$2,$3,$4)`, caseID, ordinal, mime, content)
	return err
}

func ActivityImage(ctx context.Context, db DB, caseID int64, ordinal int) (string, []byte, error) {
	var mime string
	var content []byte
	err := db.QueryRow(ctx, `SELECT mime,content FROM welfare_activity_images WHERE case_id=$1 AND ordinal=$2`, caseID, ordinal).Scan(&mime, &content)
	return mime, content, err
}

// ActivityClaimCount counts prior claim batches for one character. A multi-role
// application stores one activity_batch_key on every child case; each selected
// character is counted independently.
func ActivityClaimCount(ctx context.Context, db DB, account string, corporation, characterID int64, kind, batchKey string) (int64, error) {
	var count int64
	err := db.QueryRow(ctx, `SELECT count(DISTINCT COALESCE(NULLIF(detail->>'activity_batch_key',''), id::text))
		FROM welfare_cases
		WHERE account_id=$1::uuid AND corporation_id=$2 AND kind=$3
		AND detail->>'character_id' = ($4::bigint)::text
		AND state NOT IN ('cancelled','rejected')
		AND ($5='' OR detail->>'activity_batch_key' IS DISTINCT FROM $5)`, account, corporation, kind, characterID, batchKey).Scan(&count)
	return count, err
}

func ActivityProject(ctx context.Context, db DB, corp int64, kind string) (Policy, error) {
	var p Policy
	err := db.QueryRow(ctx, `SELECT corporation_id,kind,version,config FROM welfare_policies WHERE corporation_id=$1 AND kind=$2`, corp, kind).Scan(&p.CorporationID, &p.Kind, &p.Version, &p.Config)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, pgx.ErrNoRows
	}
	return p, err
}

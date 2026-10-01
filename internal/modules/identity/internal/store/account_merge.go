package store

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"time"
)

type MergeCharacter struct {
	ID     int64  `json:"id,string"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Owner  string `json:"-"`
	User   string `json:"-"`
	Main   bool   `json:"is_main"`
}
type MergeRequest struct {
	ID, Source, Target    string
	Session               []byte
	Identity, PreviewHash string
	Preview               json.RawMessage
	Expires               time.Time
	Completed             *time.Time
}

func MergeSource(ctx context.Context, tx pgx.Tx, id int64) (string, error) {
	var s string
	err := tx.QueryRow(ctx, `SELECT user_id::text FROM identity_characters WHERE character_id=$1 AND status='active'`, id).Scan(&s)
	return s, err
}
func MergeCharacters(ctx context.Context, tx pgx.Tx, source, target string) ([]MergeCharacter, error) {
	rows, err := tx.Query(ctx, `SELECT c.character_id,c.name,c.status,encode(c.owner_hash,'hex'),c.user_id::text,c.character_id=u.main_character_id FROM identity_characters c JOIN identity_users u ON u.id=c.user_id WHERE c.user_id IN($1::uuid,$2::uuid) ORDER BY c.character_id`, source, target)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MergeCharacter{}
	for rows.Next() {
		var c MergeCharacter
		if err = rows.Scan(&c.ID, &c.Name, &c.Status, &c.Owner, &c.User, &c.Main); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func LockMerge(ctx context.Context, tx pgx.Tx, chars []MergeCharacter, source, target string) error {
	for _, c := range chars {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, c.ID); err != nil {
			return err
		}
	}
	rows, err := tx.Query(ctx, `SELECT id FROM identity_users WHERE id IN($1::uuid,$2::uuid) ORDER BY id FOR UPDATE`, source, target)
	if err != nil {
		return err
	}
	rows.Close()
	var retired bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM identity_account_merges WHERE source_id IN($1::uuid,$2::uuid))`, source, target).Scan(&retired); err != nil {
		return err
	}
	if retired {
		return pgx.ErrNoRows
	}
	return nil
}
func NewMergeRequest(ctx context.Context, tx pgx.Tx, source, target string, session []byte, character int64, fingerprint string) (string, error) {
	// Pending requests are session-scoped, short lived, and bounded by SSO start throttling.
	_, err := tx.Exec(ctx, `DELETE FROM identity_merge_requests WHERE completed_at IS NULL AND (expires_at<now() OR (target_id=$1 AND session_hash=$2))`, target, session)
	if err != nil {
		return "", err
	}
	var id string
	err = tx.QueryRow(ctx, `INSERT INTO identity_merge_requests(source_id,target_id,session_hash,proof_character_id,identity_fingerprint) VALUES($1,$2,$3,$4,$5) RETURNING id::text`, source, target, session, character, fingerprint).Scan(&id)
	return id, err
}
func ReadMergeRequest(ctx context.Context, tx pgx.Tx, id string, session []byte, lock bool) (MergeRequest, error) {
	var p MergeRequest
	q := `SELECT id::text,source_id::text,target_id::text,session_hash,identity_fingerprint,preview_fingerprint,coalesce(preview,'{}'),expires_at,completed_at FROM identity_merge_requests WHERE id=$1 AND session_hash=$2 AND (expires_at>now() OR completed_at IS NOT NULL)`
	if lock {
		q += " FOR UPDATE"
	}
	err := tx.QueryRow(ctx, q, id, session).Scan(&p.ID, &p.Source, &p.Target, &p.Session, &p.Identity, &p.PreviewHash, &p.Preview, &p.Expires, &p.Completed)
	return p, err
}
func SetMergePreview(ctx context.Context, tx pgx.Tx, id, hash string, preview []byte) error {
	_, err := tx.Exec(ctx, `UPDATE identity_merge_requests SET preview_fingerprint=$2,preview=$3 WHERE id=$1`, id, hash, preview)
	return err
}
func FinishMerge(ctx context.Context, tx pgx.Tx, p MergeRequest, session []byte) error {
	statements := []string{
		`DELETE FROM identity_sessions WHERE user_id IN($1::uuid,$2::uuid) AND token_hash<>$3`,
		`UPDATE identity_users SET main_character_id=NULL WHERE id=$1 AND $2::uuid IS NOT NULL AND $3::bytea IS NOT NULL`,
		`INSERT INTO identity_character_events(user_id,character_id,action) SELECT $1,character_id,'unlinked' FROM identity_characters WHERE user_id=$1 AND $2::uuid IS NOT NULL AND $3::bytea IS NOT NULL`,
		`UPDATE identity_characters SET user_id=$2 WHERE user_id=$1 AND $3::bytea IS NOT NULL`,
		`INSERT INTO identity_character_events(user_id,character_id,action) SELECT $2,character_id,'linked' FROM identity_character_events WHERE user_id=$1 AND action='unlinked' AND created_at=now() AND $3::bytea IS NOT NULL`,
	}
	for _, q := range statements {
		if _, err := tx.Exec(ctx, q, p.Source, p.Target, session); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO identity_account_merges(source_id,target_id,request_id) VALUES($1,$2,$3)`, p.Source, p.Target, p.ID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE identity_merge_requests SET completed_at=now() WHERE id=$1`, p.ID)
	return err
}
func CancelMerge(ctx context.Context, tx pgx.Tx, id string, session []byte) error {
	_, err := tx.Exec(ctx, `DELETE FROM identity_merge_requests WHERE id=$1 AND session_hash=$2 AND completed_at IS NULL`, id, session)
	return err
}

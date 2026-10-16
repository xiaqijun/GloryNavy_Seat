package store

import (
	"context"
	"time"
)

type CleanupEvidence struct {
	CharacterID     int64
	OwnerHash       []byte
	CorporationID   int64
	CorporationName string
	Reliable        bool
}

func CleanupMembership(ctx context.Context, db DBTX, ids []int64, lock bool, now time.Time) ([]CleanupEvidence, error) {
	if len(ids) == 0 {
		return []CleanupEvidence{}, nil
	}
	if lock {
		rows, err := db.Query(ctx, `SELECT character_id FROM eve_credentials WHERE character_id=ANY($1) ORDER BY character_id FOR UPDATE`, ids)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	rows, err := db.Query(ctx, `SELECT c.character_id,c.owner_hash,coalesce(s.corporation_id,0),coalesce(s.corporation_name,''),
 coalesce(c.state='ready' AND s.owner_hash=c.owner_hash AND s.synced_at<=$2 AND s.valid_until>$2
 AND p.corporation_id=s.corporation_id AND p.checked_at<=$2 AND p.valid_until>$2
 AND 'esi-characters.read_corporation_roles.v1'=ANY(c.scopes)
 AND (SELECT count(*)=2 FROM eve_sync_targets t WHERE t.character_id=c.character_id AND t.resource IN('profile','authorization')
 AND t.generation=c.grant_generation AND t.last_success_at IS NOT NULL AND t.failures=0 AND t.state NOT IN('failed','blocked')),false)
 FROM eve_credentials c LEFT JOIN eve_role_snapshots s USING(character_id) LEFT JOIN eve_character_profiles p USING(character_id)
 WHERE c.character_id=ANY($1) ORDER BY c.character_id`, ids, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CleanupEvidence{}
	for rows.Next() {
		var r CleanupEvidence
		if err = rows.Scan(&r.CharacterID, &r.OwnerHash, &r.CorporationID, &r.CorporationName, &r.Reliable); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

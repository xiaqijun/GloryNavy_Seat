package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

type StructureWatch struct {
	Enabled                 bool
	AppID, Group, Owner     string
	CorporationID, Revision int64
}

func ReadStructureWatch(ctx context.Context, db DBTX) (StructureWatch, error) {
	var v StructureWatch
	err := db.QueryRow(ctx, `SELECT enabled,app_id,group_openid,coalesce(owner_id::text,''),corporation_id,revision FROM community_qq_structure_watch WHERE id`).Scan(&v.Enabled, &v.AppID, &v.Group, &v.Owner, &v.CorporationID, &v.Revision)
	return v, err
}

type StructureObservation struct {
	Observed time.Time
	State    string
	Services json.RawMessage
}

func ReadStructureObservation(ctx context.Context, tx pgx.Tx, revision, corp, id int64) (StructureObservation, error) {
	var o StructureObservation
	err := tx.QueryRow(ctx, `SELECT observed_at,state,services FROM community_qq_structure_observations WHERE watch_revision=$1 AND corporation_id=$2 AND structure_id=$3 FOR UPDATE`, revision, corp, id).Scan(&o.Observed, &o.State, &o.Services)
	return o, err
}

func SaveStructureObservation(ctx context.Context, tx pgx.Tx, revision, corp, id int64, o StructureObservation) error {
	_, err := tx.Exec(ctx, `INSERT INTO community_qq_structure_observations(watch_revision,corporation_id,structure_id,observed_at,state,services) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(watch_revision,corporation_id,structure_id) DO UPDATE SET observed_at=excluded.observed_at,state=excluded.state,services=excluded.services WHERE community_qq_structure_observations.observed_at<excluded.observed_at`, revision, corp, id, o.Observed, o.State, o.Services)
	return err
}

func InsertStructureNotice(ctx context.Context, tx pgx.Tx, watch StructureWatch, id int64, reference, kind, body string) error {
	_, err := tx.Exec(ctx, `INSERT INTO community_qq_structure_notices(watch_revision,reference,kind,corporation_id,structure_id,body) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(watch_revision,reference) DO UPDATE SET status='pending',body=excluded.body WHERE community_qq_structure_notices.status='obsolete'`, watch.Revision, reference, kind, watch.CorporationID, id, body)
	return err
}

type StructureNotice struct {
	ID, StructureID       int64
	Body, Reference, Kind string
}

func PendingStructureNotices(ctx context.Context, tx pgx.Tx, revision int64) ([]StructureNotice, error) {
	rows, err := tx.Query(ctx, `SELECT id,structure_id,body,reference,kind FROM community_qq_structure_notices WHERE watch_revision=$1 AND status='pending' ORDER BY id LIMIT 5 FOR UPDATE`, revision)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []StructureNotice
	for rows.Next() {
		var v StructureNotice
		if err = rows.Scan(&v.ID, &v.StructureID, &v.Body, &v.Reference, &v.Kind); err != nil {
			return nil, err
		}
		items = append(items, v)
	}
	return items, rows.Err()
}

func DiscardStructureNotices(ctx context.Context, tx pgx.Tx, ids []int64) error {
	_, err := tx.Exec(ctx, `UPDATE community_qq_structure_notices SET status='obsolete' WHERE id=ANY($1::bigint[]) AND status='pending'`, ids)
	return err
}

// Reset is safe only before the external send request has started.
func ResetUnsentStructureNotices(ctx context.Context, tx pgx.Tx, ids []int64) error {
	_, err := tx.Exec(ctx, `UPDATE community_qq_structure_notices SET status='pending',attempted_at=NULL WHERE id=ANY($1::bigint[]) AND status='sending'`, ids)
	return err
}

func SetStructureNoticeState(ctx context.Context, db DBTX, ids []int64, state string, code int) error {
	_, err := db.Exec(ctx, `UPDATE community_qq_structure_notices SET status=$2,error_code=NULLIF($3,0),attempted_at=now() WHERE id=ANY($1::bigint[]) AND status=CASE WHEN $2='sending' THEN 'pending' ELSE 'sending' END`, ids, state, code)
	return err
}

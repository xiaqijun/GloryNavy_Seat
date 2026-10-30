package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type FleetMOTDUpdate struct {
	ID, EventID, SourceID, FleetID, Generation, NoticeRevision int64
	ActorID                                                    pgtype.UUID
	OwnerHash                                                  []byte
	PAPPoints                                                  int32
	State, Reason                                              string
	UpdatedAt                                                  time.Time
}

func InsertFleetMOTD(ctx context.Context, tx pgx.Tx, eventID int64, key, actor pgtype.UUID, sourceID, fleetID, generation int64, owner []byte, points int32) error {
	_, err := tx.Exec(ctx, `INSERT INTO attendance_fleet_motd_updates(event_id,request_key,actor_id,source_id,fleet_id,generation,owner_hash,pap_points) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, eventID, key, actor, sourceID, fleetID, generation, owner, points)
	return err
}

func InsertFleetNoticeMOTD(ctx context.Context, tx pgx.Tx, eventID int64, key, actor pgtype.UUID, sourceID, fleetID, generation int64, owner []byte, revision, version int64) error {
	_, err := tx.Exec(ctx, `INSERT INTO attendance_fleet_motd_updates(event_id,request_key,actor_id,source_id,fleet_id,generation,owner_hash,pap_points,notice_revision,event_version) VALUES($1,$2,$3,$4,$5,$6,$7,0,$8,$9)`, eventID, key, actor, sourceID, fleetID, generation, owner, revision, version)
	return err
}

func FleetNoticeMOTDReplay(ctx context.Context, db DBTX, eventID int64, key, actor pgtype.UUID, source, revision, version int64) (bool, error) {
	var same bool
	err := db.QueryRow(ctx, `SELECT actor_id=$3 AND source_id=$4 AND notice_revision=$5 AND event_version=$6 AND pap_points=0 FROM attendance_fleet_motd_updates WHERE event_id=$1 AND request_key=$2`, eventID, key, actor, source, revision, version).Scan(&same)
	return same, err
}

func FleetMOTDOutstanding(ctx context.Context, db DBTX, eventID int64) (bool, error) {
	var outstanding bool
	err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM attendance_fleet_motd_updates WHERE event_id=$1 AND state IN ('pending','preparing','sending'))`, eventID).Scan(&outstanding)
	return outstanding, err
}

func ClaimFleetMOTD(ctx context.Context, db DBTX) (FleetMOTDUpdate, error) {
	var u FleetMOTDUpdate
	err := db.QueryRow(ctx, `UPDATE attendance_fleet_motd_updates SET state='preparing',updated_at=now() WHERE id=(SELECT id FROM attendance_fleet_motd_updates WHERE state='pending' ORDER BY id FOR UPDATE SKIP LOCKED LIMIT 1) RETURNING id,event_id,source_id,fleet_id,generation,actor_id,owner_hash,pap_points,notice_revision,state,reason,updated_at`).Scan(&u.ID, &u.EventID, &u.SourceID, &u.FleetID, &u.Generation, &u.ActorID, &u.OwnerHash, &u.PAPPoints, &u.NoticeRevision, &u.State, &u.Reason, &u.UpdatedAt)
	return u, err
}

func TransitionFleetMOTD(ctx context.Context, db DBTX, id int64, from string, updatedAt time.Time, to, reason string) (time.Time, error) {
	var next time.Time
	err := db.QueryRow(ctx, `UPDATE attendance_fleet_motd_updates SET state=$4,reason=$5,updated_at=clock_timestamp() WHERE id=$1 AND state=$2 AND updated_at=$3 RETURNING updated_at`, id, from, updatedAt, to, reason).Scan(&next)
	return next, err
}

func MaintainFleetMOTD(ctx context.Context, db DBTX) error {
	_, err := db.Exec(ctx, `UPDATE attendance_fleet_motd_updates SET state=CASE state WHEN 'preparing' THEN 'pending' ELSE 'unknown' END,reason=CASE state WHEN 'preparing' THEN '' ELSE 'interrupted' END,updated_at=now() WHERE state IN ('preparing','sending') AND updated_at<now()-interval '3 minutes'`)
	return err
}

func LatestFleetMOTD(ctx context.Context, db DBTX, eventID int64) (FleetMOTDUpdate, error) {
	var u FleetMOTDUpdate
	err := db.QueryRow(ctx, `SELECT id,event_id,source_id,fleet_id,generation,actor_id,owner_hash,pap_points,notice_revision,state,reason,updated_at FROM attendance_fleet_motd_updates WHERE event_id=$1 ORDER BY id DESC LIMIT 1`, eventID).Scan(&u.ID, &u.EventID, &u.SourceID, &u.FleetID, &u.Generation, &u.ActorID, &u.OwnerHash, &u.PAPPoints, &u.NoticeRevision, &u.State, &u.Reason, &u.UpdatedAt)
	return u, err
}

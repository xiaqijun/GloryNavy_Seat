package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type FleetDelivery struct {
	ID, DispatchID, EventID, CorporationID               int64
	ActorID, AccountID                                   pgtype.UUID
	Channel, TargetVersion, Subject, Body, State, Reason string
	UpdatedAt                                            time.Time
}
type FleetDeliveryCount struct {
	Channel, State, Reason string
	Count                  int64
}

func InsertFleetDispatch(ctx context.Context, tx pgx.Tx, eventID, corp, revision int64, actor, key pgtype.UUID, subject, body string) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `INSERT INTO attendance_fleet_dispatches(event_id,corporation_id,notice_revision,actor_id,request_key,subject,body) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`, eventID, corp, revision, actor, key, subject, body).Scan(&id)
	return id, err
}
func FleetDispatchByKey(ctx context.Context, db DBTX, eventID int64, key pgtype.UUID) (int64, error) {
	var id int64
	err := db.QueryRow(ctx, `SELECT id FROM attendance_fleet_dispatches WHERE event_id=$1 AND request_key=$2`, eventID, key).Scan(&id)
	return id, err
}
func HasOutstandingFleetDeliveries(ctx context.Context, db DBTX, eventID, revision int64, channels []string) (bool, error) {
	var exists bool
	err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM attendance_fleet_dispatches p JOIN attendance_fleet_deliveries d ON d.dispatch_id=p.id WHERE p.event_id=$1 AND p.notice_revision=$2 AND d.channel=ANY($3::text[]) AND d.state IN ('pending','preparing','sending','unknown'))`, eventID, revision, channels).Scan(&exists)
	return exists, err
}
func InsertFleetDelivery(ctx context.Context, tx pgx.Tx, dispatchID int64, channel, targetVersion string, account pgtype.UUID) error {
	_, err := tx.Exec(ctx, `INSERT INTO attendance_fleet_deliveries(dispatch_id,channel,target_version,account_id) VALUES($1,$2,$3,$4)`, dispatchID, channel, targetVersion, account)
	return err
}
func ClaimFleetDelivery(ctx context.Context, db DBTX) (FleetDelivery, error) {
	var d FleetDelivery
	err := db.QueryRow(ctx, `WITH claimed AS (UPDATE attendance_fleet_deliveries SET state='preparing',updated_at=now() WHERE id=(SELECT id FROM attendance_fleet_deliveries WHERE state='pending' ORDER BY id FOR UPDATE SKIP LOCKED LIMIT 1) RETURNING *) SELECT c.id,c.dispatch_id,p.event_id,p.corporation_id,p.actor_id,c.account_id,c.channel,c.target_version,p.subject,p.body,c.state,c.reason,c.updated_at FROM claimed c JOIN attendance_fleet_dispatches p ON p.id=c.dispatch_id`).Scan(&d.ID, &d.DispatchID, &d.EventID, &d.CorporationID, &d.ActorID, &d.AccountID, &d.Channel, &d.TargetVersion, &d.Subject, &d.Body, &d.State, &d.Reason, &d.UpdatedAt)
	return d, err
}
func TransitionFleetDelivery(ctx context.Context, db DBTX, id int64, from string, updated time.Time, to, reason string) (time.Time, error) {
	var next time.Time
	err := db.QueryRow(ctx, `UPDATE attendance_fleet_deliveries SET state=$4,reason=$5,updated_at=clock_timestamp() WHERE id=$1 AND state=$2 AND updated_at=$3 RETURNING updated_at`, id, from, updated, to, reason).Scan(&next)
	return next, err
}
func MaintainFleetDeliveries(ctx context.Context, db DBTX) error {
	_, err := db.Exec(ctx, `UPDATE attendance_fleet_deliveries SET state=CASE state WHEN 'preparing' THEN 'pending' ELSE 'unknown' END,reason=CASE state WHEN 'preparing' THEN '' ELSE 'interrupted' END,updated_at=now() WHERE state IN ('preparing','sending') AND updated_at<now()-interval '3 minutes'`)
	return err
}
func LatestFleetDispatch(ctx context.Context, db DBTX, eventID int64) (int64, error) {
	var id int64
	err := db.QueryRow(ctx, `SELECT id FROM attendance_fleet_dispatches WHERE event_id=$1 ORDER BY id DESC LIMIT 1`, eventID).Scan(&id)
	return id, err
}
func FleetDeliveryCounts(ctx context.Context, db DBTX, dispatchID int64) ([]FleetDeliveryCount, error) {
	rows, err := db.Query(ctx, `SELECT channel,state,reason,count(*) FROM attendance_fleet_deliveries WHERE dispatch_id=$1 GROUP BY channel,state,reason ORDER BY channel,state,reason`, dispatchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FleetDeliveryCount{}
	for rows.Next() {
		var c FleetDeliveryCount
		if err = rows.Scan(&c.Channel, &c.State, &c.Reason, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

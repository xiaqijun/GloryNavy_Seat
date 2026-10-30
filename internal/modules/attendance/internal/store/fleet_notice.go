package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type FleetNotice struct {
	EventID                                                 int64
	SolarSystem, Doctrine, MeetingPoint, Commander, Details string
	Revision                                                int64
	UpdatedBy                                               pgtype.UUID
	UpdatedAt                                               time.Time
}

func FleetNoticeByEvent(ctx context.Context, db DBTX, eventID int64) (FleetNotice, error) {
	var n FleetNotice
	err := db.QueryRow(ctx, `SELECT event_id,solar_system,doctrine,meeting_point,commander,details,revision,updated_by,updated_at FROM attendance_fleet_notices WHERE event_id=$1`, eventID).Scan(&n.EventID, &n.SolarSystem, &n.Doctrine, &n.MeetingPoint, &n.Commander, &n.Details, &n.Revision, &n.UpdatedBy, &n.UpdatedAt)
	return n, err
}

func SaveFleetNotice(ctx context.Context, tx pgx.Tx, n FleetNotice) (FleetNotice, error) {
	var out FleetNotice
	query := `UPDATE attendance_fleet_notices SET solar_system=$2,doctrine=$3,meeting_point=$4,commander=$5,details=$6,updated_by=$7,revision=revision+1,updated_at=now() WHERE event_id=$1 AND revision=$8 RETURNING event_id,solar_system,doctrine,meeting_point,commander,details,revision,updated_by,updated_at`
	if n.Revision == 0 {
		query = `INSERT INTO attendance_fleet_notices(event_id,solar_system,doctrine,meeting_point,commander,details,updated_by) SELECT $1,$2,$3,$4,$5,$6,$7 WHERE $8=0 ON CONFLICT(event_id) DO NOTHING RETURNING event_id,solar_system,doctrine,meeting_point,commander,details,revision,updated_by,updated_at`
	}
	err := tx.QueryRow(ctx, query, n.EventID, n.SolarSystem, n.Doctrine, n.MeetingPoint, n.Commander, n.Details, n.UpdatedBy, n.Revision).Scan(&out.EventID, &out.SolarSystem, &out.Doctrine, &out.MeetingPoint, &out.Commander, &out.Details, &out.Revision, &out.UpdatedBy, &out.UpdatedAt)
	return out, err
}

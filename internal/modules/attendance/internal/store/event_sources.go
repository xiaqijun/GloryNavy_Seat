package store

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type EventSource struct {
	SourceID         int64
	CreationSourceID pgtype.Int8
	Revision         int64
}

func GetEventSource(ctx context.Context, db DBTX, eventID int64, account pgtype.UUID) (EventSource, error) {
	var result EventSource
	err := db.QueryRow(ctx, `SELECT source_id,creation_source_id,revision FROM attendance_event_sources WHERE event_id=$1 AND account_id=$2`, eventID, account).Scan(&result.SourceID, &result.CreationSourceID, &result.Revision)
	return result, err
}

func SaveCreatedEventSource(ctx context.Context, tx pgx.Tx, eventID int64, account pgtype.UUID, source int64) error {
	_, err := tx.Exec(ctx, `INSERT INTO attendance_event_sources(event_id,account_id,source_id,creation_source_id) VALUES($1,$2,$3,$3) ON CONFLICT(event_id,account_id) DO NOTHING`, eventID, account, source)
	if err != nil {
		return err
	}
	var original pgtype.Int8
	if err = tx.QueryRow(ctx, `SELECT creation_source_id FROM attendance_event_sources WHERE event_id=$1 AND account_id=$2 FOR UPDATE`, eventID, account).Scan(&original); err != nil {
		return err
	}
	if !original.Valid || original.Int64 != source {
		return pgx.ErrNoRows
	}
	return nil
}

func SaveEventSource(ctx context.Context, tx pgx.Tx, eventID int64, account pgtype.UUID, source, revision int64) (int64, error) {
	var next int64
	if revision == 0 {
		err := tx.QueryRow(ctx, `INSERT INTO attendance_event_sources(event_id,account_id,source_id) VALUES($1,$2,$3) ON CONFLICT(event_id,account_id) DO NOTHING RETURNING revision`, eventID, account, source).Scan(&next)
		return next, err
	}
	err := tx.QueryRow(ctx, `UPDATE attendance_event_sources SET source_id=$3,revision=revision+1,updated_at=now() WHERE event_id=$1 AND account_id=$2 AND revision=$4 RETURNING revision`, eventID, account, source, revision).Scan(&next)
	return next, err
}

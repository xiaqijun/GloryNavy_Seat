package store

import (
	"context"
	"time"
)

type StructureDelivery struct {
	ID        string    `json:"id"`
	Body      string    `json:"body"`
	Kind      string    `json:"kind"`
	Status    string    `json:"status"`
	ErrorCode int       `json:"error_code"`
	CreatedAt time.Time `json:"created_at"`
}

func StructureDeliveries(ctx context.Context, db DBTX, corporationID, revision int64) ([]StructureDelivery, error) {
	rows, err := db.Query(ctx, `SELECT id::text,body,kind,CASE WHEN status='pending' AND watch_revision<>$2 THEN 'obsolete' WHEN status='sending' AND attempted_at<now()-interval '2 minutes' THEN 'unknown' ELSE status END,coalesce(error_code,0),created_at FROM community_qq_structure_notices WHERE corporation_id=$1 ORDER BY id DESC LIMIT 50`, corporationID, revision)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []StructureDelivery{}
	for rows.Next() {
		var item StructureDelivery
		if err = rows.Scan(&item.ID, &item.Body, &item.Kind, &item.Status, &item.ErrorCode, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

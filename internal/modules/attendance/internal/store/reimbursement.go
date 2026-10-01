package store

import "context"

// Read only confirmed losses for an attendance record owned by this account.
func ConfirmedLossEvents(ctx context.Context, db DBTX, account string, corp, character int64, ids []int64, lock bool) (map[int64]int64, error) {
	query := `SELECT l.killmail_id,e.id FROM attendance_losses l
	JOIN attendance_events e ON e.id=l.event_id
	JOIN attendance_entries a ON a.event_id=l.event_id AND a.character_id=l.character_id
	WHERE e.corporation_id=$1 AND a.account_id=$2::uuid AND a.character_id=$3
	AND a.present AND l.state='confirmed' AND l.killmail_id=ANY($4::bigint[]) ORDER BY e.id`
	if lock {
		query += ` FOR SHARE OF e,a,l`
	}
	rows, err := db.Query(ctx, query, corp, account, character, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]int64{}
	for rows.Next() {
		var km, event int64
		if err = rows.Scan(&km, &event); err != nil {
			return nil, err
		}
		out[km] = event
	}
	return out, rows.Err()
}

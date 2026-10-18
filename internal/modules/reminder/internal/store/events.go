package store

import "context"

// Rotate categories by their last attempt so a busy category cannot starve another.
func EventOrder(ctx context.Context, db DBTX, account string, events []string) ([]string, error) {
	rows, err := db.Query(ctx, `SELECT e FROM unnest($2::text[]) e LEFT JOIN LATERAL (SELECT max(created_at) t FROM reminder_deliveries WHERE account_id=$1 AND event=e) d ON true ORDER BY d.t NULLS FIRST,e`, account, events)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var e string
		if err = rows.Scan(&e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

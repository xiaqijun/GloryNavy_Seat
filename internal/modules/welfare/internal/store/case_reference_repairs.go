package store

import "context"

// CaseReferenceAliases binds one historical title to the current persisted
// reference of this exact case. Stale readers cannot borrow another alias.
func CaseReferenceAliases(ctx context.Context, db DB, id int64, current string) ([]string, error) {
	rows, err := db.Query(ctx, `SELECT previous_reference FROM welfare_case_reference_repairs WHERE case_id=$1 AND replacement_reference=$2`, id, current)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var ref string
		if err := rows.Scan(&ref); err != nil {
			return nil, err
		}
		out = append(out, ref)
	}
	return out, rows.Err()
}

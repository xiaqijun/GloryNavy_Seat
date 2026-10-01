package store

import (
	"context"
	"strconv"
)

type LossStatus struct{ KillmailID, Kind, State string }

// Only return state metadata for reports whose raw character scope was already checked.
func LossStatuses(ctx context.Context, db DB, actor string, admin bool, corp, character int64, ids []string) ([]LossStatus, error) {
	rows, err := db.Query(ctx, `SELECT DISTINCT ON (detail->>'killmail_id') detail->>'killmail_id',kind,state
	FROM welfare_cases WHERE corporation_id=$1 AND detail->>'character_id'=$2::text
	AND detail->>'killmail_id'=ANY($3::text[]) AND (account_id=$4::uuid OR $5)
	AND kind IN ('srp','alliance','solo') AND state IN ('submitted','information','external','approved','executing','cancel_requested','completed')
	ORDER BY detail->>'killmail_id', (state='completed') DESC, id DESC`, corp, strconv.FormatInt(character, 10), ids, actor, admin)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LossStatus{}
	for rows.Next() {
		var r LossStatus
		if err = rows.Scan(&r.KillmailID, &r.Kind, &r.State); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

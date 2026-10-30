package store

import (
	"context"
)

// LossQuotaSpan allocates Amount per calendar day in [Start, End).
// Long advances are represented in constant space, without generating dates.
type LossQuotaSpan struct {
	Kind   string
	Start  int64
	End    int64
	Amount int64
}

func LossQuotaDays(ctx context.Context, db DB, account string, corp, day int64) ([]LossQuotaSpan, error) {
	rows, err := db.Query(ctx, `
 SELECT c.kind, q.start_day, q.end_day, q.amount_minor
 FROM welfare_cases c JOIN welfare_loss_quota_days q ON q.case_id=c.id
 WHERE c.account_id=$1 AND c.corporation_id=$2 AND c.kind IN ('srp','solo')
 AND c.state IN ('approved','executing','cancel_requested','completed') AND q.end_day>$3
 UNION ALL
 SELECT c.kind, $3::bigint, $3::bigint+1, c.award_minor
 FROM welfare_cases c
 WHERE c.account_id=$1 AND c.corporation_id=$2 AND c.kind IN ('srp','solo')
 AND c.state IN ('approved','executing','cancel_requested','completed') AND c.award_minor>0
 AND NOT EXISTS (SELECT 1 FROM welfare_loss_quota_days q WHERE q.case_id=c.id)
 AND (SELECT max(a.created_at) FROM welfare_audit a WHERE a.case_id=c.id AND a.action='approve')
     >= (date '1970-01-01'+$3::integer)::timestamp AT TIME ZONE 'Asia/Shanghai'
 AND (SELECT max(a.created_at) FROM welfare_audit a WHERE a.case_id=c.id AND a.action='approve')
     < (date '1970-01-01'+$3::integer+1)::timestamp AT TIME ZONE 'Asia/Shanghai'`, account, corp, day)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LossQuotaSpan{}
	for rows.Next() {
		var v LossQuotaSpan
		if err = rows.Scan(&v.Kind, &v.Start, &v.End, &v.Amount); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// Caller holds the welfare publication lock and commits this with approval.
func SaveLossQuotaDays(ctx context.Context, db DB, id int64, spans []LossQuotaSpan) error {
	for i, v := range spans {
		if _, err := db.Exec(ctx, `INSERT INTO welfare_loss_quota_days(case_id,ordinal,start_day,end_day,amount_minor) VALUES($1,$2,$3,$4,$5)`, id, i+1, v.Start, v.End, v.Amount); err != nil {
			return err
		}
	}
	return nil
}

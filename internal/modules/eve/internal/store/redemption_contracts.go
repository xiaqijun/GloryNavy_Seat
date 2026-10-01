package store

import (
	"context"
	"time"
)

func ClaimDelivery(ctx context.Context, db DBTX, contract int64, module string, reference int64) (bool, error) {
	_, err := db.Exec(ctx, `INSERT INTO eve_delivery_claims(contract_id,module,reference_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, contract, module, reference)
	if err != nil {
		return false, err
	}
	var ok bool
	err = db.QueryRow(ctx, `SELECT module=$2 AND reference_id=$3 FROM eve_delivery_claims WHERE contract_id=$1`, contract, module, reference).Scan(&ok)
	return ok, err
}
func RedemptionContractIDs(ctx context.Context, db DBTX, recipient int64, reference string, since time.Time) ([]int64, error) {
	rows, err := db.Query(ctx, `SELECT contract_id FROM eve_contracts WHERE owner_kind='character' AND owner_id=$1 AND in_scope AND btrim(coalesce(payload->>'title',''))=$2 AND (payload->>'date_issued')::timestamptz >= $3 ORDER BY contract_id LIMIT 11`, recipient, reference, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

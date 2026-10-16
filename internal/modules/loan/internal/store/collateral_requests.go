package store

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

// Ownership follows CaseID, so the existing loan account-merge participant moves
// requests together with their case and preserves the case's original account.
type CollateralRequest struct {
	ID                   int64           `json:"id,string"`
	PublicID             string          `json:"public_id"`
	CaseID               int64           `json:"case_id,string"`
	SourceCharacterID    int64           `json:"source_character_id,string"`
	RecipientCharacterID int64           `json:"recipient_character_id,string"`
	Selections           json.RawMessage `json:"selections"`
	Items                json.RawMessage `json:"items"`
	ObservedAt           time.Time       `json:"observed_at"`
	State                string          `json:"state"`
	CollateralID         *int64          `json:"collateral_id,string,omitempty"`
	Version              int64           `json:"version"`
	CreatedAt            time.Time       `json:"created_at"`
}

const collateralRequestColumns = `r.id,r.public_id,r.case_id,r.source_character_id,r.recipient_character_id,r.selections,r.items,r.observed_at,
CASE WHEN r.state='awaiting_delivery' AND c.state NOT IN ('submitted','awaiting_acceptance','approved') THEN 'cancelled' ELSE r.state END,
r.collateral_id,r.version,r.created_at`

func scanCollateralRequest(row pgx.Row) (CollateralRequest, error) {
	var v CollateralRequest
	err := row.Scan(&v.ID, &v.PublicID, &v.CaseID, &v.SourceCharacterID, &v.RecipientCharacterID, &v.Selections, &v.Items, &v.ObservedAt, &v.State, &v.CollateralID, &v.Version, &v.CreatedAt)
	return v, err
}

func GetCollateralRequest(ctx context.Context, db DBTX, id int64) (CollateralRequest, error) {
	return scanCollateralRequest(db.QueryRow(ctx, `SELECT `+collateralRequestColumns+` FROM loan_collateral_requests r JOIN loan_cases c ON c.id=r.case_id WHERE r.id=$1`, id))
}

func ListCollateralRequests(ctx context.Context, db DBTX, caseID int64) ([]CollateralRequest, error) {
	rows, err := db.Query(ctx, `SELECT `+collateralRequestColumns+` FROM loan_collateral_requests r JOIN loan_cases c ON c.id=r.case_id WHERE r.case_id=$1 ORDER BY r.id DESC`, caseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CollateralRequest{}
	for rows.Next() {
		v, err := scanCollateralRequest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func InsertCollateralRequest(ctx context.Context, db DBTX, v CollateralRequest) (CollateralRequest, error) {
	var id int64
	err := db.QueryRow(ctx, `INSERT INTO loan_collateral_requests(public_id,case_id,source_character_id,recipient_character_id,selections,items,observed_at) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`, v.PublicID, v.CaseID, v.SourceCharacterID, v.RecipientCharacterID, v.Selections, v.Items, v.ObservedAt).Scan(&id)
	if err != nil {
		return CollateralRequest{}, err
	}
	return GetCollateralRequest(ctx, db, id)
}

func LockCollateralAssets(ctx context.Context, tx pgx.Tx, character int64) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "loan-collateral-assets:"+strconv.FormatInt(character, 10))
	return err
}

func ReservedCollateralAssets(ctx context.Context, db DBTX, character int64) (map[int64]int64, error) {
	rows, err := db.Query(ctx, `SELECT (v->>'item_id')::bigint,sum((v->>'quantity')::bigint)::bigint FROM loan_collateral_requests r JOIN loan_cases c ON c.id=r.case_id CROSS JOIN LATERAL jsonb_array_elements(r.selections) v WHERE r.source_character_id=$1 AND r.state='awaiting_delivery' AND c.state IN ('submitted','awaiting_acceptance','approved') GROUP BY v->>'item_id'`, character)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]int64{}
	for rows.Next() {
		var id, q int64
		if err := rows.Scan(&id, &q); err != nil {
			return nil, err
		}
		out[id] = q
	}
	return out, rows.Err()
}

func FinishCollateralRequest(ctx context.Context, db DBTX, id, version, collateralID int64) error {
	result, err := db.Exec(ctx, `UPDATE loan_collateral_requests SET state='delivered',collateral_id=$3,version=version+1,updated_at=now() WHERE id=$1 AND version=$2 AND state='awaiting_delivery'`, id, version, collateralID)
	if err == nil && result.RowsAffected() != 1 {
		return pgx.ErrNoRows
	}
	return err
}

func CancelCollateralRequest(ctx context.Context, db DBTX, id, version int64) error {
	result, err := db.Exec(ctx, `UPDATE loan_collateral_requests SET state='cancelled',version=version+1,updated_at=now() WHERE id=$1 AND version=$2 AND state='awaiting_delivery'`, id, version)
	if err == nil && result.RowsAffected() != 1 {
		return pgx.ErrNoRows
	}
	return err
}

func CollateralSourceCharacter(ctx context.Context, db DBTX, collateralID, fallback int64) (int64, error) {
	var character int64
	err := db.QueryRow(ctx, `SELECT source_character_id FROM loan_collateral_requests WHERE collateral_id=$1 AND state='delivered'`, collateralID).Scan(&character)
	if err == pgx.ErrNoRows {
		return fallback, nil
	}
	return character, err
}

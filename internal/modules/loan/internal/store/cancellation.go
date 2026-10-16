package store

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// LockCase serializes cancellation and security creation after account guards.
func LockCase(ctx context.Context, tx pgx.Tx, id int64) (Case, error) {
	var lockedID int64
	if err := tx.QueryRow(ctx, `SELECT id FROM loan_cases WHERE id=$1 FOR UPDATE`, id).Scan(&lockedID); err != nil {
		return Case{}, err
	}
	return GetCase(ctx, tx, lockedID)
}

func CancelApplication(ctx context.Context, tx pgx.Tx, id, version int64, actor string) (Case, error) {
	result, err := tx.Exec(ctx, `UPDATE loan_cases SET state='cancelled',version=version+1,updated_at=now() WHERE id=$1 AND version=$2 AND borrower_account_id=$3::uuid AND state='submitted'`, id, version, actor)
	if err != nil {
		return Case{}, err
	}
	if result.RowsAffected() != 1 {
		return Case{}, pgx.ErrNoRows
	}
	return GetCase(ctx, tx, id)
}

func ReleaseApplicationSecurity(ctx context.Context, tx pgx.Tx, caseID int64) error {
	if _, err := tx.Exec(ctx, `UPDATE loan_collateral_requests SET state='cancelled',version=version+1,updated_at=now() WHERE case_id=$1 AND state='awaiting_delivery'`, caseID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE loan_guarantees SET state='released',version=version+1,decided_at=now() WHERE case_id=$1 AND state IN ('invited','accepted','active')`, caseID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE loan_collateral SET state='released',version=version+1,decided_at=now() WHERE case_id=$1 AND state IN ('proposed','approved','held')`, caseID)
	return err
}

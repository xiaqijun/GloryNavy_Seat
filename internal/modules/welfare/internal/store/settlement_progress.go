package store

import "context"

// SettlementReferenceAliases keeps late-synchronized contracts addressable after
// an explicit, audited legacy reference repair. Repairs are one-time operations.
func SettlementReferenceAliases(ctx context.Context, db DB, batchID int64, current string) ([]string, error) {
	rows, err := db.Query(ctx, `SELECT previous_reference FROM welfare_contract_settlement_reference_repairs WHERE batch_id=$1 AND replacement_reference=$2`, batchID, current)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var ref string
		if err := rows.Scan(&ref); err != nil {
			return nil, err
		}
		out = append(out, ref)
	}
	return out, rows.Err()
}

// RecordSettlementProgress atomically saves the observed contract and the next
// check. A visible contract ID is evidence of observation, never proof of payment.
func RecordSettlementProgress(ctx context.Context, db DB, id int64, contractID, recipientID *int64, detail string) error {
	_, err := db.Exec(ctx, `WITH batch AS (
 UPDATE welfare_contract_settlement_batches SET state='pending',next_run_at=now()+interval '5 minutes',
 contract_id=$2,contract_recipient_id=$3,version=version+1
 WHERE id=$1 AND state IN ('pending','processing','partial') RETURNING id
 ) UPDATE welfare_contract_settlement_items SET state='pending',last_error=$4,updated_at=now()
 WHERE batch_id IN (SELECT id FROM batch) AND state IN ('pending','processing')`, id, contractID, recipientID, detail)
	return err
}

const (
	SettlementWaitingContractError = "等待合并合同同步"
	SettlementDeletedError         = "合同已删除"
	SettlementCancelledError       = "合同已取消"
	SettlementRejectedError        = "合同已拒绝"
	SettlementFailedError          = "合同已失效"
	SettlementMismatchError        = "合同金额或物品尚未匹配"
	SettlementMultipleError        = "存在多个同编号合同，请保留一份"
)

var settlementProgressCodes = map[string]string{
	SettlementAwaitingAcceptanceError: SettlementAwaitingAcceptance,
	SettlementWaitingContractError:    "waiting_contract",
	SettlementDeletedError:            "contract_deleted",
	SettlementCancelledError:          "contract_cancelled",
	SettlementRejectedError:           "contract_rejected",
	SettlementFailedError:             "contract_failed",
	SettlementMismatchError:           "contract_mismatch",
	SettlementMultipleError:           "multiple_contracts",
}

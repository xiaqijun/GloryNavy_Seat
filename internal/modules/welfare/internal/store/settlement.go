package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type SettlementBatch struct {
	ID                  int64            `json:"id,string"`
	RequestKey          string           `json:"request_key"`
	ActorID             string           `json:"actor_id"`
	Note                string           `json:"note"`
	State               string           `json:"state"`
	TotalCount          int              `json:"total_count"`
	CompletedCount      int              `json:"completed_count"`
	FailedCount         int              `json:"failed_count"`
	CreatedAt           time.Time        `json:"created_at"`
	StartedAt           *time.Time       `json:"started_at,omitempty"`
	FinishedAt          *time.Time       `json:"finished_at,omitempty"`
	NextRunAt           time.Time        `json:"next_run_at"`
	Version             int64            `json:"version,string"`
	SettlementReference string           `json:"settlement_reference,omitempty"`
	AccountID           string           `json:"account_id,omitempty"`
	ISKMinor            int64            `json:"isk_minor"`
	Items               json.RawMessage  `json:"items"`
	RecipientIDs        json.RawMessage  `json:"recipient_ids"`
	ContractID          *int64           `json:"contract_id,omitempty"`
	ContractRecipientID *int64           `json:"contract_recipient_id,omitempty"`
	Entries             []SettlementItem `json:"entries,omitempty"`
}

type SettlementItem struct {
	ID        int64     `json:"id,string"`
	BatchID   int64     `json:"batch_id,string"`
	Source    string    `json:"source"`
	SourceID  int64     `json:"source_id,string"`
	State     string    `json:"state"`
	Attempts  int       `json:"attempts"`
	LastError string    `json:"last_error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type SettlementSelection struct {
	Source   string
	SourceID int64
}

func scanSettlementBatch(row pgx.Row) (SettlementBatch, error) {
	var b SettlementBatch
	err := row.Scan(&b.ID, &b.RequestKey, &b.ActorID, &b.Note, &b.State, &b.TotalCount, &b.CompletedCount, &b.FailedCount, &b.CreatedAt, &b.StartedAt, &b.FinishedAt, &b.NextRunAt, &b.Version, &b.SettlementReference, &b.AccountID, &b.ISKMinor, &b.Items, &b.RecipientIDs, &b.ContractID, &b.ContractRecipientID)
	return b, err
}

const settlementBatchColumns = `id,request_key::text,actor_id::text,note,state,total_count,completed_count,failed_count,created_at,started_at,finished_at,next_run_at,version,coalesce(settlement_reference,''),coalesce(account_id::text,''),isk_minor,items,recipient_ids,contract_id,contract_recipient_id`

func SettlementBatchByRequestKey(ctx context.Context, db DB, actor, key string) (SettlementBatch, error) {
	return scanSettlementBatch(db.QueryRow(ctx, "SELECT "+settlementBatchColumns+" FROM welfare_contract_settlement_batches WHERE actor_id=$1 AND request_key=$2", actor, key))
}

func SettlementBatchByID(ctx context.Context, db DB, id int64) (SettlementBatch, error) {
	return scanSettlementBatch(db.QueryRow(ctx, "SELECT "+settlementBatchColumns+" FROM welfare_contract_settlement_batches WHERE id=$1", id))
}

func CreateSettlementBatch(ctx context.Context, db DB, actor, key, note, account, reference string, isk int64, items, recipients json.RawMessage, selections []SettlementSelection) (SettlementBatch, error) {
	b, err := scanSettlementBatch(db.QueryRow(ctx, "INSERT INTO welfare_contract_settlement_batches(request_key,actor_id,note,total_count,account_id,settlement_reference,isk_minor,items,recipient_ids) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING "+settlementBatchColumns, key, actor, note, len(selections), account, reference, isk, items, recipients))
	if err != nil {
		return b, err
	}
	for _, item := range selections {
		if _, err = db.Exec(ctx, `INSERT INTO welfare_contract_settlement_items(batch_id,source,source_id) VALUES($1,$2,$3)`, b.ID, item.Source, item.SourceID); err != nil {
			return SettlementBatch{}, err
		}
	}
	return b, nil
}

func ListSettlementBatches(ctx context.Context, db DB, limit int) ([]SettlementBatch, error) {
	rows, err := db.Query(ctx, "SELECT "+settlementBatchColumns+" FROM welfare_contract_settlement_batches ORDER BY id DESC LIMIT $1", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SettlementBatch{}
	for rows.Next() {
		var b SettlementBatch
		if err = rows.Scan(&b.ID, &b.RequestKey, &b.ActorID, &b.Note, &b.State, &b.TotalCount, &b.CompletedCount, &b.FailedCount, &b.CreatedAt, &b.StartedAt, &b.FinishedAt, &b.NextRunAt, &b.Version, &b.SettlementReference, &b.AccountID, &b.ISKMinor, &b.Items, &b.RecipientIDs, &b.ContractID, &b.ContractRecipientID); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	if err = rows.Err(); err != nil || len(out) == 0 {
		return out, err
	}
	ids := make([]int64, 0, len(out))
	for _, b := range out {
		ids = append(ids, b.ID)
	}
	itemRows, err := db.Query(ctx, `SELECT id,batch_id,source,source_id,state,attempts,last_error,created_at,updated_at FROM welfare_contract_settlement_items WHERE batch_id=ANY($1::bigint[]) ORDER BY batch_id,id`, ids)
	if err != nil {
		return nil, err
	}
	defer itemRows.Close()
	items := make(map[int64][]SettlementItem, len(out))
	for itemRows.Next() {
		var item SettlementItem
		if err = itemRows.Scan(&item.ID, &item.BatchID, &item.Source, &item.SourceID, &item.State, &item.Attempts, &item.LastError, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items[item.BatchID] = append(items[item.BatchID], item)
	}
	if err = itemRows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Entries = items[out[i].ID]
		if out[i].Entries == nil {
			out[i].Entries = []SettlementItem{}
		}
	}
	return out, nil
}

func SettlementItems(ctx context.Context, db DB, batchID int64) ([]SettlementItem, error) {
	rows, err := db.Query(ctx, `SELECT id,batch_id,source,source_id,state,attempts,last_error,created_at,updated_at FROM welfare_contract_settlement_items WHERE batch_id=$1 ORDER BY id`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SettlementItem{}
	for rows.Next() {
		var item SettlementItem
		if err = rows.Scan(&item.ID, &item.BatchID, &item.Source, &item.SourceID, &item.State, &item.Attempts, &item.LastError, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func DueSettlementBatches(ctx context.Context, db DB) ([]int64, error) {
	rows, err := db.Query(ctx, `SELECT id FROM welfare_contract_settlement_batches WHERE state IN ('pending','processing','partial') AND next_run_at<=now() ORDER BY id LIMIT 20`)
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

func ClaimSettlementBatch(ctx context.Context, db DB, id int64) (bool, error) {
	result, err := db.Exec(ctx, `UPDATE welfare_contract_settlement_batches SET state='processing',started_at=COALESCE(started_at,now()),next_run_at=now()+interval '5 minutes',version=version+1 WHERE id=$1 AND state IN ('pending','processing','partial') AND next_run_at<=now()`, id)
	return result.RowsAffected() == 1, err
}

func ClaimSettlementItem(ctx context.Context, db DB, id int64) (SettlementItem, bool, error) {
	var item SettlementItem
	err := db.QueryRow(ctx, `UPDATE welfare_contract_settlement_items SET state='processing',attempts=attempts+1,updated_at=now() WHERE id=$1 AND state IN ('pending','failed') AND attempts<3 RETURNING id,batch_id,source,source_id,state,attempts,last_error,created_at,updated_at`, id).Scan(&item.ID, &item.BatchID, &item.Source, &item.SourceID, &item.State, &item.Attempts, &item.LastError, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return SettlementItem{}, false, nil
	}
	return item, err == nil, err
}

func MarkSettlementItem(ctx context.Context, db DB, id int64, state, detail string) error {
	detail = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(detail, "\r", " "), "\n", " "))
	if len([]rune(detail)) > 500 {
		detail = string([]rune(detail)[:500])
	}
	_, err := db.Exec(ctx, `UPDATE welfare_contract_settlement_items SET state=$2,last_error=$3,updated_at=now() WHERE id=$1`, id, state, detail)
	return err
}

func RefreshSettlementBatch(ctx context.Context, db DB, id int64) (SettlementBatch, error) {
	_, err := db.Exec(ctx, `UPDATE welfare_contract_settlement_batches b SET completed_count=x.completed,failed_count=x.failed,state=CASE WHEN x.completed=b.total_count THEN 'completed' WHEN x.completed+x.failed=b.total_count AND x.completed>0 THEN 'partial' WHEN x.completed+x.failed=b.total_count THEN 'failed' ELSE 'processing' END,finished_at=CASE WHEN x.completed+x.failed=b.total_count THEN now() ELSE NULL END,next_run_at=CASE WHEN x.completed+x.failed=b.total_count THEN now()+interval '365 days' ELSE now()+interval '1 minute' END,version=b.version+1 FROM (SELECT batch_id,count(*) FILTER (WHERE state='completed')::int AS completed,count(*) FILTER (WHERE state='failed' AND attempts>=3)::int AS failed FROM welfare_contract_settlement_items WHERE batch_id=$1 GROUP BY batch_id) x WHERE b.id=$1`, id)
	if err != nil {
		return SettlementBatch{}, err
	}
	return SettlementBatchByID(ctx, db, id)
}

func RetrySettlementBatch(ctx context.Context, db DB, id int64) error {
	_, err := db.Exec(ctx, `WITH reset AS (UPDATE welfare_contract_settlement_items SET state='pending',last_error='',updated_at=now() WHERE batch_id=$1 AND state='failed' AND attempts<3 RETURNING id) UPDATE welfare_contract_settlement_batches SET state='pending',finished_at=NULL,next_run_at=now(),version=version+1 WHERE id=$1`, id)
	return err
}

func RescheduleSettlementBatch(ctx context.Context, db DB, id int64, detail string) error {
	detail = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(detail, "\r", " "), "\n", " "))
	if len([]rune(detail)) > 500 {
		detail = string([]rune(detail)[:500])
	}
	_, err := db.Exec(ctx, `UPDATE welfare_contract_settlement_items SET state='pending',last_error=$2,updated_at=now() WHERE batch_id=$1 AND state IN ('pending','processing')`, id, detail)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, `UPDATE welfare_contract_settlement_batches SET state='pending',next_run_at=now()+interval '5 minutes',version=version+1 WHERE id=$1`, id)
	return err
}

func SetSettlementContract(ctx context.Context, db DB, id, contractID, recipientID int64) error {
	_, err := db.Exec(ctx, `UPDATE welfare_contract_settlement_batches SET contract_id=$2,contract_recipient_id=$3,version=version+1 WHERE id=$1`, id, contractID, recipientID)
	return err
}

package store

import (
	"context"
	"encoding/json"
	"time"
)

type Contribution struct {
	ID                int64      `json:"id,string"`
	PoolID            int64      `json:"pool_id,string"`
	AccountID         string     `json:"account_id"`
	LenderKind        string     `json:"lender_kind"`
	SourceCharacterID int64      `json:"source_character_id,string"`
	CorporationID     *int64     `json:"corporation_id,string"`
	AmountMinor       int64      `json:"amount_minor"`
	State             string     `json:"state"`
	ContractID        *int64     `json:"contract_id,string"`
	Version           int64      `json:"version"`
	CreatedAt         time.Time  `json:"created_at"`
	FundedAt          *time.Time `json:"funded_at"`
}
type PoolSummary struct {
	PendingMinor   int64 `json:"pending_minor"`
	FundedMinor    int64 `json:"funded_minor"`
	CashMinor      int64 `json:"cash_minor"`
	ReservedMinor  int64 `json:"reserved_minor"`
	AvailableMinor int64 `json:"available_minor"`
}

func SharedPool(ctx context.Context, db DBTX, lock bool) (Pool, error) {
	q := `SELECT id,lender_kind,lender_user_id,corporation_id,name,state,config,version,is_shared,custodian_character_id FROM loan_pools WHERE is_shared`
	if lock {
		q += ` FOR UPDATE`
	}
	var p Pool
	err := db.QueryRow(ctx, q).Scan(&p.ID, &p.LenderKind, &p.LenderUserID, &p.CorporationID, &p.Name, &p.State, &p.Config, &p.Version, &p.IsShared, &p.CustodianCharacterID)
	return p, err
}
func Summary(ctx context.Context, db DBTX, id int64) (PoolSummary, error) {
	var v PoolSummary
	err := db.QueryRow(ctx, `SELECT
 COALESCE((SELECT sum(amount_minor) FROM loan_contributions WHERE pool_id=$1 AND state='pending'),0),
 COALESCE((SELECT sum(amount_minor) FROM loan_contributions WHERE pool_id=$1 AND state='funded'),0),
 COALESCE((SELECT sum(delta_minor) FROM loan_pool_ledger WHERE pool_id=$1),0),
 COALESCE((SELECT sum(principal_minor) FROM loan_cases WHERE pool_id=$1 AND state IN ('approved','funding')),0)`, id).Scan(&v.PendingMinor, &v.FundedMinor, &v.CashMinor, &v.ReservedMinor)
	v.AvailableMinor = v.CashMinor - v.ReservedMinor
	return v, err
}

const contributionFields = `id,pool_id,account_id::text,lender_kind,source_character_id,corporation_id,amount_minor,state,contract_id,version,created_at,funded_at`

func ContributionByID(ctx context.Context, db DBTX, id int64) (Contribution, error) {
	var c Contribution
	err := db.QueryRow(ctx, `SELECT `+contributionFields+` FROM loan_contributions WHERE id=$1`, id).Scan(&c.ID, &c.PoolID, &c.AccountID, &c.LenderKind, &c.SourceCharacterID, &c.CorporationID, &c.AmountMinor, &c.State, &c.ContractID, &c.Version, &c.CreatedAt, &c.FundedAt)
	return c, err
}
func Contributions(ctx context.Context, db DBTX, actor string, all bool) ([]Contribution, error) {
	rows, err := db.Query(ctx, `SELECT `+contributionFields+` FROM loan_contributions WHERE account_id=$1::uuid OR $2 ORDER BY id DESC LIMIT 200`, actor, all)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Contribution{}
	for rows.Next() {
		var c Contribution
		if err = rows.Scan(&c.ID, &c.PoolID, &c.AccountID, &c.LenderKind, &c.SourceCharacterID, &c.CorporationID, &c.AmountMinor, &c.State, &c.ContractID, &c.Version, &c.CreatedAt, &c.FundedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func AddContribution(ctx context.Context, db DBTX, pool int64, actor, kind string, character, corp, amount int64) (Contribution, error) {
	var id int64
	err := db.QueryRow(ctx, `INSERT INTO loan_contributions(pool_id,account_id,lender_kind,source_character_id,corporation_id,amount_minor) VALUES($1,$2::uuid,$3,$4,NULLIF($5,0),$6) RETURNING id`, pool, actor, kind, character, corp, amount).Scan(&id)
	if err != nil {
		return Contribution{}, err
	}
	return ContributionByID(ctx, db, id)
}
func FundContribution(ctx context.Context, db DBTX, id, version int64, kind string, owner, contract int64, evidence json.RawMessage) (Contribution, error) {
	var updated int64
	err := db.QueryRow(ctx, `UPDATE loan_contributions SET state='funded',contract_kind=$3,contract_owner_id=$4,contract_id=$5,evidence=$6,funded_at=now(),version=version+1 WHERE id=$1 AND version=$2 AND state='pending' RETURNING id`, id, version, kind, owner, contract, evidence).Scan(&updated)
	if err != nil {
		return Contribution{}, err
	}
	return ContributionByID(ctx, db, updated)
}
func CancelContribution(ctx context.Context, db DBTX, id, version int64) (Contribution, error) {
	var updated int64
	err := db.QueryRow(ctx, `UPDATE loan_contributions SET state='cancelled',version=version+1 WHERE id=$1 AND version=$2 AND state='pending' RETURNING id`, id, version).Scan(&updated)
	if err != nil {
		return Contribution{}, err
	}
	return ContributionByID(ctx, db, updated)
}
func PoolLedger(ctx context.Context, db DBTX, pool, contribution, payment, delta int64, kind, actor string) error {
	_, err := db.Exec(ctx, `INSERT INTO loan_pool_ledger(pool_id,contribution_id,payment_id,delta_minor,kind,actor_id) VALUES($1,NULLIF($2,0),NULLIF($3,0),$4,$5,$6::uuid)`, pool, contribution, payment, delta, kind, actor)
	return err
}

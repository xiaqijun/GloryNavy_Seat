package store

import (
	"context"
	"encoding/json"
	"time"
)

// CreditPolicy controls the income-to-credit conversion and security caps.
// It is a singleton because the loan pool is a singleton.
type CreditPolicy struct {
	IncomeWindowDays         int       `json:"income_window_days"`
	AnnualRateBPS            int       `json:"annual_rate_bps"`
	DebtRatioBPS             int       `json:"debt_ratio_bps"`
	TermMonths               int       `json:"term_months"`
	ScoreFactorBPS           int       `json:"score_factor_bps"`
	HardCapMinor             int64     `json:"hard_cap_minor"`
	GuaranteeCapBPS          int       `json:"guarantee_cap_bps"`
	CollateralCapBPS         int       `json:"collateral_cap_bps"`
	StaleAfterHours          int       `json:"stale_after_hours"`
	MinIncomeSamples         int       `json:"min_income_samples"`
	InsufficientIncomePolicy string    `json:"insufficient_income_policy"`
	Version                  int64     `json:"version"`
	UpdatedBy                *string   `json:"updated_by,omitempty"`
	UpdatedAt                time.Time `json:"updated_at"`
}

func ReadCreditPolicy(ctx context.Context, db DBTX) (CreditPolicy, error) {
	var p CreditPolicy
	err := db.QueryRow(ctx, `SELECT income_window_days,annual_rate_bps,debt_ratio_bps,term_months,score_factor_bps,hard_cap_minor,guarantee_cap_bps,collateral_cap_bps,stale_after_hours,min_income_samples,insufficient_income_policy,version,updated_by::text,updated_at FROM loan_credit_policy WHERE singleton`).
		Scan(&p.IncomeWindowDays, &p.AnnualRateBPS, &p.DebtRatioBPS, &p.TermMonths, &p.ScoreFactorBPS, &p.HardCapMinor, &p.GuaranteeCapBPS, &p.CollateralCapBPS, &p.StaleAfterHours, &p.MinIncomeSamples, &p.InsufficientIncomePolicy, &p.Version, &p.UpdatedBy, &p.UpdatedAt)
	return p, err
}

func SaveCreditPolicy(ctx context.Context, db DBTX, actor string, p CreditPolicy) (CreditPolicy, error) {
	var out CreditPolicy
	err := db.QueryRow(ctx, `UPDATE loan_credit_policy
SET income_window_days=$1,annual_rate_bps=$2,debt_ratio_bps=$3,term_months=$4,score_factor_bps=$5,hard_cap_minor=$6,
    guarantee_cap_bps=$7,collateral_cap_bps=$8,stale_after_hours=$9,min_income_samples=$10,
    insufficient_income_policy=$11,version=version+1,updated_by=$12::uuid,updated_at=now()
WHERE singleton AND version=$13
RETURNING income_window_days,annual_rate_bps,debt_ratio_bps,term_months,score_factor_bps,hard_cap_minor,guarantee_cap_bps,collateral_cap_bps,stale_after_hours,min_income_samples,insufficient_income_policy,version,updated_by::text,updated_at`,
		p.IncomeWindowDays, p.AnnualRateBPS, p.DebtRatioBPS, p.TermMonths, p.ScoreFactorBPS, p.HardCapMinor, p.GuaranteeCapBPS, p.CollateralCapBPS, p.StaleAfterHours, p.MinIncomeSamples, p.InsufficientIncomePolicy, actor, p.Version).
		Scan(&out.IncomeWindowDays, &out.AnnualRateBPS, &out.DebtRatioBPS, &out.TermMonths, &out.ScoreFactorBPS, &out.HardCapMinor, &out.GuaranteeCapBPS, &out.CollateralCapBPS, &out.StaleAfterHours, &out.MinIncomeSamples, &out.InsufficientIncomePolicy, &out.Version, &out.UpdatedBy, &out.UpdatedAt)
	if err != nil {
		return out, err
	}
	snapshot, err := json.Marshal(out)
	if err != nil {
		return out, err
	}
	_, err = db.Exec(ctx, `INSERT INTO loan_credit_policy_audit(actor_id,policy_version,snapshot) VALUES($1::uuid,$2,$3)`, actor, out.Version, snapshot)
	return out, err
}

func CreditEvaluations(ctx context.Context, db DBTX, account string, limit int) ([]CreditEvaluation, error) {
	if limit < 1 || limit > 100 {
		limit = 30
	}
	rows, err := db.Query(ctx, `SELECT id,account_id::text,policy_version,score,total_limit_minor,unsecured_limit_minor,state,factor_scores,evidence,evidence_cutoff,evaluated_at
FROM loan_credit_evaluations WHERE account_id=$1::uuid ORDER BY id DESC LIMIT $2`, account, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]CreditEvaluation, 0)
	for rows.Next() {
		var e CreditEvaluation
		if err := rows.Scan(&e.ID, &e.AccountID, &e.PolicyVersion, &e.Score, &e.TotalLimitMinor, &e.UnsecuredLimitMinor, &e.State, &e.FactorScores, &e.Evidence, &e.EvidenceCutoff, &e.EvaluatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

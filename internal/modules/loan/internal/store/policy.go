package store

import (
	"context"
	"encoding/json"
	"time"
)

// CreditPolicy controls the income/consumption-to-credit conversion and security caps.
// It is a singleton because the loan pool is a singleton.
type CreditPolicy struct {
	IncomeWindowDays         int       `json:"income_window_days"`
	AnnualRateBPS            int       `json:"annual_rate_bps"`
	DebtRatioBPS             int       `json:"debt_ratio_bps"`
	TermMonths               int       `json:"term_months"`
	ScoreFactorBPS           int       `json:"score_factor_bps"`
	IncomeWeightBPS          int       `json:"income_weight_bps"`
	AssetCreditLTVBPS        int       `json:"asset_credit_ltv_bps"`
	GuaranteeCapBPS          int       `json:"guarantee_cap_bps"`
	CollateralCapBPS         int       `json:"collateral_cap_bps"`
	BorrowerLimitBPS         int       `json:"borrower_limit_bps"`
	StaleAfterHours          int       `json:"stale_after_hours"`
	MinIncomeSamples         int       `json:"min_income_samples"`
	InsufficientIncomePolicy string    `json:"insufficient_income_policy"`
	Version                  int64     `json:"version"`
	UpdatedBy                *string   `json:"updated_by,omitempty"`
	UpdatedAt                time.Time `json:"updated_at"`
}

func ReadCreditPolicy(ctx context.Context, db DBTX) (CreditPolicy, error) {
	var p CreditPolicy
	err := db.QueryRow(ctx, `SELECT income_window_days,annual_rate_bps,debt_ratio_bps,term_months,score_factor_bps,income_weight_bps,asset_credit_ltv_bps,guarantee_cap_bps,collateral_cap_bps,borrower_limit_bps,stale_after_hours,min_income_samples,insufficient_income_policy,version,updated_by::text,updated_at FROM loan_credit_policy WHERE singleton`).
		Scan(&p.IncomeWindowDays, &p.AnnualRateBPS, &p.DebtRatioBPS, &p.TermMonths, &p.ScoreFactorBPS, &p.IncomeWeightBPS, &p.AssetCreditLTVBPS, &p.GuaranteeCapBPS, &p.CollateralCapBPS, &p.BorrowerLimitBPS, &p.StaleAfterHours, &p.MinIncomeSamples, &p.InsufficientIncomePolicy, &p.Version, &p.UpdatedBy, &p.UpdatedAt)
	return p, err
}

func SaveCreditPolicy(ctx context.Context, db DBTX, actor string, p CreditPolicy) (CreditPolicy, error) {
	var out CreditPolicy
	err := db.QueryRow(ctx, `UPDATE loan_credit_policy
SET income_window_days=$1,annual_rate_bps=$2,debt_ratio_bps=$3,term_months=$4,score_factor_bps=$5,income_weight_bps=$6,asset_credit_ltv_bps=$7,
 guarantee_cap_bps=$8,collateral_cap_bps=$9,borrower_limit_bps=$10,stale_after_hours=$11,min_income_samples=$12,
 insufficient_income_policy=$13,version=version+1,updated_by=$14::uuid,updated_at=now()
WHERE singleton AND version=$15
	RETURNING income_window_days,annual_rate_bps,debt_ratio_bps,term_months,score_factor_bps,income_weight_bps,asset_credit_ltv_bps,guarantee_cap_bps,collateral_cap_bps,borrower_limit_bps,stale_after_hours,min_income_samples,insufficient_income_policy,version,updated_by::text,updated_at`,
		p.IncomeWindowDays, p.AnnualRateBPS, p.DebtRatioBPS, p.TermMonths, p.ScoreFactorBPS, p.IncomeWeightBPS, p.AssetCreditLTVBPS, p.GuaranteeCapBPS, p.CollateralCapBPS, p.BorrowerLimitBPS, p.StaleAfterHours, p.MinIncomeSamples, p.InsufficientIncomePolicy, actor, p.Version).
		Scan(&out.IncomeWindowDays, &out.AnnualRateBPS, &out.DebtRatioBPS, &out.TermMonths, &out.ScoreFactorBPS, &out.IncomeWeightBPS, &out.AssetCreditLTVBPS, &out.GuaranteeCapBPS, &out.CollateralCapBPS, &out.BorrowerLimitBPS, &out.StaleAfterHours, &out.MinIncomeSamples, &out.InsufficientIncomePolicy, &out.Version, &out.UpdatedBy, &out.UpdatedAt)
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
	rows, err := db.Query(ctx, `SELECT id,account_id::text,policy_version,score,total_limit_minor,unsecured_limit_minor,guarantee_limit_minor,asset_support_limit_minor,state,factor_scores,evidence,evidence_cutoff,evaluated_at
FROM loan_credit_evaluations WHERE account_id=$1::uuid ORDER BY id DESC LIMIT $2`, account, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]CreditEvaluation, 0)
	for rows.Next() {
		var e CreditEvaluation
		if err := rows.Scan(&e.ID, &e.AccountID, &e.PolicyVersion, &e.Score, &e.TotalLimitMinor, &e.UnsecuredLimitMinor, &e.GuaranteeLimitMinor, &e.AssetSupportLimitMinor, &e.State, &e.FactorScores, &e.Evidence, &e.EvidenceCutoff, &e.EvaluatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func LatestCreditEvaluation(ctx context.Context, db DBTX, account string) (CreditEvaluation, error) {
	var e CreditEvaluation
	err := db.QueryRow(ctx, `SELECT id,account_id::text,policy_version,score,total_limit_minor,unsecured_limit_minor,guarantee_limit_minor,asset_support_limit_minor,state,factor_scores,evidence,evidence_cutoff,evaluated_at
FROM loan_credit_evaluations WHERE account_id=$1::uuid ORDER BY evaluated_at DESC,id DESC LIMIT 1`, account).
		Scan(&e.ID, &e.AccountID, &e.PolicyVersion, &e.Score, &e.TotalLimitMinor, &e.UnsecuredLimitMinor, &e.GuaranteeLimitMinor, &e.AssetSupportLimitMinor, &e.State, &e.FactorScores, &e.Evidence, &e.EvidenceCutoff, &e.EvaluatedAt)
	return e, err
}

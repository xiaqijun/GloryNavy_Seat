package loan

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"time"

	"glorynavy.local/seat/internal/modules/loan/internal/store"
)

const creditRuleVersion = "system-v2"

type IncomeEvidence struct {
	MonthlyNetIncomeMinor int64
	Samples               int
	Available             bool
	ObservedAt            time.Time
	SnapshotID            string
}

// CreditEvidence is deliberately narrow. Authorization state and account age
// are access/data freshness gates, never score inputs.
type CreditEvidence struct {
	PAPPoints                   int
	CorporationAttendancePoints int
	AllianceFulfillmentPoints   int
	CorporationAttendanceBPS    int32
	CorporationEligibleEvents   int64
	CorporationAttendedEvents   int64
	AllianceFulfillmentBPS      int32
	AllianceEligibleMonths      int64
	AllianceFulfilledMonths     int64
	AssetPoints                 int
	AssetValueMinor             int64
	PAPAvailable                bool
	AssetAvailable              bool
	PAPSnapshotID               string
	AssetSnapshotID             string
	EvidenceCutoff              time.Time
	Income                      IncomeEvidence
}

func defaultCreditPolicy() store.CreditPolicy {
	return store.CreditPolicy{IncomeWindowDays: 180, AnnualRateBPS: 1200, DebtRatioBPS: 3000, TermMonths: 6, ScoreFactorBPS: 10000, HardCapMinor: 1_000_000_000, GuaranteeCapBPS: 10000, CollateralCapBPS: 10000, StaleAfterHours: 72, MinIncomeSamples: 3, InsufficientIncomePolicy: "fallback_pool", Version: 1}
}

func clampScore(v, max int) int {
	if v < 0 {
		return 0
	}
	if v > max {
		return max
	}
	return v
}

// assessCredit applies the published weights: repayment 55, leverage 20,
// security history 10, PAP 5 (corporation attendance 3 plus alliance
// fulfillment 2) and asset scale 10. Missing PAP/assets are neutral and never
// negative. Account age and authorization are absent by construction.
func assessCredit(account string, signals store.CreditSignals, maxPrincipal int64, now time.Time) store.Credit {
	return assessCreditWithEvidence(account, signals, CreditEvidence{}, maxPrincipal, now)
}

func assessCreditWithEvidence(account string, signals store.CreditSignals, evidence CreditEvidence, maxPrincipal int64, now time.Time) store.Credit {
	repayment := 25
	if signals.TotalInstallments > 0 {
		repayment = 15 + signals.PaidInstallments*35/signals.TotalInstallments
		repayment += clampScore(signals.SettledLoans*2, 10)
	}
	repayment -= signals.OverdueInstallments * 8
	repayment -= signals.DefaultedLoans * 55
	repayment = clampScore(repayment, 55)

	leverage := 20
	if maxPrincipal > 0 && signals.OutstandingMinor > 0 {
		used := signals.OutstandingMinor
		if used > maxPrincipal {
			used = maxPrincipal
		}
		leverage = 20 - int((used*20)/maxPrincipal)
	}
	leverage = clampScore(leverage, 20)
	security := clampScore(10-signals.SecurityDisputes*5, 10)
	pap := clampScore(evidence.PAPPoints, 5)
	assets := clampScore(evidence.AssetPoints, 10)
	score := clampScore(repayment+leverage+security+pap+assets, 100)
	state := "active"
	if signals.DefaultedLoans > 0 {
		state = "suspended"
	}
	total := safeScaled(maxPrincipal, int64(score), 100)
	unsecuredRatio := int64(45 + score/2)
	unsecured := safeScaled(total, unsecuredRatio, 100)
	if state != "active" {
		total, unsecured = 0, 0
	}
	evidenceCutoff := evidence.EvidenceCutoff
	if evidenceCutoff.IsZero() {
		evidenceCutoff = now
	}
	reason := fmt.Sprintf("system-evaluated-v2: repayment=%d/55 leverage=%d/20 security=%d/10 corporation_attendance=%d/3 alliance_pap_fulfillment=%d/2 assets=%d/10 settled=%d active=%d defaulted=%d paid_installments=%d/%d overdue=%d", repayment, leverage, security, evidence.CorporationAttendancePoints, evidence.AllianceFulfillmentPoints, assets, signals.SettledLoans, signals.ActiveLoans, signals.DefaultedLoans, signals.PaidInstallments, signals.TotalInstallments, signals.OverdueInstallments)
	return store.Credit{
		AccountID: account, Score: &score, TotalLimitMinor: total, UnsecuredLimitMinor: unsecured,
		State: state, RuleVersion: creditRuleVersion, Reason: reason, Version: 1, EvaluatedAt: now,
		SettledLoans: signals.SettledLoans, ActiveLoans: signals.ActiveLoans, DefaultedLoans: signals.DefaultedLoans,
		PaidInstallments: signals.PaidInstallments, TotalInstallments: signals.TotalInstallments, OverdueInstallments: signals.OverdueInstallments,
		RepaymentPoints: repayment, LeveragePoints: leverage, SecurityPoints: security, PAPPoints: pap, CorporationAttendancePoints: evidence.CorporationAttendancePoints, AllianceFulfillmentPoints: evidence.AllianceFulfillmentPoints,
		CorporationAttendanceBPS: evidence.CorporationAttendanceBPS, CorporationEligibleEvents: evidence.CorporationEligibleEvents, CorporationAttendedEvents: evidence.CorporationAttendedEvents,
		AllianceFulfillmentBPS: evidence.AllianceFulfillmentBPS, AllianceEligibleMonths: evidence.AllianceEligibleMonths, AllianceFulfilledMonths: evidence.AllianceFulfilledMonths,
		AssetPoints: assets,
		Evidence:    evidenceSummary(evidence, evidenceCutoff),
	}
}

func safeScaled(value, numerator, denominator int64) int64 {
	if value <= 0 || numerator <= 0 || denominator <= 0 {
		return 0
	}
	n := new(big.Int).Mul(big.NewInt(value), big.NewInt(numerator))
	n.Quo(n, big.NewInt(denominator))
	if !n.IsInt64() {
		return math.MaxInt64 / denominator
	}
	return n.Int64()
}

func assessCreditWithPolicy(account string, signals store.CreditSignals, evidence CreditEvidence, poolCash int64, policy store.CreditPolicy, now time.Time) store.Credit {
	c := assessCreditWithEvidence(account, signals, evidence, poolCash, now)
	income := evidence.Income
	usable := income.Available && income.Samples >= policy.MinIncomeSamples && !income.ObservedAt.IsZero() && !income.ObservedAt.After(now) && now.Sub(income.ObservedAt) <= time.Duration(policy.StaleAfterHours)*time.Hour
	base := poolCash
	if usable {
		incomeCapacity := safeScaled(income.MonthlyNetIncomeMinor, int64(policy.DebtRatioBPS)*int64(policy.TermMonths), 10000)
		if incomeCapacity < base {
			base = incomeCapacity
		}
	} else if policy.InsufficientIncomePolicy == "zero" {
		base = 0
	}
	if policy.HardCapMinor > 0 && base > policy.HardCapMinor {
		base = policy.HardCapMinor
	}
	total := safeScaled(base, int64(*c.Score), 100)
	total = safeScaled(total, int64(policy.ScoreFactorBPS), 10000)
	unsecured := safeScaled(total, int64(45+*c.Score/2), 100)
	c.TotalLimitMinor, c.UnsecuredLimitMinor = total, unsecured
	c.GuaranteeLimitMinor = safeScaled(total, int64(policy.GuaranteeCapBPS), 10000)
	if c.State != "active" {
		c.TotalLimitMinor, c.UnsecuredLimitMinor, c.GuaranteeLimitMinor = 0, 0, 0
	}
	c.IncomeAvailable, c.MonthlyIncomeMinor = usable, income.MonthlyNetIncomeMinor
	c.PolicyVersion, c.RuleVersion = policy.Version, fmt.Sprintf("income-v1-policy-%d", policy.Version)
	c.Reason = fmt.Sprintf("%s; base=%d; income_available=%t; income_samples=%d; monthly_income=%d; debt_ratio_bps=%d; term_months=%d; score_factor_bps=%d", c.Reason, base, usable, income.Samples, income.MonthlyNetIncomeMinor, policy.DebtRatioBPS, policy.TermMonths, policy.ScoreFactorBPS)
	return c
}

func evidenceSummary(e CreditEvidence, cutoff time.Time) string {
	b, _ := json.Marshal(map[string]any{
		"pap_available": e.PAPAvailable, "asset_available": e.AssetAvailable,
		"asset_value_minor": e.AssetValueMinor, "pap_snapshot_id": e.PAPSnapshotID,
		"asset_snapshot_id": e.AssetSnapshotID, "evidence_cutoff": cutoff,
		"corporation_attendance_bps": e.CorporationAttendanceBPS, "corporation_eligible_events": e.CorporationEligibleEvents, "corporation_attended_events": e.CorporationAttendedEvents,
		"alliance_fulfillment_bps": e.AllianceFulfillmentBPS, "alliance_eligible_months": e.AllianceEligibleMonths, "alliance_fulfilled_months": e.AllianceFulfilledMonths,
		"income_available": e.Income.Available, "monthly_net_income_minor": e.Income.MonthlyNetIncomeMinor, "income_samples": e.Income.Samples, "income_snapshot_id": e.Income.SnapshotID, "income_observed_at": e.Income.ObservedAt,
	})
	return string(b)
}

func (s *Service) Credit(ctx context.Context, user string) (store.Credit, error) {
	signals, err := store.CreditSignalsFor(ctx, s.db(), user)
	if err != nil {
		return store.Credit{}, err
	}
	var evidence CreditEvidence
	if s.CreditEvidence != nil {
		if v, e := s.CreditEvidence(ctx, user); e == nil {
			evidence = v
		}
	}
	now := time.Now().UTC()
	policy, err := store.ReadCreditPolicy(ctx, s.db())
	if err != nil {
		return store.Credit{}, err
	}
	if s.IncomeEvidence != nil && policy.IncomeWindowDays > 0 {
		if income, e := s.IncomeEvidence(ctx, user, now.AddDate(0, 0, -policy.IncomeWindowDays)); e == nil {
			evidence.Income = income
		}
	}
	maxPrincipal := int64(0)
	if pool, poolErr := store.SharedPool(ctx, s.db(), false); poolErr == nil {
		if summary, summaryErr := store.Summary(ctx, s.db(), pool.ID); summaryErr == nil {
			maxPrincipal = summary.CashMinor
		}
	}
	return assessCreditWithPolicy(user, signals, evidence, maxPrincipal, policy, now), nil
}

func (s *Service) recordCreditEvaluation(ctx context.Context, tx store.DBTX, c store.Credit) error {
	if c.Score == nil {
		return nil
	}
	factors, _ := json.Marshal(map[string]int{"repayment": c.RepaymentPoints, "leverage": c.LeveragePoints, "security": c.SecurityPoints, "corporation_attendance": c.CorporationAttendancePoints, "alliance_pap_fulfillment": c.AllianceFulfillmentPoints, "pap": c.PAPPoints, "assets": c.AssetPoints})
	returned, _ := json.Marshal(map[string]any{"reason": c.Reason, "evidence": c.Evidence, "guarantee_limit_minor": c.GuaranteeLimitMinor, "policy_version": c.PolicyVersion})
	_, err := store.RecordCreditEvaluation(ctx, tx, store.CreditEvaluation{AccountID: c.AccountID, PolicyVersion: c.RuleVersion, Score: *c.Score, TotalLimitMinor: c.TotalLimitMinor, UnsecuredLimitMinor: c.UnsecuredLimitMinor, State: c.State, FactorScores: factors, Evidence: returned, EvidenceCutoff: c.EvaluatedAt, EvaluatedAt: c.EvaluatedAt})
	return err
}

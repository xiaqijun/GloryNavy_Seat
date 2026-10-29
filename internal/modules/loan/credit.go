package loan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/loan/internal/store"
)

const creditRuleVersion = "system-v2"

// Asset scale is scored from aggregate personal ESI assets. Values are stored
// in ISK minor units (1 ISK = 100 minor units).
var assetScaleBands = [...]struct {
	ThresholdMinor int64
	Points         int
}{
	{100_000_000 * 100, 2},
	{500_000_000 * 100, 4},
	{1_000_000_000 * 100, 6},
	{5_000_000_000 * 100, 8},
	{10_000_000_000 * 100, 10},
}

type IncomeEvidence struct {
	MonthlyIncomeMinor  int64
	MonthlyExpenseMinor int64
	Samples             int
	Available           bool
	ObservedAt          time.Time
	SnapshotID          string
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
	// AssetPricedTypes and AssetUnpricedTypes make partial ESI/market
	// observations auditable without exposing the underlying inventory.
	AssetPricedTypes   int
	AssetUnpricedTypes int
	AssetEvidenceError string
	PAPSnapshotID      string
	AssetSnapshotID    string
	AssetObservedAt    time.Time
	PAPObservedAt      time.Time
	EvidenceCutoff     time.Time
	Income             IncomeEvidence
}

func defaultCreditPolicy() store.CreditPolicy {
	return store.CreditPolicy{IncomeWindowDays: 180, AnnualRateBPS: 1200, DebtRatioBPS: 3000, TermMonths: 6, ScoreFactorBPS: 10000, IncomeWeightBPS: 7000, AssetCreditLTVBPS: 1000, GuaranteeCapBPS: 10000, CollateralCapBPS: 10000, StaleAfterHours: 72, MinIncomeSamples: 1, InsufficientIncomePolicy: "zero", Version: 1}
}

func creditPolicyRuleVersion(policy store.CreditPolicy) string {
	return fmt.Sprintf("capacity-v7-policy-%d", policy.Version)
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

func assetScaleScore(valueMinor int64) int {
	points := 0
	for _, band := range assetScaleBands {
		if valueMinor < band.ThresholdMinor {
			break
		}
		points = band.Points
	}
	return points
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
	assets := 0
	if evidence.AssetAvailable {
		assets = assetScaleScore(evidence.AssetValueMinor)
	}
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
		AssetPoints: assets, AssetValueMinor: evidence.AssetValueMinor,
		Evidence: evidenceSummary(evidence, evidenceCutoff),
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
	fresh := func(at time.Time) bool {
		return !at.IsZero() && !at.After(now) && now.Sub(at) <= time.Duration(policy.StaleAfterHours)*time.Hour
	}
	if !fresh(evidence.AssetObservedAt) {
		evidence.AssetAvailable = false
	}
	if !fresh(evidence.PAPObservedAt) {
		evidence.PAPAvailable = false
		evidence.PAPPoints, evidence.CorporationAttendancePoints, evidence.AllianceFulfillmentPoints = 0, 0, 0
	}
	c := assessCreditWithEvidence(account, signals, evidence, poolCash, now)
	income := evidence.Income
	usable := income.Available && income.MonthlyIncomeMinor > 0 && income.Samples >= policy.MinIncomeSamples && fresh(income.ObservedAt)
	base := int64(0)
	assetSupport := int64(0)
	if usable {
		factor := int64(policy.DebtRatioBPS) * int64(policy.TermMonths)
		base = safeScaled(income.MonthlyIncomeMinor, factor, 10000)
	} else if policy.InsufficientIncomePolicy == "fallback_pool" {
		// Keep the legacy option explicit for administrators who choose it;
		// the new default is evidence-first zero.
		base = poolCash
	}
	if evidence.AssetAvailable && evidence.AssetValueMinor > 0 && policy.AssetCreditLTVBPS > 0 {
		assetSupport = safeScaled(evidence.AssetValueMinor, int64(policy.AssetCreditLTVBPS), 10000)
	}
	// The credit score discounts cash-flow capacity. Verified personal assets
	// provide a separate conservative support line; any amount above the
	// unsecured line still requires an approved guarantee or collateral.
	unsecured := safeScaled(base, int64(*c.Score), 100)
	unsecured = safeScaled(unsecured, int64(policy.ScoreFactorBPS), 10000)
	total := unsecured + assetSupport
	if total < unsecured || total > poolCash {
		total = poolCash
	}
	if unsecured > total {
		unsecured = total
	}
	c.TotalLimitMinor, c.UnsecuredLimitMinor = total, unsecured
	c.AssetSupportLimitMinor = total - unsecured
	c.GuaranteeLimitMinor = safeScaled(total, int64(policy.GuaranteeCapBPS), 10000)
	if c.State != "active" {
		c.TotalLimitMinor, c.UnsecuredLimitMinor, c.GuaranteeLimitMinor = 0, 0, 0
	}
	c.IncomeAvailable, c.MonthlyIncomeMinor = usable, income.MonthlyIncomeMinor
	c.MonthlyExpenseMinor = income.MonthlyExpenseMinor
	c.PolicyVersion, c.RuleVersion = policy.Version, creditPolicyRuleVersion(policy)
	evidence.Income.Available = usable
	cutoff := now
	for _, at := range []time.Time{func() time.Time {
		if usable {
			return income.ObservedAt
		}
		return now
	}(), func() time.Time {
		if evidence.AssetAvailable {
			return evidence.AssetObservedAt
		}
		return now
	}(), func() time.Time {
		if evidence.PAPPoints > 0 {
			return evidence.PAPObservedAt
		}
		return now
	}()} {
		if at.Before(cutoff) {
			cutoff = at
		}
	}
	c.EvidenceCutoff = cutoff
	c.Evidence = evidenceSummary(evidence, cutoff)
	c.Reason = fmt.Sprintf("%s; cashflow_base=%d; unsecured=%d; asset_support=%d; income_available=%t; income_samples=%d; monthly_income=%d; monthly_consumption=%d; debt_ratio_bps=%d; term_months=%d; score_factor_bps=%d; asset_credit_ltv_bps=%d", c.Reason, base, unsecured, c.AssetSupportLimitMinor, usable, income.Samples, income.MonthlyIncomeMinor, income.MonthlyExpenseMinor, policy.DebtRatioBPS, policy.TermMonths, policy.ScoreFactorBPS, policy.AssetCreditLTVBPS)
	return c
}

func evidenceSummary(e CreditEvidence, cutoff time.Time) string {
	b, _ := json.Marshal(map[string]any{
		"pap_available": e.PAPAvailable, "asset_available": e.AssetAvailable,
		"asset_value_minor": e.AssetValueMinor, "pap_snapshot_id": e.PAPSnapshotID,
		"asset_snapshot_id": e.AssetSnapshotID, "evidence_cutoff": cutoff,
		"asset_priced_types": e.AssetPricedTypes, "asset_unpriced_types": e.AssetUnpricedTypes,
		"asset_evidence_error":       e.AssetEvidenceError,
		"corporation_attendance_bps": e.CorporationAttendanceBPS, "corporation_eligible_events": e.CorporationEligibleEvents, "corporation_attended_events": e.CorporationAttendedEvents,
		"alliance_fulfillment_bps": e.AllianceFulfillmentBPS, "alliance_eligible_months": e.AllianceEligibleMonths, "alliance_fulfilled_months": e.AllianceFulfilledMonths,
		"income_available": e.Income.Available, "monthly_income_minor": e.Income.MonthlyIncomeMinor, "monthly_consumption_minor": e.Income.MonthlyExpenseMinor, "income_samples": e.Income.Samples, "income_snapshot_id": e.Income.SnapshotID, "income_observed_at": e.Income.ObservedAt,
	})
	return string(b)
}

func creditFromEvaluation(e store.CreditEvaluation) store.Credit {
	score := e.Score
	c := store.Credit{AccountID: e.AccountID, Score: &score, TotalLimitMinor: e.TotalLimitMinor, UnsecuredLimitMinor: e.UnsecuredLimitMinor, GuaranteeLimitMinor: e.GuaranteeLimitMinor, AssetSupportLimitMinor: e.AssetSupportLimitMinor, State: e.State, RuleVersion: e.PolicyVersion, Version: 1, EvaluatedAt: e.EvaluatedAt, EvidenceCutoff: e.EvidenceCutoff}
	if len(e.Evidence) == 0 {
		return c
	}
	var envelope struct {
		Reason        string `json:"reason"`
		Evidence      string `json:"evidence"`
		PolicyVersion int64  `json:"policy_version"`
	}
	if json.Unmarshal(e.Evidence, &envelope) == nil {
		c.Reason, c.Evidence = envelope.Reason, envelope.Evidence
		c.PolicyVersion = envelope.PolicyVersion
	}
	var factors map[string]int
	if json.Unmarshal(e.FactorScores, &factors) == nil {
		c.RepaymentPoints = factors["repayment"]
		c.LeveragePoints = factors["leverage"]
		c.SecurityPoints = factors["security"]
		c.PAPPoints = factors["pap"]
		c.AssetPoints = factors["assets"]
		c.CorporationAttendancePoints = factors["corporation_attendance"]
		c.AllianceFulfillmentPoints = factors["alliance_pap_fulfillment"]
	}
	var evidence struct {
		IncomeAvailable         bool  `json:"income_available"`
		MonthlyIncomeMinor      int64 `json:"monthly_income_minor"`
		MonthlyConsumptionMinor int64 `json:"monthly_consumption_minor"`
		AssetValueMinor         int64 `json:"asset_value_minor"`
	}
	if c.Evidence != "" && json.Unmarshal([]byte(c.Evidence), &evidence) == nil {
		c.IncomeAvailable = evidence.IncomeAvailable
		c.MonthlyIncomeMinor = evidence.MonthlyIncomeMinor
		c.MonthlyExpenseMinor = evidence.MonthlyConsumptionMinor
		c.AssetValueMinor = evidence.AssetValueMinor
	}
	if c.PolicyVersion == 0 {
		if marker := strings.LastIndex(e.PolicyVersion, "policy-"); marker >= 0 {
			if v, err := strconv.ParseInt(e.PolicyVersion[marker+len("policy-"):], 10, 64); err == nil {
				c.PolicyVersion = v
			}
		}
	}
	return c
}

func (s *Service) Credit(ctx context.Context, user string) (store.Credit, error) {
	evaluation, err := store.LatestCreditEvaluation(ctx, s.db(), user)
	if err == nil {
		credit := creditFromEvaluation(evaluation)
		policy, err := store.ReadCreditPolicy(ctx, s.db())
		if err != nil {
			return store.Credit{}, err
		}
		if credit.State == "active" && !creditFresh(credit, policy, time.Now().UTC()) {
			credit.State = "provisional"
			credit.TotalLimitMinor, credit.UnsecuredLimitMinor, credit.GuaranteeLimitMinor, credit.AssetSupportLimitMinor = 0, 0, 0, 0
			credit.Score, credit.IncomeAvailable = nil, false
			credit.Reason = "等待最新信用评估"
		}
		return credit, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return store.Credit{}, err
	}
	score := 0
	return store.Credit{AccountID: user, Score: &score, State: "provisional", RuleVersion: "periodic-v3", Reason: "等待周期性信用评估"}, nil
}

func (s *Service) evaluateCreditNow(ctx context.Context, user string) (store.Credit, error) {
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

// EvaluateAndStore is called by the periodic River worker. It is deliberately
// separate from Credit so page reads and approval paths never trigger ESI or
// wallet/asset aggregation.
func (s *Service) EvaluateAndStore(ctx context.Context, user string) (store.Credit, error) {
	c, err := s.evaluateCreditNow(ctx, user)
	if err != nil {
		return store.Credit{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return store.Credit{}, err
	}
	defer tx.Rollback(ctx)
	if err = s.recordCreditEvaluation(ctx, tx, c); err != nil {
		return store.Credit{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return store.Credit{}, err
	}
	return c, nil
}

func creditFresh(c store.Credit, policy store.CreditPolicy, now time.Time) bool {
	return c.RuleVersion == creditPolicyRuleVersion(policy) && !c.EvaluatedAt.IsZero() && !c.EvaluatedAt.After(now) && now.Sub(c.EvaluatedAt) <= time.Duration(policy.StaleAfterHours)*time.Hour && !c.EvidenceCutoff.IsZero() && !c.EvidenceCutoff.After(now) && now.Sub(c.EvidenceCutoff) <= time.Duration(policy.StaleAfterHours)*time.Hour
}

func (s *Service) recordCreditEvaluation(ctx context.Context, tx store.DBTX, c store.Credit) error {
	if c.Score == nil {
		return nil
	}
	factors, _ := json.Marshal(map[string]int{"repayment": c.RepaymentPoints, "leverage": c.LeveragePoints, "security": c.SecurityPoints, "corporation_attendance": c.CorporationAttendancePoints, "alliance_pap_fulfillment": c.AllianceFulfillmentPoints, "pap": c.PAPPoints, "assets": c.AssetPoints})
	returned, _ := json.Marshal(map[string]any{"reason": c.Reason, "evidence": c.Evidence, "guarantee_limit_minor": c.GuaranteeLimitMinor, "asset_support_limit_minor": c.AssetSupportLimitMinor, "policy_version": c.PolicyVersion})
	_, err := store.RecordCreditEvaluation(ctx, tx, store.CreditEvaluation{AccountID: c.AccountID, PolicyVersion: c.RuleVersion, Score: *c.Score, TotalLimitMinor: c.TotalLimitMinor, UnsecuredLimitMinor: c.UnsecuredLimitMinor, GuaranteeLimitMinor: c.GuaranteeLimitMinor, AssetSupportLimitMinor: c.AssetSupportLimitMinor, State: c.State, FactorScores: factors, Evidence: returned, EvidenceCutoff: c.EvidenceCutoff, EvaluatedAt: c.EvaluatedAt})
	return err
}

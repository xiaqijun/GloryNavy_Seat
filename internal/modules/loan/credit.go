package loan

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"glorynavy.local/seat/internal/modules/loan/internal/store"
)

const creditRuleVersion = "system-v2"

// CreditEvidence is deliberately narrow. Authorization state and account age
// are access/data freshness gates, never score inputs.
type CreditEvidence struct {
	PAPPoints       int
	AssetPoints     int
	AssetValueMinor int64
	PAPAvailable    bool
	AssetAvailable  bool
	PAPSnapshotID   string
	AssetSnapshotID string
	EvidenceCutoff  time.Time
}

type creditPoolConfig struct {
	MaxPrincipal int64 `json:"max_principal_minor"`
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
// security history 10, PAP 5 and asset scale 10. Missing PAP/assets are
// neutral and never negative. Account age and authorization are absent by
// construction.
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
	reason := fmt.Sprintf("system-evaluated-v2: repayment=%d/55 leverage=%d/20 security=%d/10 pap=%d/5 assets=%d/10 settled=%d active=%d defaulted=%d paid_installments=%d/%d overdue=%d", repayment, leverage, security, pap, assets, signals.SettledLoans, signals.ActiveLoans, signals.DefaultedLoans, signals.PaidInstallments, signals.TotalInstallments, signals.OverdueInstallments)
	return store.Credit{
		AccountID: account, Score: &score, TotalLimitMinor: total, UnsecuredLimitMinor: unsecured,
		State: state, RuleVersion: creditRuleVersion, Reason: reason, Version: 1, EvaluatedAt: now,
		SettledLoans: signals.SettledLoans, ActiveLoans: signals.ActiveLoans, DefaultedLoans: signals.DefaultedLoans,
		PaidInstallments: signals.PaidInstallments, TotalInstallments: signals.TotalInstallments, OverdueInstallments: signals.OverdueInstallments,
		RepaymentPoints: repayment, LeveragePoints: leverage, SecurityPoints: security, PAPPoints: pap, AssetPoints: assets,
		Evidence: evidenceSummary(evidence, evidenceCutoff),
	}
}

func safeScaled(value, numerator, denominator int64) int64 {
	if value <= 0 || numerator <= 0 || denominator <= 0 {
		return 0
	}
	if value > math.MaxInt64/numerator {
		return math.MaxInt64 / denominator
	}
	return value * numerator / denominator
}

func evidenceSummary(e CreditEvidence, cutoff time.Time) string {
	b, _ := json.Marshal(map[string]any{
		"pap_available": e.PAPAvailable, "asset_available": e.AssetAvailable,
		"asset_value_minor": e.AssetValueMinor, "pap_snapshot_id": e.PAPSnapshotID,
		"asset_snapshot_id": e.AssetSnapshotID, "evidence_cutoff": cutoff,
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
	maxPrincipal := int64(0)
	if pool, poolErr := store.SharedPool(ctx, s.db(), false); poolErr == nil {
		var cfg creditPoolConfig
		if json.Unmarshal(pool.Config, &cfg) == nil && cfg.MaxPrincipal > 0 {
			maxPrincipal = cfg.MaxPrincipal
		}
	}
	return assessCreditWithEvidence(user, signals, evidence, maxPrincipal, time.Now().UTC()), nil
}

func (s *Service) recordCreditEvaluation(ctx context.Context, tx store.DBTX, c store.Credit) error {
	if c.Score == nil {
		return nil
	}
	factors, _ := json.Marshal(map[string]int{"repayment": c.RepaymentPoints, "leverage": c.LeveragePoints, "security": c.SecurityPoints, "pap": c.PAPPoints, "assets": c.AssetPoints})
	returned, _ := json.Marshal(map[string]any{"reason": c.Reason, "evidence": c.Evidence})
	_, err := store.RecordCreditEvaluation(ctx, tx, store.CreditEvaluation{AccountID: c.AccountID, PolicyVersion: c.RuleVersion, Score: *c.Score, TotalLimitMinor: c.TotalLimitMinor, UnsecuredLimitMinor: c.UnsecuredLimitMinor, State: c.State, FactorScores: factors, Evidence: returned, EvidenceCutoff: c.EvaluatedAt, EvaluatedAt: c.EvaluatedAt})
	return err
}

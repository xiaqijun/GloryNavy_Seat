package loan

import (
	"glorynavy.local/seat/internal/modules/loan/internal/store"
	"testing"
	"time"
)

func TestCreditRejectsStaleSourceAndOldRule(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	policy := defaultCreditPolicy()
	e := CreditEvidence{AssetAvailable: true, AssetValueMinor: 1_000_000, AssetObservedAt: now.Add(-73 * time.Hour), PAPAvailable: true, PAPPoints: 5, PAPObservedAt: now.Add(-73 * time.Hour), Income: IncomeEvidence{Available: true, MonthlyIncomeMinor: 100_000, MonthlyExpenseMinor: 100_000, Samples: 1, ObservedAt: now.Add(-time.Hour)}}
	c := assessCreditWithPolicy("account", store.CreditSignals{}, e, 2_000_000, policy, now)
	if c.TotalLimitMinor != 99_000 || c.AssetSupportLimitMinor != 0 || c.PAPPoints != 0 {
		t.Fatalf("stale sources changed credit: %+v", c)
	}
	if !c.EvidenceCutoff.Equal(now.Add(-time.Hour)) || !creditFresh(c, policy, now) {
		t.Fatalf("fresh income cutoff: %+v", c)
	}
	if creditFresh(c, policy, now.Add(72*time.Hour)) {
		t.Fatal("source older than threshold must expire even with recent assessment")
	}
	c.RuleVersion = "capacity-v6-policy-1"
	if creditFresh(c, policy, now) {
		t.Fatal("old rule must not be accepted")
	}
}

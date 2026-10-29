package loan

import (
	"encoding/json"
	"testing"
	"time"

	"glorynavy.local/seat/internal/modules/loan/internal/store"
)

func TestRepaymentRiskChecksEachInstallmentAndBindsToken(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	p := defaultCreditPolicy()
	e := store.CreditEvaluation{ID: 4, Evidence: json.RawMessage(`{"evidence":"{\"income_available\":true,\"monthly_income_minor\":100000}"}`)}
	c := store.Case{ID: 1, Version: 3, PrincipalMinor: 12000, InterestMinor: 0, TotalDueMinor: 12000, InstallmentCount: 2, IntervalDays: 7}
	r := repaymentRisk(c, e, p, nil, false, now)
	// 7 days at 30% of 100,000/month = 7,000; both installments fit.
	if r.Status != "adequate" {
		t.Fatalf("covered installments: %+v", r)
	}
	other := []store.RepaymentObligation{{CaseID: 9, InstallmentID: 1, DueAt: now.AddDate(0, 0, 3), OutstandingMinor: 2000}}
	withOther := repaymentRisk(c, e, p, other, false, now)
	if withOther.Status != "insufficient" || withOther.TotalDueMinor != 8000 {
		t.Fatalf("existing installment ignored: %+v", withOther)
	}
	if got := repaymentRisk(c, e, p, nil, true, now); got.Status != "unknown" {
		t.Fatalf("unfunded existing schedule: %+v", got)
	}
	c.PrincipalMinor, c.TotalDueMinor = 16000, 16000
	r = repaymentRisk(c, e, p, nil, false, now)
	if r.Status != "insufficient" || r.Days != 7 || r.TotalDueMinor != 8000 || r.EstimatedCapacityMinor != 7000 || r.Token == "" {
		t.Fatalf("first installment risk: %+v", r)
	}
	c.Version++
	if repaymentRisk(c, e, p, nil, false, now).Token == r.Token {
		t.Fatal("case revision must change acknowledgment")
	}
	e.Evidence = json.RawMessage(`{}`)
	if got := repaymentRisk(c, e, p, nil, false, now); got.Status != "unknown" || got.Token == "" {
		t.Fatalf("missing evidence: %+v", got)
	}
}

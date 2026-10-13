package loan

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"glorynavy.local/seat/internal/modules/loan/internal/store"
)

func TestEmptyGuaranteeListUsesArrayJSON(t *testing.T) {
	payload, err := json.Marshal(map[string]any{"items": []store.Guarantee{}})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(payload); got != `{"items":[]}` {
		t.Fatalf("empty guarantee payload=%s, want {\"items\":[]}", got)
	}
}

func TestAssessCreditUsesSystemSignals(t *testing.T) {
	now := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	c := assessCredit("account", store.CreditSignals{SettledLoans: 2, PaidInstallments: 8, TotalInstallments: 10}, 1_000_000, now)
	if c.Score == nil || *c.Score != 77 {
		t.Fatalf("score=%v, want 88", c.Score)
	}
	if c.TotalLimitMinor != 770_000 || c.UnsecuredLimitMinor != 639_100 || c.State != "active" {
		t.Fatalf("credit=%+v", c)
	}
	if !c.EvaluatedAt.Equal(now) || c.RuleVersion != "system-v2" {
		t.Fatalf("evaluation metadata=%+v", c)
	}
	defaulted := assessCredit("account", store.CreditSignals{DefaultedLoans: 1}, 1_000_000, now)
	if defaulted.State != "suspended" || defaulted.Score == nil || *defaulted.Score != 30 {
		t.Fatalf("defaulted credit=%+v", defaulted)
	}
}

func TestAssessCreditAuxiliarySignalsAreCappedAndMissingIsNeutral(t *testing.T) {
	now := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	withEvidence := assessCreditWithEvidence("account", store.CreditSignals{}, CreditEvidence{
		PAPPoints: 99, AssetPoints: 99, PAPAvailable: true, AssetAvailable: true,
		AssetValueMinor: 10_000_000_000 * 100, EvidenceCutoff: now,
	}, 1_000_000, now)
	if withEvidence.Score == nil || *withEvidence.Score != 70 {
		t.Fatalf("score=%v, want capped auxiliary score 70", withEvidence.Score)
	}
	if withEvidence.PAPPoints != 5 || withEvidence.AssetPoints != 10 {
		t.Fatalf("auxiliary points=%d/%d, want 5/10", withEvidence.PAPPoints, withEvidence.AssetPoints)
	}
	missing := assessCreditWithEvidence("account", store.CreditSignals{}, CreditEvidence{}, 1_000_000, now)
	if missing.Score == nil || *missing.Score != 55 || missing.PAPPoints != 0 || missing.AssetPoints != 0 {
		t.Fatalf("missing evidence=%+v, want neutral score 55", missing)
	}
}

func TestAssetScaleScoreUsesTotalPersonalAssetBands(t *testing.T) {
	tests := []struct {
		name  string
		value int64
		want  int
	}{
		{"below first band", 99_999_999 * 100, 0},
		{"one hundred million", 100_000_000 * 100, 2},
		{"five hundred million", 500_000_000 * 100, 4},
		{"one billion", 1_000_000_000 * 100, 6},
		{"five billion", 5_000_000_000 * 100, 8},
		{"ten billion", 10_000_000_000 * 100, 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := assetScaleScore(tt.value); got != tt.want {
				t.Fatalf("assetScaleScore(%d)=%d, want %d", tt.value, got, tt.want)
			}
		})
	}
}

func TestAssessCreditKeepsPAPSplitInEvaluation(t *testing.T) {
	now := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	c := assessCreditWithEvidence("account", store.CreditSignals{}, CreditEvidence{
		PAPPoints: 5, CorporationAttendancePoints: 3, AllianceFulfillmentPoints: 2,
		CorporationAttendanceBPS: 8750, CorporationEligibleEvents: 8, CorporationAttendedEvents: 7,
		AllianceFulfillmentBPS: 6667, AllianceEligibleMonths: 6, AllianceFulfilledMonths: 4,
	}, 1_000_000, now)
	if c.PAPPoints != 5 || c.CorporationAttendancePoints != 3 || c.AllianceFulfillmentPoints != 2 {
		t.Fatalf("pap split=%+v", c)
	}
	if c.CorporationEligibleEvents != 8 || c.CorporationAttendedEvents != 7 || c.AllianceEligibleMonths != 6 || c.AllianceFulfilledMonths != 4 {
		t.Fatalf("pap evidence counts=%+v", c)
	}
}

func TestAssessCreditPolicyUsesIncomeAndPoolCap(t *testing.T) {
	now := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	policy := defaultCreditPolicy()
	c := assessCreditWithPolicy("account", store.CreditSignals{}, CreditEvidence{Income: IncomeEvidence{Available: true, Samples: 3, MonthlyNetIncomeMinor: 100_000, ObservedAt: now}}, 1_000_000, policy, now)
	// 100,000 × 30% × 6 months = 180,000; the score then scales the
	// responsibility limit while the pool remains the hard cash ceiling.
	if c.TotalLimitMinor != 99_000 || c.UnsecuredLimitMinor != 71_280 || !c.IncomeAvailable {
		t.Fatalf("credit=%+v", c)
	}
	poolCapped := assessCreditWithPolicy("account", store.CreditSignals{}, CreditEvidence{Income: IncomeEvidence{Available: true, Samples: 3, MonthlyNetIncomeMinor: 10_000_000, ObservedAt: now}}, 100_000, policy, now)
	if poolCapped.TotalLimitMinor != 55_000 {
		t.Fatalf("pool cap credit=%+v", poolCapped)
	}
}

func TestCoveredByHaircutAvoidsIntermediateOverflow(t *testing.T) {
	if got := coveredByHaircut(9_000_000_000_000_000, 5000); got != 4_500_000_000_000_000 {
		t.Fatalf("coveredByHaircut()=%d", got)
	}
}

func TestSimpleAnnualInterestUsesPolicySnapshot(t *testing.T) {
	got, err := simpleAnnualInterest(100_000_00, 1200, 1, 365)
	if err != nil {
		t.Fatal(err)
	}
	// 100,000 ISK at 12% nominal annual rate for 365 days = 12,000 ISK.
	if got != 1_200_000 {
		t.Fatalf("simpleAnnualInterest()=%d, want %d", got, 1_200_000)
	}
	if got, err := simpleAnnualInterest(100, 1, 1, 1); err != nil || got != 0 {
		t.Fatalf("sub-cent interest=(%d,%v), want 0,nil", got, err)
	}
}

func TestValidLoanIntervalDays(t *testing.T) {
	for _, days := range []int{1, 3, 7, 14, 30} {
		if !validLoanIntervalDays(days) {
			t.Fatalf("validLoanIntervalDays(%d) = false", days)
		}
	}
	for _, days := range []int{0, 2, 5, 10, 31, 365} {
		if validLoanIntervalDays(days) {
			t.Fatalf("validLoanIntervalDays(%d) = true", days)
		}
	}
}

func TestAvailableGuaranteeCapacityUsesTotalResponsibility(t *testing.T) {
	credit := store.Credit{State: "active", TotalLimitMinor: 100_000}
	if got := availableGuaranteeCapacity(credit, 25_000); got != 75_000 {
		t.Fatalf("capacity=%d, want 75000", got)
	}
	if got := availableGuaranteeCapacity(credit, 80_000); got != 20_000 {
		t.Fatalf("capacity=%d, want 20000", got)
	}
	if got := availableGuaranteeCapacity(credit, 100_000); got != 0 {
		t.Fatalf("capacity=%d, want 0", got)
	}
}

func TestRepaymentInstallmentsStartAtFundingInterval(t *testing.T) {
	funded := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	first := funded.AddDate(0, 0, 7)
	rows := repaymentInstallments(100_00, 11_00, 2, 7, first)
	if len(rows) != 2 {
		t.Fatalf("installment count=%d, want 2", len(rows))
	}
	if !rows[0].DueAt.Equal(first) || !rows[1].DueAt.Equal(first.AddDate(0, 0, 7)) {
		t.Fatalf("due dates=%s/%s, want %s/%s", rows[0].DueAt, rows[1].DueAt, first, first.AddDate(0, 0, 7))
	}
	if rows[0].PrincipalMinor+rows[1].PrincipalMinor != 100_00 || rows[0].InterestMinor+rows[1].InterestMinor != 11_00 {
		t.Fatalf("schedule totals=%d/%d, want 10000/1100", rows[0].PrincipalMinor+rows[1].PrincipalMinor, rows[0].InterestMinor+rows[1].InterestMinor)
	}
}

func TestGuaranteeCandidatesExcludeOwnCharactersAndAccountDetails(t *testing.T) {
	s := &Service{
		Characters: func(context.Context, string) ([]Character, error) {
			return []Character{{ID: 1, Name: "Borrower"}}, nil
		},
		GuaranteeCharacters: func(context.Context) ([]Character, error) {
			return []Character{{ID: 1, Name: "Borrower", AccountID: "self"}, {ID: 2, Name: "Guarantor", AccountID: "other", Nickname: "hidden"}}, nil
		},
	}
	items, err := s.GuaranteeCandidates(context.Background(), "self")
	if err != nil || len(items) != 1 || items[0].ID != 2 || items[0].AccountID != "" || items[0].Nickname != "" {
		t.Fatalf("candidates=%+v, err=%v", items, err)
	}
}

func TestCashMatchesCustodyDirections(t *testing.T) {
	base := Contract{Type: "item_exchange", Status: "finished", Completed: "2026-10-06T00:00:00Z", Items: []byte(`[]`), ItemsReady: true, IssuerID: 101, AcceptorID: 202}
	amount := int64(12500)
	tests := []struct {
		name            string
		contract        Contract
		payer, receiver cashParty
		want            bool
	}{
		{"price paid to custodian", func() Contract { c := base; c.Price = "125"; c.Reward = "0"; return c }(), cashParty{"character", 101}, cashParty{"character", 202}, true},
		{"reward paid by custodian", func() Contract { c := base; c.Price = "0"; c.Reward = "125"; return c }(), cashParty{"character", 202}, cashParty{"character", 101}, true},
		{"wrong direction", func() Contract { c := base; c.Price = "125"; c.Reward = "0"; return c }(), cashParty{"character", 202}, cashParty{"character", 101}, false},
		{"items are not cash only", func() Contract {
			c := base
			c.Price = "125"
			c.Reward = "0"
			c.Items = []byte(`[{"type_id":34}]`)
			return c
		}(), cashParty{"character", 101}, cashParty{"character", 202}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cashMatches(tt.contract, tt.payer, tt.receiver, amount); got != tt.want {
				t.Fatalf("cashMatches()=%v, want %v", got, tt.want)
			}
		})
	}
}

func TestCustodyUsesConfiguredCustodianCharacter(t *testing.T) {
	character := int64(101)
	if got := custody(store.Pool{CustodianCharacterID: &character}); got != (cashParty{kind: "character", id: character}) {
		t.Fatalf("custody=%+v, want character %d", got, character)
	}
}

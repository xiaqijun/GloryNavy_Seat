package loan

import (
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
		AssetValueMinor: 9_000_000, EvidenceCutoff: now,
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

func TestCoveredByHaircutAvoidsIntermediateOverflow(t *testing.T) {
	if got := coveredByHaircut(9_000_000_000_000_000, 5000); got != 4_500_000_000_000_000 {
		t.Fatalf("coveredByHaircut()=%d", got)
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

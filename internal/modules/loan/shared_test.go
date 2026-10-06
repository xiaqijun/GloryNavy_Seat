package loan

import (
	"testing"
	"time"

	"glorynavy.local/seat/internal/modules/loan/internal/store"
)

func TestAssessCreditUsesSystemSignals(t *testing.T) {
	now := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	c := assessCredit("account", store.CreditSignals{SettledLoans: 2, PaidInstallments: 8, TotalInstallments: 10}, 1_000_000, now)
	if c.Score == nil || *c.Score != 88 {
		t.Fatalf("score=%v, want 88", c.Score)
	}
	if c.TotalLimitMinor != 880_000 || c.UnsecuredLimitMinor != 774_400 || c.State != "active" {
		t.Fatalf("credit=%+v", c)
	}
	if !c.EvaluatedAt.Equal(now) || c.RuleVersion != "system-v1" {
		t.Fatalf("evaluation metadata=%+v", c)
	}
	defaulted := assessCredit("account", store.CreditSignals{DefaultedLoans: 1}, 1_000_000, now)
	if defaulted.State != "suspended" || defaulted.Score == nil || *defaulted.Score != 25 {
		t.Fatalf("defaulted credit=%+v", defaulted)
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

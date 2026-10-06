package loan

import "testing"

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

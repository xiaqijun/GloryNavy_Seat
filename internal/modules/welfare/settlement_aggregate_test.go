package welfare

import (
	"testing"

	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
)

func TestSettlementContractMatchesFrozenAggregate(t *testing.T) {
	b := store.SettlementBatch{
		SettlementReference: "BATCH-20260930-00000000-0000-4000-8000-000000000000",
		ISKMinor:            5352666651,
		Items:               []byte(`[{"type_id":"34","quantity":"10"}]`),
	}
	c := eve.DeliveryContract{Type: "item_exchange", Title: b.SettlementReference, IssuerID: 987, AssigneeID: 123, Price: "0.00", Reward: "53526666", ItemsReady: true, Items: []eve.DeliveryItem{{TypeID: 34, Quantity: 10, Included: true}}}
	if !settlementContractMatches(b, c, []int64{123, 456}) {
		t.Fatal("whole ISK contract should match the frozen minor amount")
	}
	c.Items[0].Quantity = 9
	if settlementContractMatches(b, c, []int64{123, 456}) {
		t.Fatal("missing one item must not match")
	}
	c.Items[0].Quantity = 10
	c.AssigneeID = 999
	if settlementContractMatches(b, c, []int64{123, 456}) {
		t.Fatal("recipient outside the account group must not match")
	}
}

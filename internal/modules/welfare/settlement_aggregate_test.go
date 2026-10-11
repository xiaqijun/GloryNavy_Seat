package welfare

import (
	"context"
	"testing"

	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
)

func TestSettlementRecipientsUsesCurrentMainCharacter(t *testing.T) {
	s := &Service{MainCharacterID: func(context.Context, string) (int64, error) {
		return 456, nil
	}}
	b := store.SettlementBatch{
		AccountID:    "account-1",
		RecipientIDs: []byte(`[123,456]`),
	}
	got, err := s.settlementRecipients(context.Background(), b)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != 456 {
		t.Fatalf("recipients = %#v, want only main character 456", got)
	}
}

func TestSettlementBatchForReadHidesLegacyAltRecipients(t *testing.T) {
	s := &Service{
		MainCharacterID:   func(context.Context, string) (int64, error) { return 456, nil },
		MainCharacterName: func(context.Context, string) (string, error) { return "Main Pilot", nil },
		Names:             &presentationNames{},
	}
	b := store.SettlementBatch{
		AccountID:           "account-1",
		SettlementReference: "BATCH-20260930-00000000-0000-4000-8000-000000000000",
		Items:               []byte(`[{"type_id":"34","quantity":"10"}]`),
		RecipientIDs:        []byte(`[123,456]`),
	}
	got, err := s.settlementBatchForRead(context.Background(), b)
	if err != nil {
		t.Fatal(err)
	}
	if string(got.RecipientIDs) != `[456]` {
		t.Fatalf("recipient_ids = %s, want [456]", got.RecipientIDs)
	}
	if string(got.RecipientNames) != `["Main Pilot"]` {
		t.Fatalf("recipient_names = %s, want [\"Main Pilot\"]", got.RecipientNames)
	}
	if string(got.Items) != `[{"type_id":"34","quantity":"10","name":"三钛合金"}]` {
		t.Fatalf("items = %s, want localized item name", got.Items)
	}
	if string(b.RecipientIDs) != `[123,456]` {
		t.Fatalf("stored snapshot mutated: %s", b.RecipientIDs)
	}
	if string(b.Items) != `[{"type_id":"34","quantity":"10"}]` {
		t.Fatalf("stored item snapshot mutated: %s", b.Items)
	}
}

func TestSettlementBatchForReadUsesFrozenItemNameWithoutLookup(t *testing.T) {
	names := &presentationNames{}
	s := &Service{Names: names}
	b := store.SettlementBatch{
		Items: []byte(`[{"type_id":"34","quantity":"10","name":"已冻结名称"}]`),
	}
	got, err := s.settlementBatchForRead(context.Background(), b)
	if err != nil {
		t.Fatal(err)
	}
	if names.calls != 0 {
		t.Fatalf("frozen names triggered lookup: %d calls", names.calls)
	}
	if string(got.Items) != string(b.Items) {
		t.Fatalf("frozen item snapshot changed: %s", got.Items)
	}
}

func TestFreezeSettlementItemNamesResolvesOnlyMissingNames(t *testing.T) {
	names := &presentationNames{}
	s := &Service{Names: names}
	reward := SettlementReward{Items: []SettlementItemSummary{
		{TypeID: 34, Quantity: 10},
		{TypeID: 587, Quantity: 1, Name: "已有名称"},
	}}
	s.freezeSettlementItemNames(context.Background(), &reward)
	if names.calls != 1 {
		t.Fatalf("name lookup calls = %d, want 1", names.calls)
	}
	if reward.Items[0].Name != "三钛合金" || reward.Items[1].Name != "已有名称" {
		t.Fatalf("names = %#v", reward.Items)
	}
}

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
	c.Title = b.SettlementReference[:len(b.SettlementReference)-1]
	if !settlementContractMatches(b, c, []int64{123, 456}) {
		t.Fatal("EVE's 50-character title truncation should still match")
	}
	c.Title = b.SettlementReference
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

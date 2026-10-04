package store

import "testing"

func TestSettlementDeliveryStatus(t *testing.T) {
	entries := []SettlementItem{
		{LastError: SettlementAwaitingAcceptanceError},
		{LastError: SettlementAwaitingAcceptanceError},
	}
	if got := SettlementDeliveryStatus(2, entries); got != SettlementAwaitingAcceptance {
		t.Fatalf("awaiting batch status = %q, want %q", got, SettlementAwaitingAcceptance)
	}
	entries[1].LastError = "等待合并合同同步"
	if got := SettlementDeliveryStatus(2, entries); got != "" {
		t.Fatalf("unmatched batch status = %q, want empty", got)
	}
	if got := SettlementDeliveryStatus(2, entries[:1]); got != "" {
		t.Fatalf("incomplete batch status = %q, want empty", got)
	}
}

package store

import (
	"testing"

	"glorynavy.local/seat/internal/platform/reviewqueue"
)

func TestBucketMatchesSourceApprovalStates(t *testing.T) {
	tests := []struct {
		name   string
		item   reviewqueue.Item
		bucket string
	}{
		{name: "submitted", item: reviewqueue.Item{State: "submitted"}, bucket: "pending"},
		{name: "information", item: reviewqueue.Item{State: "information"}, bucket: "information"},
		{name: "waiting contract", item: reviewqueue.Item{State: "approved", Status: "waiting_contract"}, bucket: "fulfillment"},
		{name: "delivery exception", item: reviewqueue.Item{State: "approved", Status: "multiple_contracts"}, bucket: "exceptions"},
		{name: "awaiting acceptance", item: reviewqueue.Item{State: "pending", Status: "awaiting_acceptance"}, bucket: "history"},
		{name: "fulfilled", item: reviewqueue.Item{State: "fulfilled"}, bucket: "history"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := bucket(tt.item); got != tt.bucket {
				t.Fatalf("bucket() = %q, want %q", got, tt.bucket)
			}
		})
	}
}

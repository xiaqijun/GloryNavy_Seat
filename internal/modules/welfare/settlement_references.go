package welfare

import (
	"context"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
)

func settlementMatchesAnyReference(b store.SettlementBatch, c eve.DeliveryContract, recipients []int64, references []string) bool {
	for _, ref := range references {
		b.SettlementReference = ref
		if settlementContractMatches(b, c, recipients) {
			return true
		}
	}
	return false
}

func (s *Service) recordSettlementProgress(ctx context.Context, b store.SettlementBatch, contract *eve.DeliveryContract, detail string) error {
	var id, recipient *int64
	if contract != nil {
		id, recipient = &contract.ID, &contract.AssigneeID
	}
	return store.RecordSettlementProgress(ctx, s.Pool, b.ID, id, recipient, detail)
}

func settlementDeadMessage(status string) string {
	switch status {
	case "deleted":
		return store.SettlementDeletedError
	case "cancelled":
		return store.SettlementCancelledError
	case "rejected":
		return store.SettlementRejectedError
	default:
		return store.SettlementFailedError
	}
}

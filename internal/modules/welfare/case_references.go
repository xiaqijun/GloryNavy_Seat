package welfare

import (
	"context"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
	"slices"
	"strings"
)

// Lookup both names, retaining original contract evidence. Multiple candidates
// remain ambiguous; aliases never weaken the normal payment or cancellation guards.
func (s *Service) casePaymentContracts(ctx context.Context, tx pgx.Tx, v Case, recipient int64) ([]eve.DeliveryContract, []string, error) {
	var db store.DB = s.Pool
	if tx != nil {
		db = tx
	}
	aliases, err := store.CaseReferenceAliases(ctx, db, v.ID, v.Reference)
	if err != nil {
		return nil, nil, err
	}
	refs := append([]string{v.Reference}, aliases...)
	out := []eve.DeliveryContract{}
	for _, ref := range refs {
		rows, err := s.PaymentContracts(ctx, tx, v.AccountID, recipient, ref, v.CreatedAt)
		if err != nil {
			return nil, nil, err
		}
		out = append(out, rows...)
	}
	return out, refs, nil
}

// Only a recorded exact alias can supply the comparison reference. The Case
// value is copied, and the observed contract title/content token are unchanged.
func caseForContractReference(v Case, refs []string, title string) Case {
	title = strings.TrimSpace(title)
	if slices.Contains(refs, title) {
		v.Reference = title
	}
	return v
}

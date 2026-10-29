package eve

import (
	"context"
	"time"

	"glorynavy.local/seat/internal/modules/eve/internal/store"
)

type CreditWalletSummary struct {
	OwnerID         int64
	Income, Expense string
	ObservedAt      time.Time
}

type CreditWalletOwner struct {
	ID        int64
	OwnerHash []byte
}

// CreditWalletSummaries is for an already-authorized set of characters on one
// account. Unlike the wallet page it excludes transfers between those characters.
func (s *AuthorizationService) CreditWalletSummaries(ctx context.Context, owners []CreditWalletOwner, from time.Time) ([]CreditWalletSummary, error) {
	bindings := make([]store.CreditWalletOwner, 0, len(owners))
	for _, owner := range owners {
		bindings = append(bindings, store.CreditWalletOwner{ID: owner.ID, OwnerHash: owner.OwnerHash})
	}
	rows, err := store.ReadCreditWalletSummaries(ctx, s.pool, bindings, from)
	if err != nil {
		return nil, err
	}
	out := make([]CreditWalletSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, CreditWalletSummary{OwnerID: row.OwnerID, Income: row.Income, Expense: row.Expense, ObservedAt: row.ObservedAt})
	}
	return out, nil
}

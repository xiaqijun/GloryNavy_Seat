package eve

import (
	"context"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
)

// Host supplies current account bindings and gates administrator-only projections.
func (s *AuthorizationService) AuthorizationReminderKeys(ctx context.Context, id int64, owner []byte) ([]string, error) {
	return store.AuthorizationReminderKeys(ctx, s.pool, id, owner)
}
func (s *AuthorizationService) SyncReminderKeys(ctx context.Context) ([]string, error) {
	return store.SyncReminderKeys(ctx, s.pool)
}
func (h *ContractHTTP) ContractReminderKeys(ctx context.Context, account string, id int64, owner []byte) ([]string, error) {
	if h.LookupOwner == nil {
		return []string{}, nil
	}
	if _, err := h.LookupOwner(ctx, account, "character", id); err != nil {
		return nil, err
	}
	return store.ContractReminderKeys(ctx, h.pool, id, owner)
}

package eve

import (
	"context"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
	"glorynavy.local/seat/internal/platform/remindermessage"
)

func (s *AuthorizationService) AuthorizationReminderDetails(ctx context.Context, id int64, owner []byte) ([]remindermessage.Item, error) {
	keys, err := s.AuthorizationReminderKeys(ctx, id, owner)
	if err != nil {
		return nil, err
	}
	return store.ReminderDetails(ctx, s.pool, "authorization_expired", keys, id)
}
func (s *AuthorizationService) SyncReminderDetails(ctx context.Context) ([]remindermessage.Item, error) {
	keys, err := s.SyncReminderKeys(ctx)
	if err != nil {
		return nil, err
	}
	return store.ReminderDetails(ctx, s.pool, "sync_anomaly", keys, 0)
}
func (h *ContractHTTP) ContractReminderDetails(ctx context.Context, account string, id int64, owner []byte) ([]remindermessage.Item, error) {
	keys, err := h.ContractReminderKeys(ctx, account, id, owner)
	if err != nil {
		return nil, err
	}
	return store.ReminderDetails(ctx, h.pool, "contract_pending", keys, id)
}

package exchange

import (
	"context"
	"glorynavy.local/seat/internal/modules/exchange/internal/store"
	"time"
)

// ReminderKeys is a recipient-scoped, local-only notification projection.
func (s *Service) ReminderKeys(ctx context.Context, account, kind string, since time.Time) ([]string, error) {

	return store.ReminderKeys(ctx, s.Pool, account, kind, since)
}

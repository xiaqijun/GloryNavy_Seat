package sentry

import (
	"context"
	"glorynavy.local/seat/internal/modules/sentry/internal/store"
	"glorynavy.local/seat/internal/platform/remindermessage"
	"time"
)

func (s *Service) ReminderDetails(ctx context.Context, account, kind string, since time.Time) ([]remindermessage.Item, error) {
	keys, err := s.ReminderKeys(ctx, account, kind, since)
	if err != nil {
		return nil, err
	}
	return store.ReminderDetails(ctx, s.Pool, kind, keys)
}

package feedback

import (
	"context"
	"glorynavy.local/seat/internal/modules/feedback/internal/store"
	"time"
)

// ReminderKeys is a recipient-scoped, local-only notification projection.
func (s *Service) ReminderKeys(ctx context.Context, account, kind string, since time.Time) ([]string, error) {
	if kind == "feedback_pending" {
		if s.Administrator == nil {
			return []string{}, nil
		}
		ok, err := s.Administrator(ctx, account)
		if err != nil {
			return nil, err
		}
		if !ok {
			return []string{}, nil
		}
	}
	return store.ReminderKeys(ctx, s.Pool, account, kind, since)
}

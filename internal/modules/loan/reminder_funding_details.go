package loan

import (
	"context"
	"glorynavy.local/seat/internal/modules/loan/internal/store"
	"glorynavy.local/seat/internal/platform/remindermessage"
	"time"
)

func (s *Service) FundingReminderDetails(ctx context.Context, account string, since time.Time) ([]remindermessage.Item, error) {
	keys, err := s.FundingReminderKeys(ctx, account, since)
	if err != nil {
		return nil, err
	}
	return store.ReminderDetails(ctx, s.Pool, "loan_funding", keys)
}

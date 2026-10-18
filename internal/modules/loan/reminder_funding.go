package loan

import (
	"context"
	"glorynavy.local/seat/internal/modules/loan/internal/store"
	"time"
)

func (s *Service) FundingReminderKeys(ctx context.Context, account string, _ time.Time) ([]string, error) {
	pools, err := store.ListAllPools(ctx, s.db())
	if err != nil {
		return nil, err
	}
	ids := []int64{}
	for _, p := range pools {
		ok, e := s.canManage(ctx, account, p)
		if e != nil {
			return nil, e
		}
		if ok {
			ids = append(ids, p.ID)
		}
	}
	return store.FundingReminderKeys(ctx, s.Pool, account, ids)
}

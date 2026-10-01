package exchange

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"glorynavy.local/seat/internal/modules/exchange/internal/store"
	"glorynavy.local/seat/internal/platform/jobs"
	"time"
)

type priceArgs struct {
	RewardID int64 `json:"reward_id"`
}

func (priceArgs) Kind() string { return "exchange.reward-price.v1" }

type priceWorker struct {
	river.WorkerDefaults[priceArgs]
	runtime *DeliveryJobs
}

func (w *priceWorker) Work(ctx context.Context, j *river.Job[priceArgs]) error {
	if !w.runtime.Enabled || w.runtime.Service == nil {
		return river.JobSnooze(time.Hour)
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	return w.runtime.Service.RefreshRewardPrice(ctx, j.Args.RewardID)
}
func (r *DeliveryJobs) scanPrices(ctx context.Context) error {
	ids, err := store.New(r.Service.Pool).DuePrices(ctx)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err = r.queue.Insert(ctx, priceArgs{id}, &river.InsertOpts{Queue: "exchange", UniqueOpts: jobs.ActiveUnique(), MaxAttempts: 3}); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) RefreshRewardPrice(ctx context.Context, id int64) error {
	q := store.New(s.Pool)
	snapshot, err := q.PriceSnapshot(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var content PhysicalReward
	if err = json.Unmarshal(snapshot.Content, &content); err != nil {
		return err
	}
	quote, quoteErr := s.valueContent(ctx, content)
	status := "ready"
	if quoteErr != nil {
		status = "failed"
	} else if !quote.Complete {
		status = "incomplete"
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	q = store.New(tx)
	current, err := q.LockReward(ctx, id)
	if err != nil {
		return err
	}
	if current.Archived || current.Version != snapshot.Version {
		return nil
	}
	if err = q.PublishPrice(ctx, snapshot, quote.Value, status, quote.ObservedAt); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

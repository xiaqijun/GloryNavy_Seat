package feedback

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/feedback/internal/store"
)

type RewardSettings = store.RewardSettings
type RewardConfig struct {
	Amount     int64  `json:"amount_minor"`
	Version    int64  `json:"version"`
	RequestKey string `json:"request_key"`
}

func (s *Service) RewardSettings(ctx context.Context, actor string) (RewardSettings, error) {
	ok, err := s.Administrator(ctx, actor)
	if err != nil {
		return RewardSettings{}, err
	}
	if !ok {
		return RewardSettings{}, forbidden
	}
	return store.ReadRewardSettings(ctx, s.Pool, false)
}
func (s *Service) ConfigureReward(ctx context.Context, actor string, in RewardConfig) (RewardSettings, error) {
	if !validKey(in.RequestKey) || in.Version < 1 || in.Amount < 0 || in.Amount > 1000000000000 {
		return RewardSettings{}, invalid
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return RewardSettings{}, err
	}
	defer tx.Rollback(context.Background())
	if err = s.LockAccounts(ctx, tx, []string{actor}); err != nil {
		return RewardSettings{}, err
	}
	ok, err := s.Administrator(ctx, actor)
	if err != nil {
		return RewardSettings{}, err
	}
	if !ok {
		return RewardSettings{}, forbidden
	}
	cfg, err := store.ReadRewardSettings(ctx, tx, true)
	if err != nil {
		return cfg, err
	}
	old, err := store.RewardConfigReplay(ctx, tx, actor, in.RequestKey)
	if err == nil {
		if old.Amount != in.Amount {
			return cfg, changed
		}
		return old, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return cfg, err
	}
	if cfg.Version != in.Version {
		return cfg, changed
	}
	ok, err = s.Administrator(ctx, actor)
	if err != nil {
		return cfg, err
	}
	if !ok {
		return cfg, forbidden
	}
	cfg, err = store.SaveRewardSettings(ctx, tx, actor, in.RequestKey, in.Amount)
	if err != nil {
		return cfg, err
	}
	return cfg, tx.Commit(ctx)
}
func (s *Service) adoptReward(ctx context.Context, tx pgx.Tx, actor string, id int64, t Ticket) error {
	if actor == t.AccountID {
		return &Fault{403, "feedback_self_accept", "不能采纳自己的反馈"}
	}
	if t.AcceptedAt != nil {
		return nil
	}
	cfg, err := store.ReadRewardSettings(ctx, tx, true)
	if err != nil {
		return err
	}
	if cfg.Amount > 0 {
		if s.Credit == nil {
			return errors.New("feedback reward service unavailable")
		}
		if err = s.Credit(ctx, tx, actor, t.AccountID, id, cfg.Amount); err != nil {
			return err
		}
	}
	ok, err := s.Administrator(ctx, actor)
	if err != nil {
		return err
	}
	if !ok {
		return forbidden
	}
	return store.AdoptReward(ctx, tx, id, actor, cfg.Amount)
}

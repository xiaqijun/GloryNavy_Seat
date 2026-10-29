package loan

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/loan/internal/store"
)

func (s *Service) CreditPolicy(ctx context.Context) (store.CreditPolicy, error) {
	return store.ReadCreditPolicy(ctx, s.db())
}

func validCreditPolicy(p store.CreditPolicy) bool {
	return p.Version > 0 && p.IncomeWindowDays >= 30 && p.IncomeWindowDays <= 730 &&
		p.AnnualRateBPS >= 0 && p.AnnualRateBPS <= 10000 &&
		p.DebtRatioBPS >= 0 && p.DebtRatioBPS <= 10000 && p.TermMonths >= 1 && p.TermMonths <= 60 &&
		p.ScoreFactorBPS >= 0 && p.ScoreFactorBPS <= 20000 && p.IncomeWeightBPS >= 0 && p.IncomeWeightBPS <= 10000 && p.AssetCreditLTVBPS >= 0 && p.AssetCreditLTVBPS <= 10000 &&
		p.GuaranteeCapBPS >= 0 && p.GuaranteeCapBPS <= 10000 && p.CollateralCapBPS >= 0 && p.CollateralCapBPS <= 10000 &&
		p.BorrowerLimitBPS >= 0 && p.BorrowerLimitBPS <= 10000 &&
		p.StaleAfterHours >= 1 && p.StaleAfterHours <= 8760 && p.MinIncomeSamples >= 0 && p.MinIncomeSamples <= 12 &&
		(p.InsufficientIncomePolicy == "fallback_pool" || p.InsufficientIncomePolicy == "zero")
}

func (s *Service) UpdateCreditPolicy(ctx context.Context, actor string, p store.CreditPolicy) (store.CreditPolicy, error) {
	if !validCreditPolicy(p) {
		return store.CreditPolicy{}, ErrInvalid
	}
	pool, err := store.SharedPool(ctx, s.db(), false)
	if err != nil {
		return store.CreditPolicy{}, err
	}
	allowed, err := s.canManage(ctx, actor, pool)
	if err != nil {
		return store.CreditPolicy{}, err
	}
	if !allowed {
		return store.CreditPolicy{}, ErrForbidden
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return store.CreditPolicy{}, err
	}
	defer tx.Rollback(context.Background())
	allowed, err = s.canManage(ctx, actor, pool)
	if err != nil || !allowed {
		if err != nil {
			return store.CreditPolicy{}, err
		}
		return store.CreditPolicy{}, ErrForbidden
	}
	out, err := store.SaveCreditPolicy(ctx, tx, actor, p)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.CreditPolicy{}, ErrConflict
	}
	if err != nil {
		return store.CreditPolicy{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return store.CreditPolicy{}, err
	}
	return out, nil
}

func (s *Service) CreditEvaluations(ctx context.Context, actor string) ([]store.CreditEvaluation, error) {
	return store.CreditEvaluations(ctx, s.db(), actor, 50)
}

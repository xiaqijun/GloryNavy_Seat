package loan

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/loan/internal/store"
)

const collateralAdvanceBPS = 8000

var (
	ErrNoSecurityNeeded = errors.New("no unallocated excess needs security")
	ErrCollateralValue  = errors.New("collateral value after 80 percent advance is insufficient")
	ErrCollateralQuote  = errors.New("collateral valuation unavailable")
)

type SecuritySummary struct {
	CreditReady          bool  `json:"credit_ready"`
	AvailableCreditMinor int64 `json:"available_credit_minor"`
	RequiredMinor        int64 `json:"required_minor"`
	EffectiveMinor       int64 `json:"effective_minor"`
	PendingMinor         int64 `json:"pending_minor"`
	UnallocatedMinor     int64 `json:"unallocated_minor"`
	LegacyPending        bool  `json:"legacy_pending"`
}

// Only call after case-object authorization. Mutation callers take the account
// credit lock before the case lock; this is the same order as loan approval.
func (s *Service) caseSecurity(ctx context.Context, db store.DBTX, c store.Case, excludeGuarantee int64) (SecuritySummary, error) {
	var out SecuritySummary
	evaluation, err := store.LatestCreditEvaluation(ctx, db, c.BorrowerAccountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	policy, err := store.ReadCreditPolicy(ctx, db)
	if err != nil {
		return out, err
	}
	credit := creditFromEvaluation(evaluation)
	if credit.State != "active" || !creditFresh(credit, policy, time.Now().UTC()) {
		return out, nil
	}
	used, err := store.UsedResponsibility(ctx, db, c.BorrowerAccountID)
	if err != nil {
		return out, err
	}
	out.CreditReady = true
	out.AvailableCreditMinor = max(0, credit.TotalLimitMinor-used)
	out.RequiredMinor = requiredCreditCoverage(credit.TotalLimitMinor, used, c.PrincipalMinor)
	out.EffectiveMinor, out.PendingMinor, out.LegacyPending, err = store.SecurityCommitments(ctx, db, c.ID, c.PrincipalMinor, excludeGuarantee)
	if err != nil {
		return out, err
	}
	out.UnallocatedMinor = max(0, max(0, out.RequiredMinor-out.EffectiveMinor)-out.PendingMinor)
	return out, nil
}

func securityTarget(summary SecuritySummary, expected int64) (int64, error) {
	if !summary.CreditReady {
		return 0, ErrRule
	}
	if summary.LegacyPending {
		return 0, ErrConflict
	}
	if summary.UnallocatedMinor <= 0 {
		return 0, ErrNoSecurityNeeded
	}
	if expected < 0 || expected > 0 && expected != summary.UnallocatedMinor {
		return 0, ErrConflict
	}
	return summary.UnallocatedMinor, nil
}

// allocation is an explicit partial amount; expected remains the previously
// displayed entire gap so stale clients cannot silently allocate against a
// changed snapshot. Zero keeps the original full-gap behavior.
func securityAllocation(summary SecuritySummary, expected, allocation int64) (int64, error) {
	gap, err := securityTarget(summary, expected)
	if err != nil {
		return 0, err
	}
	if allocation < 0 || allocation > gap {
		return 0, ErrConflict
	}
	if allocation == 0 {
		return gap, nil
	}
	return allocation, nil
}

package welfare

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
)

var ErrLossWeeklyConfirmation = fmt.Errorf("weekly confirmation required: %w", ErrLossQuota)
var ErrLossMonthlyConfirmation = fmt.Errorf("monthly confirmation required: %w", ErrLossQuota)
var ErrLossBeforeStart = fmt.Errorf("loss predates rolling application window: %w", ErrRule)

type LossQuotaReview struct {
	PolicyVersion int64     `json:"policy_version,string"`
	CaseVersion   int64     `json:"case_version,string"`
	CheckedAt     time.Time `json:"checked_at"`
	Quota         LossQuota `json:"quota"`
}

type LossQuotaApproval struct {
	Level  string          `json:"level"`
	Amount int64           `json:"amount_minor"`
	Review LossQuotaReview `json:"review"`
}

// Both the review and the locked approval use the claimant's current usage,
// excluding this application from pending reservations.
func (s *Service) lossQuotaReview(ctx context.Context, db store.DB, v Case, cfg Config) (LossQuotaReview, error) {
	var out LossQuotaReview
	out.CaseVersion = v.Version
	if err := db.QueryRow(ctx, `SELECT now()`).Scan(&out.CheckedAt); err != nil {
		return out, err
	}
	day, nextDay, week, nextWeek, month, nextMonth := lossQuotaPeriods(out.CheckedAt)
	totals, err := store.ApprovedLossTotals(ctx, db, v.AccountID, v.CorporationID, day, nextDay, week, nextWeek, month, nextMonth)
	if err != nil {
		return out, err
	}
	occupied, err := store.LossQuotaDays(ctx, db, v.AccountID, v.CorporationID, quotaDay(out.CheckedAt))
	if err != nil {
		return out, err
	}
	pending, err := store.PendingLosses(ctx, db, v.AccountID, v.CorporationID)
	if err != nil {
		return out, err
	}
	reserved := [3]int64{}
	for _, candidate := range pending {
		if candidate.ID == v.ID || candidate.Kind != v.Kind {
			continue
		}
		amount := pendingLossAward(candidate, cfg)
		for i, p := range [][2]time.Time{{day, nextDay}, {week, nextWeek}, {month, nextMonth}} {
			if !candidate.CreatedAt.Before(p[0]) && candidate.CreatedAt.Before(p[1]) {
				reserved[i] = quotaAdd(reserved[i], amount)
			}
		}
	}
	daily, future := dailyUsage(occupied, v.Kind, quotaDay(out.CheckedAt))
	out.Quota = LossQuota{Kind: v.Kind, Daily: quotaPeriod(daily, reserved[0], cfg.LossDailyCapMinor), Weekly: quotaPeriod(totals[v.Kind].Weekly, reserved[1], cfg.LossWeeklyCapMinor), Monthly: quotaPeriod(totals[v.Kind].Monthly, reserved[2], cfg.LossMonthlyCapMinor), FutureUsedMinor: future}
	return out, nil
}

func lossQuotaLevel(q LossQuota, amount int64) (string, error) {
	if amount <= 0 || amount > 100000000000000 {
		return "", ErrInvalid
	}
	exceeds := func(p LossQuotaPeriod) bool { return p.RemainingMinor != nil && amount > *p.RemainingMinor }
	if exceeds(q.Monthly) {
		return "", ErrLossMonthlyQuota
	}
	if exceeds(q.Weekly) {
		return "monthly", nil
	}
	if exceeds(q.Daily) {
		return "weekly", nil
	}
	return "", nil
}

func (s *Service) ReviewLossQuota(ctx context.Context, actor string, id int64) (LossQuotaReview, error) {
	v, err := s.Read(ctx, actor, id)
	if err != nil {
		return LossQuotaReview{}, err
	}
	if !cashLoss(v.Kind) || actor == v.AccountID {
		return LossQuotaReview{}, pgx.ErrNoRows
	}
	if err = s.allowedLoss(ctx, actor, v.CorporationID, true); err != nil {
		return LossQuotaReview{}, err
	}
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return LossQuotaReview{}, err
	}
	defer tx.Rollback(ctx)
	current, err := store.Read(ctx, tx, id)
	if err != nil {
		return LossQuotaReview{}, err
	}
	if current.AccountID != v.AccountID || current.CorporationID != v.CorporationID {
		return LossQuotaReview{}, ErrConflict
	}
	p, cfg, err := s.lossPolicy(ctx, tx, v.CorporationID, v.Kind)
	if err != nil {
		return LossQuotaReview{}, err
	}
	out, err := s.lossQuotaReview(ctx, tx, current, cfg)
	out.PolicyVersion = p.Version
	return out, err
}

func (h Handler) lossQuotaReview(w http.ResponseWriter, r *http.Request) {
	id := number(chi.URLParam(r, "id"))
	if id == 0 {
		respond(w, r, nil, ErrInvalid)
		return
	}
	v, err := h.Service.ReviewLossQuota(r.Context(), h.User(r), id)
	w.Header().Set("Cache-Control", "private, no-store")
	respond(w, r, v, err)
}

func checkLossStart(cfg Config, occurred string, now time.Time) error {
	days := int64(30)
	if cfg.LossApplicationDays != nil {
		days = *cfg.LossApplicationDays
	}
	if days < 1 || days > 3650 {
		return ErrRule
	}
	at, err := time.Parse(time.RFC3339, occurred)
	if err != nil || at.After(now) {
		return ErrRule
	}
	if at.Before(now.Add(-time.Duration(days) * 24 * time.Hour)) {
		return ErrLossBeforeStart
	}
	return nil
}

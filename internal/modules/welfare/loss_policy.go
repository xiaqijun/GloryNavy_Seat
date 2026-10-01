package welfare

import (
	"context"
	"encoding/json"
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
	"time"
)

func cashLoss(kind string) bool { return kind == "srp" || kind == "solo" }

func lossQuotaPeriods(now time.Time) (dayStart, dayEnd, weekStart, weekEnd, monthStart, monthEnd time.Time) {
	zone := time.FixedZone("CST", 8*60*60)
	local := now.In(zone)
	dayStart = time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, zone)
	dayEnd = dayStart.AddDate(0, 0, 1)
	weekStart = dayStart.AddDate(0, 0, -((int(dayStart.Weekday()) + 6) % 7))
	weekEnd = weekStart.AddDate(0, 0, 7)
	monthStart = time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, zone)
	monthEnd = monthStart.AddDate(0, 1, 0)
	return
}

func quotaPeriod(used, reserved int64, cap *int64) LossQuotaPeriod {
	p := LossQuotaPeriod{UsedMinor: used, ReservedMinor: reserved, CapMinor: cap}
	if cap != nil && *cap > 0 {
		remaining := *cap - used - reserved
		if remaining < 0 {
			remaining = 0
		}
		p.RemainingMinor = &remaining
	} else {
		p.CapMinor = nil
	}
	return p
}

func pendingLossAward(v store.PendingLoss, cfg Config) int64 {
	if v.Amount > 0 {
		return v.Amount
	}
	base := v.BaseMinor
	if v.Kind == "solo" && v.ValuationMinor > 0 {
		base = v.ValuationMinor
	}
	amount, _ := lossAward(base, cfg)
	return amount
}

// LossQuotas is the member-facing projection of the same quota calculation
// used by approval. It intentionally exposes only the selected corporation.
func (s *Service) LossQuotas(ctx context.Context, actor string, corp int64, policies []Policy) ([]LossQuota, error) {
	var now time.Time
	if err := s.Pool.QueryRow(ctx, `SELECT now()`).Scan(&now); err != nil {
		return nil, err
	}
	dayStart, dayEnd, weekStart, weekEnd, monthStart, monthEnd := lossQuotaPeriods(now)
	usage, err := store.ApprovedLossTotals(ctx, s.Pool, actor, corp, dayStart, dayEnd, weekStart, weekEnd, monthStart, monthEnd)
	if err != nil {
		return nil, err
	}
	configs := map[string]Config{}
	for _, p := range policies {
		if !cashLoss(p.Kind) {
			continue
		}
		var c Config
		if err := json.Unmarshal(p.Config, &c); err != nil {
			return nil, err
		}
		configs[p.Kind] = c
	}
	pending, err := store.PendingLosses(ctx, s.Pool, actor, corp)
	if err != nil {
		return nil, err
	}
	reserved := map[string][3]int64{}
	for _, v := range pending {
		c := configs[v.Kind]
		amount := pendingLossAward(v, c)
		if amount <= 0 {
			continue
		}
		periods := reserved[v.Kind]
		if !v.CreatedAt.Before(dayStart) && v.CreatedAt.Before(dayEnd) {
			periods[0] += amount
		}
		if !v.CreatedAt.Before(weekStart) && v.CreatedAt.Before(weekEnd) {
			periods[1] += amount
		}
		if !v.CreatedAt.Before(monthStart) && v.CreatedAt.Before(monthEnd) {
			periods[2] += amount
		}
		reserved[v.Kind] = periods
	}
	out := make([]LossQuota, 0, 2)
	for _, kind := range []string{"srp", "solo"} {
		c := configs[kind]
		u := usage[kind]
		r := reserved[kind]
		out = append(out, LossQuota{
			Kind:    kind,
			Daily:   quotaPeriod(u.Daily, r[0], c.LossDailyCapMinor),
			Weekly:  quotaPeriod(u.Weekly, r[1], c.LossWeeklyCapMinor),
			Monthly: quotaPeriod(u.Monthly, r[2], c.LossMonthlyCapMinor),
		})
	}
	return out, nil
}

// Unconfigured reimbursements retain the existing 100%, uncapped amount.
func (s *Service) lossPolicy(ctx context.Context, db store.DB, corp int64, kind string) (Policy, Config, error) {
	rows, err := store.Policies(ctx, db, corp)
	if err != nil {
		return Policy{}, Config{}, err
	}
	for _, p := range rows {
		if p.Kind == kind {
			var cfg Config
			err = json.Unmarshal(p.Config, &cfg)
			return p, cfg, err
		}
	}
	return Policy{}, Config{}, nil
}

// The welfare publication lock serializes approvals, so the usage check and
// the new award are committed atomically without a separate quota ledger.
func (s *Service) checkLossQuota(ctx context.Context, db store.DB, v Case, cfg Config) error {
	if (cfg.LossDailyCapMinor == nil || *cfg.LossDailyCapMinor == 0) &&
		(cfg.LossWeeklyCapMinor == nil || *cfg.LossWeeklyCapMinor == 0) &&
		(cfg.LossMonthlyCapMinor == nil || *cfg.LossMonthlyCapMinor == 0) {
		return nil
	}
	var now time.Time
	if err := db.QueryRow(ctx, `SELECT now()`).Scan(&now); err != nil {
		return err
	}
	day, nextDay, week, nextWeek, month, nextMonth := lossQuotaPeriods(now)
	periods := []struct {
		cap        *int64
		start, end time.Time
		exceeded   error
	}{
		{cfg.LossDailyCapMinor, day, nextDay, ErrLossDailyQuota},
		{cfg.LossWeeklyCapMinor, week, nextWeek, ErrLossWeeklyQuota},
		{cfg.LossMonthlyCapMinor, month, nextMonth, ErrLossMonthlyQuota},
	}
	pending, err := store.PendingLosses(ctx, db, v.AccountID, v.CorporationID)
	if err != nil {
		return err
	}
	for _, p := range periods {
		if p.cap == nil || *p.cap == 0 {
			continue
		}
		used, err := store.ApprovedLossTotal(ctx, db, v.AccountID, v.CorporationID, v.Kind, p.start, p.end)
		if err != nil {
			return err
		}
		for _, candidate := range pending {
			if candidate.ID == v.ID || candidate.Kind != v.Kind || candidate.CreatedAt.Before(p.start) || !candidate.CreatedAt.Before(p.end) {
				continue
			}
			var pendingConfig Config
			if candidate.Kind == v.Kind {
				pendingConfig = cfg
			}
			used += pendingLossAward(candidate, pendingConfig)
		}
		if v.Award > *p.cap || used > *p.cap-v.Award {
			return p.exceeded
		}
	}
	return nil
}

func lossAward(base int64, cfg Config) (int64, error) {
	if base <= 0 || base > 100000000000000 || validateConfig("solo", cfg) != nil {
		return 0, ErrInvalid
	}
	rate := int64(10000)
	if cfg.LossRateBPS != nil {
		rate = *cfg.LossRateBPS
	}
	amount := base/10000*rate + (base%10000*rate)/10000
	if cfg.LossCapMinor != nil && *cfg.LossCapMinor > 0 {
		amount = min(amount, *cfg.LossCapMinor)
	}
	if amount < 100 {
		return 0, ErrInvalid
	}
	return amount, nil
}

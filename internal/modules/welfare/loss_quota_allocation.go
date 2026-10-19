package welfare

import (
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
	"math"
	"slices"
	"time"
)

func quotaDay(now time.Time) int64 { return (now.Unix() + 8*60*60) / 86400 }
func quotaCap(cap *int64) int64 {
	if cap == nil {
		return 0
	}
	return *cap
}
func dailyAdvance(cfg Config) bool {
	return quotaCap(cfg.LossDailyCapMinor) > 0 && quotaCap(cfg.LossWeeklyCapMinor) == 0 && quotaCap(cfg.LossMonthlyCapMinor) == 0
}

func quotaAdd(a, b int64) int64 {
	if b > 0 && a > math.MaxInt64-b {
		return math.MaxInt64
	}
	return a + b
}

func dailyUsage(spans []store.LossQuotaSpan, kind string, day int64) (today, future int64) {
	for _, v := range spans {
		if v.Kind != kind {
			continue
		}
		if v.Start <= day && v.End > day {
			today = quotaAdd(today, v.Amount)
		}
		if n := v.End - max(v.Start, day+1); n > 0 {
			future = quotaAdd(future, n*v.Amount)
		}
	}
	return
}

// Allocate earliest available days. Sweep boundaries instead of iterating days:
// even the maximum award with a 1 ISK daily cap takes a bounded number of spans.
func allocateDailyQuota(kind string, day, amount, cap int64, occupied []store.LossQuotaSpan) ([]store.LossQuotaSpan, error) {
	if day < 0 || amount <= 0 || amount > 100000000000000 || cap <= 0 {
		return nil, ErrInvalid
	}
	events := map[int64]int64{day: 0}
	for _, v := range occupied {
		if v.Kind != kind || v.End <= day {
			continue
		}
		if v.Amount <= 0 || v.End <= v.Start {
			return nil, ErrInvalid
		}
		start := max(v.Start, day)
		// Fail closed on corrupted/overflowing usage rather than freeing capacity.
		if events[start] > math.MaxInt64-v.Amount || events[v.End] < math.MinInt64+v.Amount {
			return nil, ErrInvalid
		}
		events[start] += v.Amount
		events[v.End] -= v.Amount
	}
	points := make([]int64, 0, len(events))
	for point := range events {
		points = append(points, point)
	}
	slices.Sort(points)
	out := []store.LossQuotaSpan{}
	used := int64(0)
	for i, start := range points {
		delta := events[start]
		if delta > 0 && used > math.MaxInt64-delta {
			return nil, ErrInvalid
		}
		used += delta
		if used < 0 {
			return nil, ErrInvalid
		}
		available := cap - used
		if available <= 0 {
			continue
		}
		full := amount / available
		length := full + 1
		if i+1 < len(points) {
			length = points[i+1] - start
		}
		take := min(full, length)
		if take > 0 {
			out = append(out, store.LossQuotaSpan{Kind: kind, Start: start, End: start + take, Amount: available})
			amount -= take * available
		}
		if amount == 0 {
			return out, nil
		}
		if take < length {
			out = append(out, store.LossQuotaSpan{Kind: kind, Start: start + take, End: start + take + 1, Amount: amount})
			return out, nil
		}
	}
	return nil, ErrInvalid
}

func planDailyQuota(kind string, day, amount int64, cfg Config, occupied []store.LossQuotaSpan) ([]store.LossQuotaSpan, error) {
	if dailyAdvance(cfg) {
		return allocateDailyQuota(kind, day, amount, quotaCap(cfg.LossDailyCapMinor), occupied)
	}
	return []store.LossQuotaSpan{{Kind: kind, Start: day, End: day + 1, Amount: amount}}, nil
}

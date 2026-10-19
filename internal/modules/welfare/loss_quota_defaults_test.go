package welfare

import (
	"context"
	"encoding/json"
	"errors"
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
	"testing"
)

func TestLossQuotaDefaults(t *testing.T) {
	ptr := func(v int64) *int64 { return &v }
	for _, tt := range []struct {
		name        string
		config      Config
		week, month int64
	}{
		{"absent", Config{}, 0, 0},
		{"daily", Config{LossDailyCapMinor: ptr(20000000000)}, 140000000000, 600000000000},
		{"zero", Config{LossDailyCapMinor: ptr(100), LossWeeklyCapMinor: ptr(0), LossMonthlyCapMinor: ptr(0)}, 700, 3000},
		{"manual-week", Config{LossDailyCapMinor: ptr(100), LossWeeklyCapMinor: ptr(1234)}, 1234, 3000},
		{"manual-month", Config{LossDailyCapMinor: ptr(100), LossMonthlyCapMinor: ptr(1234)}, 700, 1234},
		{"manual-both", Config{LossDailyCapMinor: ptr(100), LossWeeklyCapMinor: ptr(2345), LossMonthlyCapMinor: ptr(1234)}, 2345, 1234},
		{"no-daily", Config{LossDailyCapMinor: ptr(0), LossMonthlyCapMinor: ptr(1234)}, 0, 1234},
		{"maximum", Config{LossDailyCapMinor: ptr(100000000000000)}, 700000000000000, 3000000000000000},
	} {
		t.Run(tt.name, func(t *testing.T) {
			before := string(raw(tt.config))
			c := effectiveLossConfig(tt.config)
			if quotaCap(c.LossWeeklyCapMinor) != tt.week || quotaCap(c.LossMonthlyCapMinor) != tt.month {
				t.Fatal(c)
			}
			if string(raw(tt.config)) != before {
				t.Fatal("mutated stored automatic settings")
			}
			if validateConfig("srp", c) != nil || validateConfig("solo", c) != nil {
				t.Fatal("effective config rejected")
			}
			if string(raw(effectiveLossConfig(c))) != string(raw(c)) {
				t.Fatal("not idempotent")
			}
		})
	}
	for _, c := range []Config{{LossDailyCapMinor: ptr(100000000000001)}, {LossWeeklyCapMinor: ptr(700000000000001)}, {LossMonthlyCapMinor: ptr(3000000000000001)}, {LossWeeklyCapMinor: ptr(-1)}, {LossMonthlyCapMinor: ptr(99)}} {
		if validateConfig("srp", c) == nil {
			t.Fatal("invalid cap accepted", c)
		}
	}
}

func TestLossQuotaDefaultsEnforced(t *testing.T) {
	for _, period := range []string{"week", "month"} {
		t.Run(period, func(t *testing.T) {
			s, _ := fixture(t)
			ctx := context.Background()
			day := int64(10000)
			cfg := Config{LossDailyCapMinor: &day}
			limit := int64(70000)
			expected := ErrLossMonthlyConfirmation
			if period == "month" {
				week := int64(1000000)
				cfg.LossWeeklyCapMinor = &week
				limit = 300000
				expected = ErrLossMonthlyQuota
			}
			setQuota(t, s, cfg)
			p, effective, err := s.lossPolicy(ctx, s.Pool, 10, "srp")
			if err != nil {
				t.Fatal(err)
			}
			var stored Config
			if err = json.Unmarshal(p.Config, &stored); err != nil {
				t.Fatal(err)
			}
			if quotaCap(stored.LossMonthlyCapMinor) != 0 || quotaCap(effective.LossMonthlyCapMinor) != 300000 {
				t.Fatal("automatic mode lost", stored, effective)
			}
			v := quotaPending(t, s, 8501)
			if _, err = s.Execute(ctx, adminID, quotaApprove(v, limit+1, 8501)); !errors.Is(err, expected) {
				t.Fatal("ceiling bypassed", err)
			}
			if _, err = s.Execute(ctx, adminID, quotaApprove(v, limit, 8502)); err != nil {
				t.Fatal("exact cap rejected", err)
			}
			saved, err := store.Read(ctx, s.Pool, v.ID)
			if err != nil {
				t.Fatal(err)
			}
			var detail Detail
			if err = json.Unmarshal(saved.Detail, &detail); err != nil {
				t.Fatal(err)
			}
			if quotaCap(detail.Rule.LossMonthlyCapMinor) != 300000 || quotaCap(detail.Rule.LossWeeklyCapMinor) != quotaCap(effective.LossWeeklyCapMinor) {
				t.Fatal("effective caps not frozen", detail.Rule)
			}
			quotas, err := s.LossQuotas(ctx, userID, 10, []Policy{p})
			if err != nil {
				t.Fatal(err)
			}
			q := quotas[0]
			if q.FutureUsedMinor != 0 || q.Weekly.UsedMinor != limit || q.Monthly.UsedMinor != limit || quotaCap(q.Monthly.CapMinor) != 300000 {
				t.Fatal(q)
			}
		})
	}
}

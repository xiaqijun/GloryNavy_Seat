package welfare

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
)

func TestLossQuotaConfirmationLifecycle(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	day, week, month := int64(10000), int64(30000), int64(50000)
	setQuota(t, s, Config{LossDailyCapMinor: &day, LossWeeklyCapMinor: &week, LossMonthlyCapMinor: &month})
	a := quotaPending(t, s, 9101)
	cmd := quotaApprove(a, 11000, 9101)
	cmd.LossQuotaConsent = ""
	if _, err := s.Execute(ctx, adminID, cmd); !errors.Is(err, ErrLossWeeklyConfirmation) {
		t.Fatal(err)
	}
	var n int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM welfare_audit WHERE case_id=$1 AND action='approve'`, a.ID).Scan(&n); err != nil || n != 0 {
		t.Fatal(n, err)
	}
	preview, err := s.ReviewLossQuota(ctx, adminID, a.ID)
	if err != nil || preview.PolicyVersion != 1 || preview.Quota.Daily.ReservedMinor != 0 || quotaCap(preview.Quota.Daily.RemainingMinor) != 10000 {
		t.Fatal(preview, err)
	}
	for _, actor := range []string{userID, otherID} {
		if _, err = s.ReviewLossQuota(ctx, actor, a.ID); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatal("scope", actor, err)
		}
	}
	cmd.LossQuotaConsent = "weekly"
	out, err := s.Execute(ctx, adminID, cmd)
	if err != nil {
		t.Fatal(err)
	}
	var approved Case
	var detail Detail
	if json.Unmarshal(out, &approved) != nil || json.Unmarshal(approved.Detail, &detail) != nil || detail.LossQuotaApproval == nil || detail.LossQuotaApproval.Level != "weekly" || detail.LossQuotaApproval.Amount != 11000 || detail.Reviewer != adminID {
		t.Fatal(string(out))
	}
	if _, err = s.Execute(ctx, adminID, cmd); err != nil {
		t.Fatal("replay", err)
	}
	cmd.LossQuotaConsent = "monthly"
	if _, err = s.Execute(ctx, adminID, cmd); !errors.Is(err, ErrConflict) {
		t.Fatal("changed replay", err)
	}
	b := quotaPending(t, s, 9102)
	cmd = quotaApprove(b, 20000, 9102)
	if _, err = s.Execute(ctx, adminID, cmd); !errors.Is(err, ErrLossMonthlyConfirmation) {
		t.Fatal(err)
	}
	cmd.LossQuotaConsent = "monthly"
	// Reimbursement officers use their existing corporation scope; applicants cannot self-approve.
	s.CompensationScope = func(_ context.Context, actor string, corp int64) (bool, error) {
		return corp == 10 && (actor == otherID || actor == userID), nil
	}
	baseMembers := s.Members
	s.Members = func(ctx context.Context, actor string, corp int64) ([]Character, error) {
		if actor == otherID && corp == 10 {
			return baseMembers(ctx, adminID, corp)
		}
		return baseMembers(ctx, actor, corp)
	}
	if _, err = s.Execute(ctx, userID, cmd); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("self approval", err)
	}
	if _, err = s.Execute(ctx, otherID, cmd); err != nil {
		t.Fatal("officer", err)
	}
	c := quotaPending(t, s, 9103)
	cmd = quotaApprove(c, 19001, 9103)
	cmd.LossQuotaConsent = "monthly"
	if _, err = s.Execute(ctx, adminID, cmd); !errors.Is(err, ErrLossMonthlyQuota) {
		t.Fatal("monthly bypass", err)
	}
	cmd.Detail.BaseMinor = 19000
	if _, err = s.Execute(ctx, adminID, cmd); err != nil {
		t.Fatal("exact monthly", err)
	}
	var now time.Time
	s.Pool.QueryRow(ctx, `SELECT now()`).Scan(&now)
	spans, err := store.LossQuotaDays(ctx, s.Pool, userID, 10, quotaDay(now))
	used, future := dailyUsage(spans, "srp", quotaDay(now))
	if err != nil || used != 50000 || future != 0 {
		t.Fatal(used, future, err)
	}
}

func TestLossQuotaConcurrentConsentEscalation(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	day, week, month := int64(10000), int64(30000), int64(50000)
	setQuota(t, s, Config{LossDailyCapMinor: &day, LossWeeklyCapMinor: &week, LossMonthlyCapMinor: &month})
	a, b := quotaPending(t, s, 9201), quotaPending(t, s, 9202)
	commands := []Command{quotaApprove(a, 20000, 9201), quotaApprove(b, 20000, 9202)}
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i, c := range commands {
		wg.Add(1)
		go func(i int, c Command) { defer wg.Done(); _, errs[i] = s.Execute(ctx, adminID, c) }(i, c)
	}
	wg.Wait()
	successes := 0
	for i, err := range errs {
		if err == nil {
			successes++
			continue
		}
		if !errors.Is(err, ErrLossMonthlyConfirmation) {
			t.Fatal(err)
		}
		commands[i].LossQuotaConsent = "monthly"
		if _, err = s.Execute(ctx, adminID, commands[i]); err != nil {
			t.Fatal(err)
		}
	}
	if successes != 1 {
		t.Fatal(successes, errs)
	}
	var n int
	s.Pool.QueryRow(ctx, `SELECT count(*) FROM welfare_audit WHERE action='approve'`).Scan(&n)
	if n != 2 {
		t.Fatal(n)
	}
}

func TestLossQuotaLevelsAndErrors(t *testing.T) {
	day, week, month := int64(100), int64(700), int64(3000)
	q := LossQuota{Daily: quotaPeriod(0, 0, &day), Weekly: quotaPeriod(0, 0, &week), Monthly: quotaPeriod(0, 0, &month)}
	for _, tc := range []struct {
		amount int64
		level  string
		err    error
	}{{100, "", nil}, {101, "weekly", nil}, {700, "weekly", nil}, {701, "monthly", nil}, {3000, "monthly", nil}, {3001, "", ErrLossMonthlyQuota}} {
		level, err := lossQuotaLevel(q, tc.amount)
		if level != tc.level || !errors.Is(err, tc.err) {
			t.Fatal(tc, level, err)
		}
	}
	for _, tc := range []struct {
		err  error
		code string
	}{{ErrLossWeeklyConfirmation, "welfare_loss_weekly_confirmation"}, {ErrLossMonthlyConfirmation, "welfare_loss_monthly_confirmation"}, {ErrLossBeforeStart, "welfare_loss_before_start"}} {
		w := httptest.NewRecorder()
		respond(w, httptest.NewRequest("POST", "/", nil), nil, tc.err)
		if w.Code != 409 || !strings.Contains(w.Body.String(), tc.code) {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if checkLossStart(Config{LossOccurredAfter: "2026-10-01T00:00:00+08:00"}, "2026-09-30T16:00:00Z") != nil {
		t.Fatal("timezone equality")
	}
	for _, cfg := range []Config{{LossOccurredAfter: "invalid"}, {LossOccurredAfter: "2026-10-01"}} {
		if validateConfig("srp", cfg) == nil {
			t.Fatal("invalid date", cfg)
		}
	}
	if validateConfig("growth_gila", Config{LossOccurredAfter: "2026-10-01T00:00:00Z"}) == nil {
		t.Fatal("non loss date")
	}
}

func TestLossApplicationStartVerifiedTimestamp(t *testing.T) {
	for i, kind := range []string{"srp", "solo"} {
		t.Run(kind, func(t *testing.T) {
			s, _ := fixture(t)
			ctx := context.Background()
			start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
			cfg := Config{LossOccurredAfter: start.Format(time.RFC3339)}
			if _, err := s.Execute(ctx, adminID, Command{Action: "configure", CorporationID: 10, Kind: kind, Config: cfg, RequestKey: key(9300 + i)}); err != nil {
				t.Fatal(err)
			}
			loss := Loss{ID: int64(9301 + i), CharacterID: 123, CorporationID: 10, ShipTypeID: 34, At: start.Add(-time.Second)}
			s.Losses = func(context.Context, string, int64, int64, int64, int64) ([]Loss, error) { return []Loss{loss}, nil }
			cmd := Command{Action: "apply", CorporationID: 10, Kind: kind, RequestKey: key(9310 + i), Detail: Detail{SyncedLoss: true, CharacterID: 123, ShipTypeID: 34, KillmailID: loss.ID, EventID: 1, OccurredAt: time.Now().Format(time.RFC3339), Description: "损失", Evidence: "ESI"}}
			if _, err := s.Execute(ctx, userID, cmd); !errors.Is(err, ErrLossBeforeStart) {
				t.Fatal("forged timestamp", err)
			}
			loss.At = start
			out, err := s.Execute(ctx, userID, cmd)
			if err != nil {
				t.Fatal("inclusive boundary", err)
			}
			var v Case
			json.Unmarshal(out, &v)
			cfg.LossOccurredAfter = start.Add(time.Hour).Format(time.RFC3339)
			if _, err = s.Execute(ctx, adminID, Command{Action: "configure", CorporationID: 10, Kind: kind, Config: cfg, Version: 1, RequestKey: key(9320 + i)}); err != nil {
				t.Fatal(err)
			}
			approval := quotaApprove(v, 10000, 9330+i)
			version := int64(2)
			approval.LossPolicyVersion = &version
			approval.ManualPricing = kind == "solo"
			if _, err = s.Execute(ctx, adminID, approval); err != nil {
				t.Fatal("existing application should survive", err)
			}
			// Manual administrator entry is subject to the same start time.
			s.Losses = nil
			cmd.RequestKey = key(9340 + i)
			cmd.Detail.SyncedLoss = false
			cmd.Detail.OccurredAt = start.Format(time.RFC3339)
			cmd.Detail.KillmailID += 100
			if _, err = s.Execute(ctx, adminID, cmd); !errors.Is(err, ErrLossBeforeStart) {
				t.Fatal("manual bypass", err)
			}
		})
	}
}

package welfare

import (
	"context"
	"encoding/json"
	"errors"
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
	"math/rand"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestDailyQuotaAllocationIntervals(t *testing.T) {
	occupied := []store.LossQuotaSpan{{Kind: "srp", Start: 10, End: 11, Amount: 60}, {Kind: "srp", Start: 11, End: 13, Amount: 100}, {Kind: "srp", Start: 14, End: 15, Amount: 80}}
	plan, err := allocateDailyQuota("srp", 10, 290, 100, occupied)
	if err != nil {
		t.Fatal(err)
	}
	want := map[int64]int64{10: 40, 13: 100, 14: 20, 15: 100, 16: 30}
	for _, p := range plan {
		for d := p.Start; d < p.End; d++ {
			if want[d] != p.Amount {
				t.Fatalf("day %d: %+v", d, plan)
			}
			delete(want, d)
		}
	}
	if len(want) != 0 {
		t.Fatal(want)
	}
	plan, err = allocateDailyQuota("solo", 10, 100000000000000, 100, occupied)
	if err != nil || len(plan) != 1 || plan[0].End-plan[0].Start != 1000000000000 {
		t.Fatal(plan, err)
	}
	// Natural Beijing date boundaries, including UTC on the previous date.
	for _, v := range []string{"2026-10-31T16:00:00Z", "2026-11-01T15:59:59Z"} {
		at, _ := time.Parse(time.RFC3339, v)
		if quotaDay(at) != quotaDay(time.Date(2026, 11, 1, 12, 0, 0, 0, time.FixedZone("CST", 28800))) {
			t.Fatal(v)
		}
	}
}

func TestDailyQuotaAllocationMatchesCalendar(t *testing.T) {
	r := rand.New(rand.NewSource(1822))
	for n := 0; n < 300; n++ {
		cap := int64(r.Intn(100) + 1)
		occupied := []store.LossQuotaSpan{}
		calendar := map[int64]int64{}
		for k := 0; k < 20; k++ {
			start := int64(r.Intn(20))
			end := start + int64(r.Intn(4)+1)
			amount := int64(r.Intn(100) + 1)
			occupied = append(occupied, store.LossQuotaSpan{Kind: "srp", Start: start, End: end, Amount: amount})
			for d := start; d < end; d++ {
				calendar[d] += amount
			}
		}
		amount := int64(r.Intn(1000) + 1)
		plan, err := allocateDailyQuota("srp", 0, amount, cap, occupied)
		if err != nil {
			t.Fatal(err)
		}
		expected := map[int64]int64{}
		left := amount
		for d := int64(0); left > 0; d++ {
			take := min(left, max(int64(0), cap-calendar[d]))
			if take > 0 {
				expected[d] = take
				left -= take
			}
		}
		for _, v := range plan {
			for d := v.Start; d < v.End; d++ {
				if expected[d] != v.Amount {
					t.Fatalf("run %d day %d: %+v / %+v", n, d, plan, expected)
				}
				delete(expected, d)
			}
		}
		if len(expected) > 0 {
			t.Fatal(expected)
		}
	}
}

func quotaPending(t *testing.T, s *Service, km int64) Case {
	t.Helper()
	v, err := store.Save(context.Background(), s.Pool, Case{AccountID: userID, CorporationID: 10, Kind: "srp", State: "submitted", Keys: []string{}, Detail: raw(Detail{CharacterID: 123, ShipTypeID: 34, KillmailID: km, EventID: 1, OccurredAt: time.Now().Add(-time.Hour).Format(time.RFC3339)})})
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func quotaApprove(v Case, amount int64, n int) Command {
	version := int64(1)
	return Command{Action: "approve", ID: v.ID, Version: v.Version, RequestKey: key(n), Note: "审核", Detail: Detail{BaseMinor: amount}, LossPolicyVersion: &version}
}
func setQuota(t *testing.T, s *Service, c Config) {
	t.Helper()
	if _, err := s.Execute(context.Background(), adminID, Command{Action: "configure", CorporationID: 10, Kind: "srp", Config: c, RequestKey: key(8100)}); err != nil {
		t.Fatal(err)
	}
}

func TestLossQuotaWeeklyMonthlyRemainCeilings(t *testing.T) {
	for _, period := range []string{"week", "month", "both"} {
		t.Run(period, func(t *testing.T) {
			s, _ := fixture(t)
			ctx := context.Background()
			day, week, month := int64(10000), int64(30000), int64(40000)
			cfg := Config{LossDailyCapMinor: &day}
			if period != "month" {
				cfg.LossWeeklyCapMinor = &week
			}
			if period != "week" {
				cfg.LossMonthlyCapMinor = &month
			}
			setQuota(t, s, cfg)
			first := quotaPending(t, s, 8101)
			cmd := quotaApprove(first, 25000, 8101)
			result, err := s.Execute(ctx, adminID, cmd)
			if err != nil {
				t.Fatal("daily incorrectly blocked", err)
			}
			repeated, err := s.Execute(ctx, adminID, cmd)
			var originalJSON, repeatedJSON any
			if json.Unmarshal(result, &originalJSON) != nil || json.Unmarshal(repeated, &repeatedJSON) != nil {
				t.Fatal("invalid replay JSON")
			}
			if err != nil || !reflect.DeepEqual(originalJSON, repeatedJSON) {
				t.Fatal("replay", err)
			}
			var now time.Time
			s.Pool.QueryRow(ctx, `SELECT now()`).Scan(&now)
			spans, err := store.LossQuotaDays(ctx, s.Pool, userID, 10, quotaDay(now))
			if err != nil {
				t.Fatal(err)
			}
			daily, future := dailyUsage(spans, "srp", quotaDay(now))
			if daily != 25000 || future != 0 {
				t.Fatal(daily, future)
			}
			second := quotaPending(t, s, 8102)
			amount := int64(5001)
			expected := ErrLossWeeklyQuota
			if period == "month" {
				amount = 15001
				expected = ErrLossMonthlyQuota
			}
			if _, err = s.Execute(ctx, adminID, quotaApprove(second, amount, 8102)); !errors.Is(err, expected) {
				t.Fatal("ceiling bypassed", err)
			}
			var count int
			s.Pool.QueryRow(ctx, `SELECT count(*) FROM welfare_loss_quota_days WHERE case_id=$1`, second.ID).Scan(&count)
			if count != 0 {
				t.Fatal("rejected approval wrote allocations")
			}
			if _, err = s.Execute(ctx, adminID, quotaApprove(second, amount-1, 8103)); err != nil {
				t.Fatal("exact cap", err)
			}
		})
	}
}

func TestLossQuotaConcurrentAdvanceReplayAndCancellation(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	cap := int64(10000)
	setQuota(t, s, Config{LossDailyCapMinor: &cap})
	a, b := quotaPending(t, s, 8201), quotaPending(t, s, 8202)
	commands := []Command{quotaApprove(a, 30000, 8201), quotaApprove(b, 30000, 8202)}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, cmd := range commands {
		wg.Add(1)
		go func(c Command) { defer wg.Done(); _, err := s.Execute(ctx, adminID, c); errs <- err }(cmd)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Execute(ctx, adminID, commands[0]); err != nil {
		t.Fatal(err)
	}
	var now time.Time
	if err := s.Pool.QueryRow(ctx, `SELECT now()`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	day := quotaDay(now)
	spans, err := store.LossQuotaDays(ctx, s.Pool, userID, 10, day)
	if err != nil {
		t.Fatal(err)
	}
	used, future := dailyUsage(spans, "srp", day)
	if used != 10000 || future != 50000 {
		t.Fatal(used, future, spans)
	}
	for d := day; d < day+6; d++ {
		used, _ := dailyUsage(spans, "srp", d)
		if used != 10000 {
			t.Fatal(d, used)
		}
	}
	// Delivery state changes preserve the original allocations, including tomorrow.
	if _, err = s.Pool.Exec(ctx, `UPDATE welfare_cases SET state='cancel_requested',updated_at=now()+interval '10 days' WHERE id=$1`, a.ID); err != nil {
		t.Fatal(err)
	}
	after, err := store.LossQuotaDays(ctx, s.Pool, userID, 10, day+1)
	if err != nil {
		t.Fatal(err)
	}
	used, future = dailyUsage(after, "srp", day+1)
	if used != 10000 || future != 40000 {
		t.Fatal(used, future)
	}
	if _, err = s.Pool.Exec(ctx, `UPDATE welfare_cases SET state='cancelled' WHERE id=$1`, a.ID); err != nil {
		t.Fatal(err)
	}
	after, err = store.LossQuotaDays(ctx, s.Pool, userID, 10, day)
	if err != nil {
		t.Fatal(err)
	}
	used, future = dailyUsage(after, "srp", day)
	if used+future != 30000 {
		t.Fatal("cancellation did not free exact plan", used, future)
	}
	// The ledger follows parent ownership during the existing account merge.
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = s.MergeAccountTx(ctx, tx, userID, otherID, true); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	old, err := store.LossQuotaDays(ctx, s.Pool, userID, 10, day)
	if err != nil || len(old) != 0 {
		t.Fatal(old, err)
	}
	moved, err := store.LossQuotaDays(ctx, s.Pool, otherID, 10, day)
	if err != nil {
		t.Fatal(err)
	}
	used, future = dailyUsage(moved, "srp", day)
	if used+future != 30000 {
		t.Fatal(used, future)
	}
}

func TestLossQuotaSummaryPendingAndFrozenSettings(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	cap := int64(10000)
	cfg := Config{LossDailyCapMinor: &cap}
	setQuota(t, s, cfg)
	first := quotaPending(t, s, 8301)
	if _, err := s.Execute(ctx, adminID, quotaApprove(first, 25000, 8301)); err != nil {
		t.Fatal(err)
	}
	pending := quotaPending(t, s, 8302)
	if _, err := s.Pool.Exec(ctx, `UPDATE welfare_cases SET detail=jsonb_set(detail,'{base_minor}','10000') WHERE id=$1`, pending.ID); err != nil {
		t.Fatal(err)
	}
	policies := []Policy{{Kind: "srp", Config: raw(cfg)}}
	quotas, err := s.LossQuotas(ctx, userID, 10, policies)
	if err != nil {
		t.Fatal(err)
	}
	q := quotas[0]
	if q.Daily.UsedMinor != 10000 || q.Daily.ReservedMinor != 0 || q.FutureUsedMinor != 15000 || q.FutureReservedMinor != 10000 || q.Weekly.UsedMinor != 25000 || q.Weekly.ReservedMinor != 10000 {
		t.Fatalf("%+v", q)
	}
	// Changing the daily setting does not rewrite already frozen future dates.
	changed := int64(20000)
	cfg.LossDailyCapMinor = &changed
	quotas, err = s.LossQuotas(ctx, userID, 10, []Policy{{Kind: "srp", Config: raw(cfg)}})
	if err != nil {
		t.Fatal(err)
	}
	q = quotas[0]
	if q.FutureUsedMinor != 15000 || q.Daily.UsedMinor != 10000 || q.Daily.ReservedMinor != 10000 {
		t.Fatalf("%+v", q)
	}
	if b, err := json.Marshal(q); err != nil || len(b) == 0 {
		t.Fatal(err)
	}
}

func TestLossQuotaApprovalFailureDoesNotReserveDays(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	cap := int64(10000)
	setQuota(t, s, Config{LossDailyCapMinor: &cap})
	first := quotaPending(t, s, 8401)
	if _, err := s.Execute(ctx, adminID, quotaApprove(first, 25000, 8401)); err != nil {
		t.Fatal(err)
	}
	duplicate := quotaPending(t, s, 8401)
	if _, err := s.Execute(ctx, adminID, quotaApprove(duplicate, 25000, 8402)); err == nil {
		t.Fatal("duplicate entitlement approved")
	}
	var count int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM welfare_loss_quota_days WHERE case_id=$1`, duplicate.ID).Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
	saved, err := store.Read(ctx, s.Pool, duplicate.ID)
	if err != nil || saved.State != "submitted" || saved.Version != duplicate.Version {
		t.Fatal(saved, err)
	}
	if err = s.Pool.QueryRow(ctx, `SELECT count(*) FROM welfare_audit WHERE case_id=$1 AND action='approve'`, duplicate.ID).Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
}

package exchange

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"glorynavy.local/seat/internal/modules/identity"
)

func clientUsageRequest(n int, seconds int64, system, version string) (AlertTimeGrantRequest, AlertTimeIntervalRequest, string) {
	start := time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC).Add(time.Duration(n) * time.Hour)
	g := AlertTimeGrantRequest{ID: rewardKey(10000 + n*3), AccountID: manager, SystemID: system, RequestKey: rewardKey(10001 + n*3), PriceVersion: version, UnitSeconds: 3600, UnitPriceMinor: 200, ReservedSeconds: seconds, ExpiresAt: start.Add(100 * 24 * time.Hour)}
	i := AlertTimeIntervalRequest{GrantID: g.ID, IntervalID: fmt.Sprintf("raw-usage-%d", n), RequestKey: rewardKey(10002 + n*3), StartedAt: start, EndedAt: start.Add(time.Duration(seconds) * time.Second)}
	return g, i, rewardKey(30000 + n)
}

func chargeClientUsage(t *testing.T, s *Service, n int, seconds int64, system, version string) error {
	t.Helper()
	g, i, key := clientUsageRequest(n, seconds, system, version)
	return s.SettleClientUsage(context.Background(), g, i, key)
}

func clientUsageFixture(t *testing.T) *Service {
	t.Helper()
	s, _, _ := rewardFixture(t)
	if _, err := s.Pool.Exec(context.Background(), `INSERT INTO identity_users(id) VALUES($1),($2) ON CONFLICT DO NOTHING`, manager, member); err != nil {
		t.Fatal(err)
	}
	s.AllowAlertConsumption = true
	s.LockAccounts = identity.New(s.Pool).LockActiveAccounts
	return s
}

func TestCumulativeAlertCostBoundaries(t *testing.T) {
	for _, c := range []struct{ seconds, price, unit, credit, cost, next int64 }{
		{1, 200, 3600, 0, 1, 3400}, {1, 200, 3600, 3400, 0, 3200},
		{18, 200, 3600, 0, 1, 0}, {60, 200, 3600, 0, 4, 2400},
		{1, 200, 3600, 6800, 0, 6600}, {math.MaxInt64, 1, math.MaxInt64, 0, 1, 0},
	} {
		cost, next, err := cumulativeAlertCost(c.seconds, c.price, c.unit, c.credit)
		if err != nil || cost != c.cost || next != c.next {
			t.Fatalf("%+v: cost=%d credit=%d error=%v", c, cost, next, err)
		}
	}
	if _, _, err := cumulativeAlertCost(math.MaxInt64, 2, 3600, 0); !errors.Is(err, ErrInvalid) {
		t.Fatalf("overflow accepted: %v", err)
	}
}

func TestClientUsageRoundingReplayRestartAndIsolation(t *testing.T) {
	s := clientUsageFixture(t)
	ctx := context.Background()
	if err := chargeClientUsage(t, s, 0, 1, "A", "v1"); err != nil {
		t.Fatal(err)
	}
	// A new service object and a changed current price cannot reprice a replay.
	restarted := *s
	if err := chargeClientUsage(t, &restarted, 0, 1, "A", "v2"); err != nil {
		t.Fatal(err)
	}
	if err := chargeClientUsage(t, &restarted, 1, 1, "A", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := chargeClientUsage(t, &restarted, 2, 1, "B", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := chargeClientUsage(t, &restarted, 3, 1, "A", "v2"); err != nil {
		t.Fatal(err)
	}
	g, i, key := clientUsageRequest(4, 1, "A", "v1")
	g.AccountID = member
	if err := s.SettleClientUsage(ctx, g, i, key); err != nil {
		t.Fatal(err)
	}
	// Even a mistakenly reused version label must not mix different frozen rates.
	g, i, key = clientUsageRequest(5, 1, "A", "v1")
	g.UnitPriceMinor = 100
	if err := s.SettleClientUsage(ctx, g, i, key); err != nil {
		t.Fatal(err)
	}
	var charges, total, buckets int64
	if err := s.Pool.QueryRow(ctx, `SELECT count(*),sum(coins_minor),(SELECT count(*) FROM exchange_alert_rounding) FROM exchange_alert_charges`).Scan(&charges, &total, &buckets); err != nil {
		t.Fatal(err)
	}
	if charges != 6 || total != 5 || buckets != 5 {
		t.Fatalf("charges=%d total=%d buckets=%d", charges, total, buckets)
	}
	if err := chargeClientUsage(t, s, 0, 2, "A", "v1"); !errors.Is(err, ErrConflict) {
		t.Fatalf("altered replay accepted: %v", err)
	}
}

func TestClientUsageRoundingInsufficientBalanceAndZeroCharge(t *testing.T) {
	s := clientUsageFixture(t)
	ctx := context.Background()
	if err := chargeClientUsage(t, s, 0, 1, "A", "v1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO exchange_coin_ledger(account_id,kind,reference,request_key,delta,reason) VALUES($1,'source','test-drain',$2,-9,'fixture')`, manager, rewardKey(9001)); err != nil {
		t.Fatal(err)
	}
	if err := chargeClientUsage(t, s, 1, 1, "A", "v1"); err != nil {
		t.Fatalf("prepaid fraction must cover zero-cost interval: %v", err)
	}
	if err := chargeClientUsage(t, s, 2, 18, "A", "v1"); !errors.Is(err, ErrInsufficientCoinsMinor) {
		t.Fatalf("insufficient funds: %v", err)
	}
	var credit, charges, entries int64
	if err := s.Pool.QueryRow(ctx, `SELECT credit_numerator,(SELECT count(*) FROM exchange_alert_charges),(SELECT count(*) FROM exchange_coin_ledger WHERE kind='alert_reserve') FROM exchange_alert_rounding`).Scan(&credit, &charges, &entries); err != nil {
		t.Fatal(err)
	}
	if credit != 3200 || charges != 2 || entries != 1 {
		t.Fatalf("credit=%d charges=%d entries=%d", credit, charges, entries)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO exchange_coin_ledger(account_id,kind,reference,request_key,delta,reason) VALUES($1,'source','test-refill',$2,1,'fixture')`, manager, rewardKey(9002)); err != nil {
		t.Fatal(err)
	}
	if err := chargeClientUsage(t, s, 2, 18, "A", "v1"); err != nil {
		t.Fatal(err)
	}
}

func TestClientUsageRoundingConcurrentDuplicate(t *testing.T) {
	s := clientUsageFixture(t)
	var wg sync.WaitGroup
	errs := make(chan error, 40)
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for n := worker % 2; n < 20; n += 2 {
				errs <- chargeClientUsage(t, s, n, 1, "A", "v1")
			}
		}(worker)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var charges, total int64
	if err := s.Pool.QueryRow(context.Background(), `SELECT count(*),sum(coins_minor) FROM exchange_alert_charges`).Scan(&charges, &total); err != nil {
		t.Fatal(err)
	}
	if charges != 20 || total != 2 {
		t.Fatalf("charges=%d cost=%d", charges, total)
	}
}

func TestClientUsageRoundingTransactionFailure(t *testing.T) {
	s := clientUsageFixture(t)
	ctx := context.Background()
	if _, err := s.Pool.Exec(ctx, `CREATE FUNCTION reject_alert_coin() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'test ledger failure'; END$$; CREATE TRIGGER reject_alert_coin BEFORE INSERT ON exchange_coin_ledger FOR EACH ROW EXECUTE FUNCTION reject_alert_coin()`); err != nil {
		t.Fatal(err)
	}
	if err := chargeClientUsage(t, s, 0, 60, "A", "v1"); err == nil {
		t.Fatal("ledger failure swallowed")
	}
	var rows int64
	if err := s.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM exchange_alert_grants)+(SELECT count(*) FROM exchange_alert_charges)+(SELECT count(*) FROM exchange_alert_rounding)`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf("partial transaction remains: %d rows", rows)
	}
	if _, err := s.Pool.Exec(ctx, `DROP TRIGGER reject_alert_coin ON exchange_coin_ledger`); err != nil {
		t.Fatal(err)
	}
	if err := chargeClientUsage(t, s, 0, 60, "A", "v1"); err != nil {
		t.Fatal(err)
	}
}

func TestClientUsageLegacyBatchReplayDoesNotRebill(t *testing.T) {
	s := clientUsageFixture(t)
	ctx := context.Background()
	g, i, key := clientUsageRequest(0, 60, "", "legacy")
	i.IntervalID = "old-batch"
	if err := s.ReserveAlertTime(ctx, g); err != nil {
		t.Fatal(err)
	}
	if err := s.ReserveAlertInterval(ctx, i); err != nil {
		t.Fatal(err)
	}
	if err := s.SettleAlertTime(ctx, g.ID, i.IntervalID, key); err != nil {
		t.Fatal(err)
	}
	for n := 1; n <= 2; n++ {
		raw, interval, settle := clientUsageRequest(n, 30, "", "new-price")
		interval.StartedAt = i.StartedAt.Add(time.Duration(n-1) * 30 * time.Second)
		interval.EndedAt = interval.StartedAt.Add(30 * time.Second)
		if err := s.SettleClientUsage(ctx, raw, interval, settle); err != nil {
			t.Fatal(err)
		}
	}
	raw, interval, settle := clientUsageRequest(3, 30, "", "new-price")
	interval.StartedAt = i.EndedAt.Add(-15 * time.Second)
	interval.EndedAt = i.EndedAt.Add(15 * time.Second)
	if err := s.SettleClientUsage(ctx, raw, interval, settle); !errors.Is(err, ErrConflict) {
		t.Fatalf("ambiguous partial coverage accepted: %v", err)
	}
	var charges, total, buckets int64
	if err := s.Pool.QueryRow(ctx, `SELECT count(*),sum(coins_minor),(SELECT count(*) FROM exchange_alert_rounding) FROM exchange_alert_charges`).Scan(&charges, &total, &buckets); err != nil {
		t.Fatal(err)
	}
	if charges != 1 || total != 4 || buckets != 0 {
		t.Fatalf("legacy history rewritten: count=%d cost=%d buckets=%d", charges, total, buckets)
	}
}

func TestClientUsageRoundingAccountMergePreservesCredits(t *testing.T) {
	s := clientUsageFixture(t)
	ctx := context.Background()
	if err := chargeClientUsage(t, s, 0, 1, "A", "v1"); err != nil {
		t.Fatal(err)
	}
	g, i, key := clientUsageRequest(1, 1, "A", "v1")
	g.AccountID = member
	if err := s.SettleClientUsage(ctx, g, i, key); err != nil {
		t.Fatal(err)
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	before, err := s.MergeAccountTx(ctx, tx, member, manager, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE exchange_alert_rounding SET credit_numerator=credit_numerator-1 WHERE account_id=$1`, member); err != nil {
		t.Fatal(err)
	}
	after, err := s.MergeAccountTx(ctx, tx, member, manager, false)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) == string(after) {
		t.Fatal("rounding credit absent from merge fingerprint")
	}
	if _, err := tx.Exec(ctx, `UPDATE exchange_alert_rounding SET credit_numerator=credit_numerator+1 WHERE account_id=$1`, member); err != nil {
		t.Fatal(err)
	}
	if _, err = s.MergeAccountTx(ctx, tx, member, manager, true); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO identity_account_merges(source_id,target_id,request_id) VALUES($1,$2,$3)`, member, manager, rewardKey(8000)); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var credit, originalCharges int64
	if err := s.Pool.QueryRow(ctx, `SELECT credit_numerator,(SELECT count(*) FROM exchange_alert_charges WHERE original_account_id=$2) FROM exchange_alert_rounding WHERE account_id=$1`, manager, member).Scan(&credit, &originalCharges); err != nil {
		t.Fatal(err)
	}
	if credit != 6800 || originalCharges != 1 {
		t.Fatalf("credit=%d provenance=%d", credit, originalCharges)
	}
	if err := chargeClientUsage(t, s, 2, 30, "A", "v1"); err != nil {
		t.Fatal(err)
	}
	s.LockAccounts = identity.New(s.Pool).LockActiveAccounts
	g, i, key = clientUsageRequest(3, 1, "A", "v1")
	g.AccountID = member
	if err := s.SettleClientUsage(ctx, g, i, key); err == nil {
		t.Fatal("retired account accepted")
	}
}

func TestClientUsageCumulativeRounding(t *testing.T) {
	s := clientUsageFixture(t)
	ctx := context.Background()
	if _, err := s.Pool.Exec(ctx, `INSERT INTO exchange_coin_ledger(account_id,kind,reference,request_key,delta,reason) VALUES($1,'source','test-fund',$2,1000,'fixture')`, manager, rewardKey(9000)); err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 60; n++ {
		if err := chargeClientUsage(t, s, n, 1, "30000142", "hourly-v13"); err != nil {
			t.Fatal(err)
		}
	}
	var total int64
	if err := s.Pool.QueryRow(ctx, `SELECT sum(coins_minor) FROM exchange_alert_charges`).Scan(&total); err != nil {
		t.Fatal(err)
	}
	// 60 seconds at 2.00 coins/hour: one cumulative ceiling is 0.04 coins.
	if total != 4 {
		t.Fatalf("fragmented usage charged %d minor; want 4", total)
	}
}

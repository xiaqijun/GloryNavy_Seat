package app

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"glorynavy.local/seat/internal/modules/access"
	"glorynavy.local/seat/internal/modules/exchange"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/modules/sentry"
	"glorynavy.local/seat/internal/testutil"
	"glorynavy.local/seat/migrations"
)

const usageWindowAccount = "11111111-1111-4111-8111-111111111111"

func usageWindowFixture(t *testing.T) (*pgxpool.Pool, *sentry.Service, *sentryAlertSettlement, time.Time) {
	t.Helper()
	pool := testutil.Database(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO identity_users(id) VALUES($1);`, usageWindowAccount); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO access_administrators(user_id) VALUES($1)`, usageWindowAccount); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO exchange_coin_ledger(account_id,kind,reference,request_key,delta,reason) VALUES($1,'source','window-fixture','99999999-1111-4111-8111-111111111111',1000,'fixture')`, usageWindowAccount); err != nil {
		t.Fatal(err)
	}
	s := sentry.New(pool, nil)
	s.AlertEnabled = true
	s.Administrator = access.New(pool, nil, nil).IsAdministrator
	enabled := true
	p, err := s.EditTimePricing(ctx, usageWindowAccount, sentry.TimePricingEdit{AlertHourlyPriceMinor: 200, MonitorHourlyRewardMinor: 100, ChargingEnabled: &enabled})
	if err != nil {
		t.Fatal(err)
	}
	b := &sentryAlertSettlement{exchange: &exchange.Service{Pool: pool, AllowAlertConsumption: true, LockAccounts: identity.New(pool).LockActiveAccounts}}
	opened := p.UpdatedAt.Add(-time.Hour)
	if _, err := pool.Exec(ctx, `UPDATE sentry_alert_pricing_audit SET created_at=$1`, opened); err != nil {
		t.Fatal(err)
	}
	return pool, s, b, opened
}

func TestClientUsageWindowSerializesClosing(t *testing.T) {
	pool, service, boundary, opened := usageWindowFixture(t)
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	c := windowUsage(0, opened, "30000142")
	if excluded, err := sentry.GuardClientUsageTx(ctx, tx, c, false); err != nil || excluded {
		t.Fatalf("valid guard: excluded=%t error=%v", excluded, err)
	}
	closing, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	disabled := false
	_, err = service.EditTimePricing(closing, usageWindowAccount, sentry.TimePricingEdit{AlertHourlyPriceMinor: 200, MonitorHourlyRewardMinor: 100, Version: 1, ChargingEnabled: &disabled})
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("closing did not wait for billing transaction: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := service.EditTimePricing(ctx, usageWindowAccount, sentry.TimePricingEdit{AlertHourlyPriceMinor: 200, MonitorHourlyRewardMinor: 100, Version: 1, ChargingEnabled: &disabled}); err != nil {
		t.Fatal(err)
	}
	if err := boundary.SettleClientUsage(ctx, c); err == nil {
		t.Fatal("new interval charged after closing committed")
	}
	var rows int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM exchange_alert_charges)+(SELECT count(*) FROM sentry_client_usage_exclusions)`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("closing left charge or exclusion: %d %v", rows, err)
	}
}

func TestClientUsageWindowExclusionsMergeAndGuardDown(t *testing.T) {
	pool, service, boundary, opened := usageWindowFixture(t)
	ctx := context.Background()
	c := windowUsage(0, opened.Add(-time.Hour), "")
	if err := boundary.SettleClientUsage(ctx, c); err != nil {
		t.Fatal(err)
	}
	const target = "22222222-2222-4222-8222-222222222222"
	if _, err := pool.Exec(ctx, `INSERT INTO identity_users(id) VALUES($1)`, target); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	before, err := service.MergeAccountTx(ctx, tx, usageWindowAccount, target, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE sentry_client_usage_exclusions SET reason='crosses_latest_enable' WHERE usage_id=$1`, c.UsageID); err != nil {
		t.Fatal(err)
	}
	after, err := service.MergeAccountTx(ctx, tx, usageWindowAccount, target, false)
	if err != nil || string(before) == string(after) {
		t.Fatalf("exclusion absent from merge fingerprint: %v", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE sentry_client_usage_exclusions SET reason='before_latest_enable' WHERE usage_id=$1`, c.UsageID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.MergeAccountTx(ctx, tx, usageWindowAccount, target, true); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var owner, original string
	if err := pool.QueryRow(ctx, `SELECT account_id::text,original_account_id::text FROM sentry_client_usage_exclusions WHERE usage_id=$1`, c.UsageID).Scan(&owner, &original); err != nil || owner != target || original != usageWindowAccount {
		t.Fatalf("merge provenance: %s %s %v", owner, original, err)
	}
	c.AccountID = target
	if err := boundary.SettleClientUsage(ctx, c); err != nil {
		t.Fatalf("merged exclusion replay failed: %v", err)
	}
	db := stdlib.OpenDB(*pool.Config().ConnConfig)
	defer db.Close()
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.DownTo(ctx, 106); err == nil {
		t.Fatal("Down erased exclusion evidence")
	}
}

func TestClientUsageWindowReplayPriceEditAndReenable(t *testing.T) {
	pool, service, boundary, opened := usageWindowFixture(t)
	ctx := context.Background()
	paid := windowUsage(0, opened, "30000142")
	excluded := windowUsage(1, opened.Add(-time.Hour), "")
	for _, c := range []sentry.ClientUsageSettlement{paid, excluded} {
		if err := boundary.SettleClientUsage(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	var originalAudit int64
	if err := pool.QueryRow(ctx, `SELECT enable_audit_id FROM sentry_client_usage_exclusions WHERE usage_id=$1`, excluded.UsageID).Scan(&originalAudit); err != nil {
		t.Fatal(err)
	}
	enabled := true
	p, err := service.EditTimePricing(ctx, usageWindowAccount, sentry.TimePricingEdit{AlertHourlyPriceMinor: 200, MonitorHourlyRewardMinor: 100, Version: 1, ChargingEnabled: &enabled})
	if err != nil {
		t.Fatal(err)
	}
	pending := windowUsage(2, opened.Add(time.Second), "30000142")
	pending.PriceVersion = "hourly-v2"
	if err := boundary.SettleClientUsage(ctx, pending); err != nil {
		t.Fatalf("ordinary price edit lost earlier pending interval: %v", err)
	}
	enabled = false
	p, err = service.EditTimePricing(ctx, usageWindowAccount, sentry.TimePricingEdit{AlertHourlyPriceMinor: 200, MonitorHourlyRewardMinor: 100, Version: p.Version, ChargingEnabled: &enabled})
	if err != nil {
		t.Fatal(err)
	}
	beforePause := windowUsage(3, opened.Add(2*time.Second), "30000142")
	if err := boundary.SettleClientUsage(ctx, beforePause); err == nil {
		t.Fatal("closed gate accepted new interval")
	}
	enabled = true
	p, err = service.EditTimePricing(ctx, usageWindowAccount, sentry.TimePricingEdit{AlertHourlyPriceMinor: 200, MonitorHourlyRewardMinor: 100, Version: p.Version, ChargingEnabled: &enabled})
	if err != nil {
		t.Fatal(err)
	}
	beforePause.PriceVersion = "hourly-v4"
	for _, c := range []sentry.ClientUsageSettlement{paid, excluded, beforePause, excluded} {
		if err := boundary.SettleClientUsage(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	excluded.SystemID = "30000142"
	if err := boundary.SettleClientUsage(ctx, excluded); err == nil {
		t.Fatal("excluded usage payload drift accepted")
	}
	var charges, exclusions int
	var replayAudit int64
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM exchange_alert_charges),(SELECT count(*) FROM sentry_client_usage_exclusions),(SELECT enable_audit_id FROM sentry_client_usage_exclusions WHERE usage_id=$1)`, excluded.UsageID).Scan(&charges, &exclusions, &replayAudit); err != nil {
		t.Fatal(err)
	}
	if charges != 2 || exclusions != 2 || replayAudit != originalAudit {
		t.Fatalf("replay or reenable changed evidence: charges=%d exclusions=%d audit=%d/%d", charges, exclusions, replayAudit, originalAudit)
	}
}

func TestClientUsageWindowMissingBoundaryAndExclusionFailure(t *testing.T) {
	pool, _, boundary, opened := usageWindowFixture(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE sentry_alert_pricing_audit SET before_snapshot=after_snapshot`); err != nil {
		t.Fatal(err)
	}
	c := windowUsage(0, opened, "30000142")
	if err := boundary.SettleClientUsage(ctx, c); err == nil {
		t.Fatal("missing enable boundary accepted")
	}
	if _, err := pool.Exec(ctx, `UPDATE sentry_alert_pricing_audit SET before_snapshot=jsonb_set(before_snapshot,'{charging_enabled}','false')`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `CREATE FUNCTION reject_usage_exclusion() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'test exclusion failure'; END$$; CREATE TRIGGER reject_usage_exclusion BEFORE INSERT ON sentry_client_usage_exclusions FOR EACH ROW EXECUTE FUNCTION reject_usage_exclusion()`); err != nil {
		t.Fatal(err)
	}
	c = windowUsage(1, opened.Add(-time.Hour), "")
	if err := boundary.SettleClientUsage(ctx, c); err == nil {
		t.Fatal("exclusion write failure swallowed")
	}
	var rows int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM exchange_alert_charges)+(SELECT count(*) FROM sentry_client_usage_exclusions)+(SELECT count(*) FROM exchange_alert_rounding)`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("failed exclusion left state: %d %v", rows, err)
	}
	if _, err := pool.Exec(ctx, `DROP TRIGGER reject_usage_exclusion ON sentry_client_usage_exclusions`); err != nil {
		t.Fatal(err)
	}
	if err := boundary.SettleClientUsage(ctx, c); err != nil {
		t.Fatal(err)
	}
}

func windowUsage(n int, start time.Time, system string) sentry.ClientUsageSettlement {
	return sentry.ClientUsageSettlement{UsageID: fmt.Sprintf("window-usage-%d", n), GrantID: fmt.Sprintf("10000000-0000-4000-8000-%012d", n), AccountID: usageWindowAccount, SystemID: system, ReserveKey: fmt.Sprintf("20000000-0000-4000-8000-%012d", n), IntervalKey: fmt.Sprintf("30000000-0000-4000-8000-%012d", n), SettleKey: fmt.Sprintf("40000000-0000-4000-8000-%012d", n), PriceVersion: "hourly-v1", UnitSeconds: 3600, UnitPriceMinor: 200, DurationSeconds: 1, StartedAt: start, EndedAt: start.Add(time.Second), ExpiresAt: start.Add(24 * time.Hour)}
}

func TestClientUsageWindowExcludesPriorIntervals(t *testing.T) {
	pool, _, boundary, opened := usageWindowFixture(t)
	ctx := context.Background()
	for n, start := range []time.Time{opened.Add(-time.Hour), opened.Add(-500 * time.Millisecond)} {
		c := windowUsage(n, start, "")
		if err := boundary.SettleClientUsage(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	var charges, ledger, exclusions int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM exchange_alert_charges),(SELECT count(*) FROM exchange_coin_ledger WHERE kind='alert_reserve')`).Scan(&charges, &ledger); err != nil {
		t.Fatal(err)
	}
	if charges != 0 || ledger != 0 {
		t.Fatalf("usage outside enable window was charged: charges=%d ledger=%d", charges, ledger)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM sentry_client_usage_exclusions WHERE enable_audit_id IS NOT NULL`).Scan(&exclusions); err != nil || exclusions != 2 {
		t.Fatalf("missing exclusion evidence: %d %v", exclusions, err)
	}
	if err := boundary.SettleClientUsage(ctx, windowUsage(2, opened, "30000142")); err != nil {
		t.Fatalf("interval starting at enable boundary rejected: %v", err)
	}
}

func TestClientUsageWindowRejectsInvalidSystemAndPreservesEvidence(t *testing.T) {
	pool, _, boundary, opened := usageWindowFixture(t)
	ctx := context.Background()
	c := windowUsage(0, opened, "")
	if err := boundary.SettleClientUsage(ctx, c); !errors.Is(err, exchange.ErrInvalid) {
		t.Fatalf("invalid system accepted: %v", err)
	}
	var rows int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM exchange_alert_charges)+(SELECT count(*) FROM sentry_client_usage_exclusions)+(SELECT count(*) FROM exchange_alert_rounding)`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("invalid current usage swallowed: rows=%d error=%v", rows, err)
	}
	c.SystemID = "30000142"
	if err := boundary.SettleClientUsage(ctx, c); err != nil {
		t.Fatal(err)
	}
}

func TestClientUsageWindowDoesNotReleaseLegacyReservation(t *testing.T) {
	pool, _, boundary, opened := usageWindowFixture(t)
	ctx := context.Background()
	c := windowUsage(0, opened.Add(-time.Hour), "")
	g := exchange.AlertTimeGrantRequest{ID: c.GrantID, AccountID: c.AccountID, SystemID: c.SystemID, RequestKey: c.ReserveKey, PriceVersion: c.PriceVersion, UnitSeconds: c.UnitSeconds, UnitPriceMinor: c.UnitPriceMinor, ReservedSeconds: c.DurationSeconds, ExpiresAt: c.ExpiresAt}
	i := exchange.AlertTimeIntervalRequest{GrantID: c.GrantID, IntervalID: c.UsageID, RequestKey: c.IntervalKey, StartedAt: c.StartedAt, EndedAt: c.EndedAt}
	if err := boundary.exchange.ReserveAlertTime(ctx, g); err != nil {
		t.Fatal(err)
	}
	if err := boundary.exchange.ReserveAlertInterval(ctx, i); err != nil {
		t.Fatal(err)
	}
	if err := boundary.SettleClientUsage(ctx, c); err != nil {
		t.Fatal(err)
	}
	var state string
	var reserved, settled, released, exclusions, ledger int64
	if err := pool.QueryRow(ctx, `SELECT c.state,g.reserved_minor,g.settled_minor,g.released_minor,(SELECT count(*) FROM sentry_client_usage_exclusions),(SELECT count(*) FROM exchange_coin_ledger WHERE kind='alert_reserve') FROM exchange_alert_charges c JOIN exchange_alert_grants g ON g.id=c.grant_id WHERE c.interval_id=$1`, c.UsageID).Scan(&state, &reserved, &settled, &released, &exclusions, &ledger); err != nil {
		t.Fatal(err)
	}
	if state != "reserved" || reserved != 1 || settled != 0 || released != 0 || exclusions != 1 || ledger != 1 {
		t.Fatalf("exclusion changed old reservation: %s %d %d %d %d %d", state, reserved, settled, released, exclusions, ledger)
	}
}

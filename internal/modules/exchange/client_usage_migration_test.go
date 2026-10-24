package exchange

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"glorynavy.local/seat/migrations"
)

func TestClientUsageMigrationPreservesHistoryAndGuardsDown(t *testing.T) {
	s := clientUsageFixture(t)
	ctx := context.Background()
	db := stdlib.OpenDB(*s.Pool.Config().ConnConfig)
	defer db.Close()
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = provider.DownTo(ctx, 105); err != nil {
		t.Fatal(err)
	}
	g, i, key := clientUsageRequest(0, 60, "A", "v1")
	if err = s.ReserveAlertTime(ctx, g); err != nil {
		t.Fatal(err)
	}
	if err = s.ReserveAlertInterval(ctx, i); err != nil {
		t.Fatal(err)
	}
	if err = s.SettleAlertTime(ctx, g.ID, i.IntervalID, key); err != nil {
		t.Fatal(err)
	}
	const history = `SELECT md5((SELECT string_agg((to_jsonb(g)-'cumulative_rounding')::text,'' ORDER BY id) FROM exchange_alert_grants g) || (SELECT string_agg(to_jsonb(c)::text,'' ORDER BY id) FROM exchange_alert_charges c) || (SELECT string_agg(to_jsonb(l)::text,'' ORDER BY id) FROM exchange_coin_ledger l))`
	var before, after string
	if err = s.Pool.QueryRow(ctx, history).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err = provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.Pool.QueryRow(ctx, history).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("upgrade rewrote historical ledger evidence")
	}
	var buckets int
	if err = s.Pool.QueryRow(ctx, `SELECT count(*) FROM exchange_alert_rounding`).Scan(&buckets); err != nil || buckets != 0 {
		t.Fatalf("historical amount seeded credit: %d %v", buckets, err)
	}
	if err = chargeClientUsage(t, s, 1, 1, "A", "v1"); err != nil {
		t.Fatal(err)
	}
	if err = chargeClientUsage(t, s, 2, 1, "A", "v1"); err != nil {
		t.Fatal(err)
	}
	if _, err = provider.DownTo(ctx, 105); err == nil || !strings.Contains(err.Error(), "Cumulative alert billing exists") {
		t.Fatalf("unsafe Down accepted: %v", err)
	}
	var total int64
	if err = s.Pool.QueryRow(ctx, `SELECT sum(coins_minor) FROM exchange_alert_charges`).Scan(&total); err != nil || total != 5 {
		t.Fatalf("new epoch total=%d err=%v", total, err)
	}
}

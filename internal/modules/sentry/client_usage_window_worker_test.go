package sentry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"glorynavy.local/seat/internal/modules/exchange"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/testutil"
)

type windowWorkerBoundary struct {
	roundingWorkerSettlement
	exchange *exchange.Service
}

func (b *windowWorkerBoundary) SettleClientUsage(ctx context.Context, c ClientUsageSettlement) error {
	return b.exchange.SettleClientUsage(ctx, exchange.AlertTimeGrantRequest{ID: c.GrantID, AccountID: c.AccountID, SystemID: c.SystemID, RequestKey: c.ReserveKey, PriceVersion: c.PriceVersion, UnitSeconds: c.UnitSeconds, UnitPriceMinor: c.UnitPriceMinor, ReservedSeconds: c.DurationSeconds, ExpiresAt: c.ExpiresAt}, exchange.AlertTimeIntervalRequest{GrantID: c.GrantID, IntervalID: c.UsageID, RequestKey: c.IntervalKey, StartedAt: c.StartedAt, EndedAt: c.EndedAt}, c.SettleKey, func(ctx context.Context, tx pgx.Tx, replayOnly bool) (bool, error) {
		return GuardClientUsageTx(ctx, tx, c, replayOnly)
	})
}

func TestClientUsageWindowWorkerNeverDropsUnauditedRecords(t *testing.T) {
	pool := testutil.Database(t)
	ctx := context.Background()
	const account = "11111111-1111-4111-8111-111111111111"
	opened := time.Now().UTC().Truncate(time.Second).Add(-time.Hour)
	if _, err := pool.Exec(ctx, `INSERT INTO identity_users(id) VALUES($1)`, account); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sentry_alert_pricing(id,price_version,unit_seconds,unit_price_minor,max_grant_seconds,grant_ttl_seconds,updated_by,charging_enabled) VALUES(1,'v1',3600,200,7200,3600,$1,true)`, account); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sentry_alert_pricing_audit(actor_id,previous_version,version,before_snapshot,after_snapshot,created_at) VALUES($1,0,1,'{"charging_enabled":false}','{"charging_enabled":true}',$2)`, account, opened); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO exchange_coin_ledger(account_id,kind,reference,request_key,delta,reason) VALUES($1,'source','worker-test','99999999-1111-4111-8111-111111111111',100,'fixture')`, account); err != nil {
		t.Fatal(err)
	}
	usages := []ClientUsage{}
	for n, start := range []time.Time{opened.Add(-time.Hour), opened, opened.Add(time.Second)} {
		usages = append(usages, ClientUsage{UsageID: []string{"old", "valid", "unknown"}[n], AccountID: account, KeyID: "key", ClientID: "client", StartedAt: start.Format(time.RFC3339Nano), EndedAt: start.Add(time.Second).Format(time.RFC3339Nano), DurationSeconds: 1})
	}
	usages[1].SystemID = "30000142"
	remote := &roundingWorkerRemote{pages: map[string]ClientUsagePage{"": {Usage: usages, NextCursor: "complete"}}}
	boundary := &windowWorkerBoundary{exchange: &exchange.Service{Pool: pool, AllowAlertConsumption: true, LockAccounts: identity.New(pool).LockActiveAccounts}}
	service := &Service{Pool: pool, AlertEnabled: true, ClientUsageRemote: remote, AlertFunding: boundary, AlertSettlement: boundary}
	for _, invalid := range []any{nil, float64(0), "0"} {
		usages[2].SystemID = invalid
		if err := (&clientUsageWorker{runtime: service}).Work(ctx, nil); !errors.Is(err, exchange.ErrInvalid) {
			t.Fatalf("missing attribution was not rejected: %v", err)
		}
		if cursor, err := service.clientUsageCursor(ctx); err != nil || cursor != "" {
			t.Fatalf("cursor skipped unaudited interval: %q %v", cursor, err)
		}
	}
	var exclusions, charges int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM sentry_client_usage_exclusions),(SELECT count(*) FROM exchange_alert_charges)`).Scan(&exclusions, &charges); err != nil || exclusions != 1 || charges != 1 {
		t.Fatalf("page retry duplicated evidence: %d %d %v", exclusions, charges, err)
	}
	usages[2].SystemID = "30000142"
	var snooze *river.JobSnoozeError
	if err := (&clientUsageWorker{runtime: service}).Work(ctx, nil); !errors.As(err, &snooze) {
		t.Fatal(err)
	}
	if cursor, err := service.clientUsageCursor(ctx); err != nil || cursor != "complete" {
		t.Fatalf("completed evidence did not advance cursor: %q %v", cursor, err)
	}
	usages[0].DurationSeconds = 2
	usages[0].EndedAt = opened.Add(-time.Hour + 2*time.Second).Format(time.RFC3339Nano)
	remote.pages["complete"] = ClientUsagePage{Usage: usages[:1], NextCursor: "must-not-advance"}
	if err := (&clientUsageWorker{runtime: service}).Work(ctx, nil); err == nil {
		t.Fatal("excluded usage drift accepted")
	}
	if cursor, _ := service.clientUsageCursor(ctx); cursor != "complete" {
		t.Fatalf("drift changed cursor: %q", cursor)
	}
}

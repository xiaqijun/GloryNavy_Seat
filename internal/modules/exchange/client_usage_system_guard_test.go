package exchange

import (
	"context"
	"errors"
	"testing"
)

func TestClientUsageRejectsInvalidNewSystemWithoutWrites(t *testing.T) {
	s := clientUsageFixture(t)
	ctx := context.Background()
	for n, system := range []string{"", " ", "0", "-1", "+30000142", "030000142", "R-YWID", "30000142.0", "9223372036854775808"} {
		g, interval, key := clientUsageRequest(n, 1, "30000142", "v1")
		g.SystemID = system
		if err := s.SettleClientUsage(ctx, g, interval, key); !errors.Is(err, ErrInvalid) {
			t.Fatalf("system %q accepted or returned wrong error: %v", system, err)
		}
	}
	var rows int64
	if err := s.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM exchange_alert_grants)+(SELECT count(*) FROM exchange_alert_charges)+(SELECT count(*) FROM exchange_alert_rounding)+(SELECT count(*) FROM exchange_coin_ledger WHERE kind='alert_reserve')`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf("invalid attribution changed billing: %d rows", rows)
	}
	if err := chargeClientUsage(t, s, 100, 1, "30000142", "v1"); err != nil {
		t.Fatalf("valid system rejected: %v", err)
	}
}

func TestClientUsageSystemGuardPreservesSettledReplay(t *testing.T) {
	s := clientUsageFixture(t)
	ctx := context.Background()
	for n := 0; n < 3; n++ {
		g, interval, key := clientUsageRequest(n, 1, "", "legacy")
		if err := s.ReserveAlertTime(ctx, g); err != nil {
			t.Fatal(err)
		}
		if err := s.ReserveAlertInterval(ctx, interval); err != nil {
			t.Fatal(err)
		}
		if n < 2 {
			if err := s.SettleAlertTime(ctx, g.ID, interval.IntervalID, key); err != nil {
				t.Fatal(err)
			}
		}
		if n == 1 {
			if _, err := s.Pool.Exec(ctx, `UPDATE exchange_alert_grants SET cumulative_rounding=true WHERE id=$1`, g.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	const snapshot = `SELECT md5((SELECT string_agg(to_jsonb(g)::text,'' ORDER BY id) FROM exchange_alert_grants g) || (SELECT string_agg(to_jsonb(c)::text,'' ORDER BY id) FROM exchange_alert_charges c) || (SELECT string_agg(to_jsonb(l)::text,'' ORDER BY id) FROM exchange_coin_ledger l))`
	var before, after string
	if err := s.Pool.QueryRow(ctx, snapshot).Scan(&before); err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 3; n++ {
		g, interval, key := clientUsageRequest(n, 1, "", "new-price")
		err := s.SettleClientUsage(ctx, g, interval, key)
		if n < 2 && err != nil {
			t.Fatalf("settled replay rejected: %v", err)
		}
		if n == 2 && !errors.Is(err, ErrInvalid) {
			t.Fatalf("unsettled unknown-system usage accepted: %v", err)
		}
	}
	if err := s.Pool.QueryRow(ctx, snapshot).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("system guard rewrote previous billing evidence")
	}
}

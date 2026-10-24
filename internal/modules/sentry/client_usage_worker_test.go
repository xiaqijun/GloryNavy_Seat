package sentry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"glorynavy.local/seat/internal/testutil"
)

type roundingWorkerRemote struct{ pages map[string]ClientUsagePage }

func (r *roundingWorkerRemote) ListClientUsage(_ context.Context, cursor string, _ int) (ClientUsagePage, error) {
	return r.pages[cursor], nil
}

type roundingWorkerSettlement struct {
	fail     string
	received map[string]ClientUsageSettlement
}

var errRoundingWorker = errors.New("test settlement failure")

func (s *roundingWorkerSettlement) ReserveAlertTime(context.Context, string, string, string, string, int64, int64, int64, time.Time) error {
	return errors.New("legacy funding called")
}
func (s *roundingWorkerSettlement) ReserveAlertInterval(context.Context, string, string, string, time.Time, time.Time) error {
	return errors.New("legacy reservation called")
}
func (s *roundingWorkerSettlement) SettleAlertInterval(context.Context, string, string, string) error {
	return errors.New("legacy settlement called")
}
func (s *roundingWorkerSettlement) SettleClientUsage(_ context.Context, c ClientUsageSettlement) error {
	if s.fail == c.UsageID {
		s.fail = ""
		return errRoundingWorker
	}
	if old, ok := s.received[c.UsageID]; ok && (old.GrantID != c.GrantID || old.ReserveKey != c.ReserveKey || old.IntervalKey != c.IntervalKey || old.SettleKey != c.SettleKey) {
		return errors.New("idempotency keys changed")
	}
	s.received[c.UsageID] = c
	return nil
}

func TestClientUsageWorkerKeepsRawIDsAcrossPageRetryAndRestart(t *testing.T) {
	pool := testutil.Database(t)
	ctx := context.Background()
	const account = "11111111-1111-4111-8111-111111111111"
	if _, err := pool.Exec(ctx, `INSERT INTO identity_users(id) VALUES($1);`, account); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sentry_alert_pricing(id,price_version,unit_seconds,unit_price_minor,max_grant_seconds,grant_ttl_seconds,updated_by,charging_enabled) VALUES(1,'v1',3600,200,7200,3600,$1,true)`, account); err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)
	usages := make([]ClientUsage, 3)
	for n, id := range []string{"one", "two", "three"} {
		usages[n] = ClientUsage{UsageID: id, AccountID: account, KeyID: "key", ClientID: "client", SystemID: "30000142", DurationSeconds: 1, StartedAt: start.Add(time.Duration(n) * time.Second).Format(time.RFC3339), EndedAt: start.Add(time.Duration(n+1) * time.Second).Format(time.RFC3339)}
	}
	remote := &roundingWorkerRemote{pages: map[string]ClientUsagePage{"": {Usage: usages[:2], NextCursor: "page-one", HasMore: true}}}
	settlement := &roundingWorkerSettlement{fail: "two", received: map[string]ClientUsageSettlement{}}
	service := func() *Service {
		return &Service{Pool: pool, AlertEnabled: true, ClientUsageRemote: remote, AlertFunding: settlement, AlertSettlement: settlement}
	}
	if err := (&clientUsageWorker{runtime: service()}).Work(ctx, nil); !errors.Is(err, errRoundingWorker) {
		t.Fatalf("failure=%v", err)
	}
	var cursor string
	if cursor, _ = service().clientUsageCursor(ctx); cursor != "" {
		t.Fatalf("cursor advanced after failure: %q", cursor)
	}
	// Retry fetches a differently sized page; every raw ID must still use the
	// same deterministic keys, even when the worker process has been recreated.
	remote.pages[""] = ClientUsagePage{Usage: usages, NextCursor: "page-two"}
	var snooze *river.JobSnoozeError
	if err := (&clientUsageWorker{runtime: service()}).Work(ctx, nil); !errors.As(err, &snooze) {
		t.Fatalf("retry=%v", err)
	}
	if cursor, _ = service().clientUsageCursor(ctx); cursor != "page-two" {
		t.Fatalf("cursor=%q", cursor)
	}
	if len(settlement.received) != 3 {
		t.Fatalf("raw usage collapsed into page IDs: %#v", settlement.received)
	}
	for _, usage := range usages {
		c := settlement.received[usage.UsageID]
		if c.UsageID != usage.UsageID || c.DurationSeconds != 1 || c.SystemID != "30000142" || c.PriceVersion != "v1" {
			t.Fatalf("invalid settlement: %+v", c)
		}
	}
}

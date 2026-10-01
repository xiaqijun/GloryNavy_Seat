package esiclient

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"glorynavy.local/seat/migrations"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"glorynavy.local/seat/internal/modules/eve/internal/store"
	"glorynavy.local/seat/internal/testutil"
)

func rateResponse(used, remaining string) *http.Response {
	h := http.Header{}
	h.Set("X-Ratelimit-Group", "test")
	h.Set("X-Ratelimit-Limit", "20/15m")
	if used != "" {
		h.Set("X-Ratelimit-Used", used)
	}
	if remaining != "" {
		h.Set("X-Ratelimit-Remaining", remaining)
	}
	return &http.Response{StatusCode: 200, Header: h}
}

func TestSlidingReservationSettlementAndRestart(t *testing.T) {
	ctx := context.Background()
	pool := testutil.Database(t)
	s := New(pool, nil)
	q := store.New(pool)
	key := "group:test:public-egress"
	policy := routePolicy{Group: "test", Capacity: 20, WindowSeconds: 900}
	clearPacing := func() {
		t.Helper()
		if _, err := pool.Exec(ctx, "UPDATE eve_esi_limits SET next_request_at='epoch'"); err != nil {
			t.Fatal(err)
		}
	}
	reserve := func() reservation {
		t.Helper()
		clearPacing()
		r, err := s.reserve(ctx, key, policy)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	usage := func(want int64) {
		t.Helper()
		u, err := q.ESIBudgetUsage(ctx, key)
		if err != nil || u.Used != want {
			t.Fatalf("usage=%d want=%d err=%v", u.Used, want, err)
		}
	}
	for _, cost := range []int64{2, 1, 5, 0} {
		r := reserve()
		before, _ := q.ESIBudgetUsage(ctx, key)
		res := rateResponse(strconv.FormatInt(cost, 10), "")
		if err := s.settle(ctx, r, "GET /test/", "public-egress", res); err != nil {
			t.Fatal(err)
		}
		usage(before.Used - 5 + cost)
		if err := s.settle(ctx, r, "GET /test/", "public-egress", res); err != nil {
			t.Fatal(err)
		}
		usage(before.Used - 5 + cost)
	}
	usage(8)
	r := reserve()
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := s.settle(canceled, r, "GET /test/", "public-egress", nil); err != nil {
		t.Fatal(err)
	}
	usage(13)
	crash := reserve() // durable five-token reservation survives a new client
	usage(18)
	clearPacing()
	if _, err := New(pool, nil).reserve(ctx, key, policy); err == nil {
		t.Fatal("restart bypassed budget")
	}
	// Only the oldest two-token request ages out; later requests cannot extend it.
	if _, err := pool.Exec(ctx, "UPDATE eve_esi_charges SET expires_at=now()-interval '1 second' WHERE id=(SELECT min(id) FROM eve_esi_charges WHERE limit_key=$1)", key); err != nil {
		t.Fatal(err)
	}
	usage(16)
	clearPacing()
	if _, err := s.reserve(ctx, key, policy); err == nil {
		t.Fatal("partial expiry incorrectly refilled entire bucket")
	}
	if _, err := pool.Exec(ctx, "UPDATE eve_esi_charges SET expires_at=now()-interval '1 second' WHERE id=$1", crash.ID); err != nil {
		t.Fatal(err)
	}
	usage(11)
	_ = reserve()
	usage(16)
}

func TestSlidingHeaderReconciliationAndIsolation(t *testing.T) {
	ctx := context.Background()
	pool := testutil.Database(t)
	s := New(pool, nil)
	q := store.New(pool)
	key := "group:test:public-egress"
	p := routePolicy{Group: "test", Capacity: 20, WindowSeconds: 900}
	r, err := s.reserve(ctx, key, p)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.settle(ctx, r, "GET /test/", "public-egress", rateResponse("2", "10")); err != nil {
		t.Fatal(err)
	}
	u, _ := q.ESIBudgetUsage(ctx, key)
	if u.Used != 10 {
		t.Fatal("missing external debt", u)
	}
	if _, err = pool.Exec(ctx, "UPDATE eve_esi_limits SET next_request_at='epoch'"); err != nil {
		t.Fatal(err)
	}
	r, err = s.reserve(ctx, key, p)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.settle(ctx, r, "GET /test/", "public-egress", rateResponse("1", "19")); err != nil {
		t.Fatal(err)
	}
	u, _ = q.ESIBudgetUsage(ctx, key)
	if u.Used != 11 {
		t.Fatal("stale header created tokens", u)
	}
	for _, other := range []string{"group:test:character:1", "group:test:character:2", "group:other:public-egress"} {
		if _, err = s.reserve(ctx, other, p); err != nil {
			t.Fatal("independent bucket blocked", err)
		}
	}
	// New response groups move the same reservation, without counting it twice.
	if _, err = pool.Exec(ctx, "UPDATE eve_esi_limits SET next_request_at='epoch'"); err != nil {
		t.Fatal(err)
	}
	r, err = s.reserve(ctx, key, p)
	if err != nil {
		t.Fatal(err)
	}
	res := rateResponse("2", "")
	res.Header.Set("X-Ratelimit-Group", "discovered")
	if err = s.settle(ctx, r, "GET /changed/", "public-egress", res); err != nil {
		t.Fatal(err)
	}
	u, _ = q.ESIBudgetUsage(ctx, key)
	if u.Used != 11 {
		t.Fatal("old bucket still charged", u)
	}
	u, _ = q.ESIBudgetUsage(ctx, "group:discovered:public-egress")
	if u.Used != 2 {
		t.Fatal("new group charge", u)
	}
}

func TestSlidingConcurrentWorkersCannotOverspend(t *testing.T) {
	ctx := context.Background()
	pool := testutil.Database(t)
	key := "group:test:public-egress"
	// Remove pacing in this isolated test to stress the budget lock itself.
	if _, err := pool.Exec(ctx, `CREATE FUNCTION test_unpace() RETURNS trigger LANGUAGE plpgsql AS 'BEGIN NEW.next_request_at := ''epoch''; RETURN NEW; END'; CREATE TRIGGER test_unpace BEFORE UPDATE ON eve_esi_limits FOR EACH ROW EXECUTE FUNCTION test_unpace()`); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 20)
	for range 20 {
		wg.Go(func() {
			_, err := New(pool, nil).reserve(ctx, key, routePolicy{Group: "test", Capacity: 20, WindowSeconds: 900})
			results <- err
		})
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else {
			var retry RetryError
			if !errors.As(err, &retry) {
				t.Fatal(err)
			}
		}
	}
	if success != 4 {
		t.Fatalf("got %d reservations for 20-token budget", success)
	}
	u, err := store.New(pool).ESIBudgetUsage(ctx, key)
	if err != nil || u.Used != 20 {
		t.Fatal(u, err)
	}
}

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCatalogCacheAndNoInventedBucket(t *testing.T) {
	ctx := context.Background()
	pool := testutil.Database(t)
	s := New(pool, nil)
	calls := 0
	client := &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("X-Compatibility-Date") != CompatibilityDate {
			t.Error("compatibility header")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`[]`))}, nil
	})}
	var out any
	for range 2 {
		if _, err := s.Request(ctx, client, "POST", "/characters/affiliation/", "", []byte(`[1]`), &out); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatal("cache bypassed", calls)
	}
	var ttl float64
	if err := pool.QueryRow(ctx, "SELECT extract(epoch FROM (expires_at-updated_at))::float8 FROM eve_esi_cache").Scan(&ttl); err != nil || ttl < 3590 || ttl > 3601 {
		t.Fatal("catalog TTL", ttl, err)
	}
	var buckets int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM eve_esi_buckets").Scan(&buckets); err != nil || buckets != 0 {
		t.Fatal("invented bucket", buckets, err)
	}
	// Catalog knows status before first response, even if no headers were seen.
	if _, err := s.Request(ctx, client, "GET", "/status/", "", nil, &out); err != nil {
		t.Fatal(err)
	}
	rows, err := store.New(pool).ListESIBuckets(ctx, store.ListESIBucketsParams{})
	if err != nil || len(rows) != 1 || rows[0].GroupName != "status" || rows[0].Remaining.Valid {
		t.Fatal("catalog confused with measured balance", rows, err)
	}
	if _, err := pool.Exec(ctx, "UPDATE eve_esi_limits SET blocked_until=now()+interval '1 hour'"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Request(ctx, client, "GET", "/status/", "", nil, &out); err != nil {
		t.Fatal("fresh cache blocked", err)
	}
	if calls != 2 {
		t.Fatal("fresh cache consumed network budget")
	}
}

func TestCatalogAndRetryDelay(t *testing.T) {
	for _, path := range []string{"/characters/123/contracts/", "/characters/123/contracts/456/items/", "/characters/123/contracts/456/bids/"} {
		p := catalog[routeKey("GET", path)]
		if p.Group != "char-contract" || p.Capacity != 600 || p.WindowSeconds != 900 {
			t.Fatal(p)
		}
	}
	if p := catalog[routeKey("POST", "/characters/affiliation/")]; p.Group != "" || p.CacheSeconds != 3600 {
		t.Fatal(p)
	}
	now := time.Now().UTC().Truncate(time.Second)
	for _, tc := range []struct {
		key, value string
		seconds    int
	}{{"Retry-After", "17", 17}, {"X-Esi-Error-Limit-Reset", "5", 5}, {"Retry-After", "0", 0}, {"Retry-After", now.Add(8 * time.Second).Format(http.TimeFormat), 8}, {"Retry-After", "invalid", 60}} {
		h := http.Header{}
		h.Set(tc.key, tc.value)
		if got := RetryUntil(h, now).Sub(now); got != time.Duration(tc.seconds)*time.Second {
			t.Fatal(tc, got)
		}
	}
}

func TestConditionalFallbackAndNoStore(t *testing.T) {
	ctx := context.Background()
	pool := testutil.Database(t)
	s := New(pool, nil)
	modified := time.Now().UTC().Add(-time.Hour).Format(http.TimeFormat)
	calls := 0
	client := &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		calls++
		h := http.Header{"Last-Modified": {modified}, "Cache-Control": {"no-cache"}}
		status := 200
		if calls > 1 {
			if r.Header.Get("If-Modified-Since") != modified || r.Header.Get("If-None-Match") != "" {
				t.Error("missing Last-Modified conditional")
			}
			status = 304
			h.Set("Cache-Control", "no-store")
		}
		return &http.Response{StatusCode: status, Header: h, Body: io.NopCloser(strings.NewReader(`{"ok":true}`))}, nil
	})}
	for range 2 {
		var out map[string]bool
		if _, err := s.Request(ctx, client, "GET", "/test/", "", nil, &out); err != nil || !out["ok"] {
			t.Fatal(out, err)
		}
		if _, err := pool.Exec(ctx, "UPDATE eve_esi_limits SET next_request_at='epoch'"); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM eve_esi_cache").Scan(&count); err != nil || count != 0 {
		t.Fatal("no-store retained old response", count, err)
	}
}

func TestRuntimeWindowAndExactRetry(t *testing.T) {
	ctx := context.Background()
	pool := testutil.Database(t)
	s := New(pool, nil)
	key := "group:test:public-egress"
	r, err := s.reserve(ctx, key, routePolicy{Group: "test", Capacity: 20, WindowSeconds: 120})
	if err != nil {
		t.Fatal(err)
	}
	res := rateResponse("0", "20")
	res.StatusCode = 429
	res.Header.Set("X-Ratelimit-Limit", "20/2m")
	res.Header.Set("Retry-After", "17")
	if err = s.settle(ctx, r, "GET /test/", "public-egress", res); err != nil {
		t.Fatal(err)
	}
	row, err := store.New(pool).LockESILimit(ctx, key)
	if err != nil || row.WindowSeconds != 120 || time.Until(row.BlockedUntil.Time) > 18*time.Second || time.Until(row.BlockedUntil.Time) < 15*time.Second {
		t.Fatal("runtime retry/window", row, err)
	}
	if _, err = s.reserve(ctx, key, routePolicy{}); err == nil {
		t.Fatal("429 did not block")
	}
	res.StatusCode = 420
	res.Header = http.Header{"X-Esi-Error-Limit-Reset": {"5"}}
	if err = s.settle(ctx, reservation{Key: "route:unknown:public-egress"}, "GET /unknown/", "public-egress", res); err != nil {
		t.Fatal(err)
	}
	row, err = store.New(pool).LockESILimit(ctx, "egress")
	if err != nil || time.Until(row.BlockedUntil.Time) > 6*time.Second || time.Until(row.BlockedUntil.Time) < 3*time.Second {
		t.Fatal("legacy window was extended", row, err)
	}
}

func TestSlidingMigrationPreservesOutstandingBudget(t *testing.T) {
	ctx := context.Background()
	pool := testutil.Database(t)
	db := stdlib.OpenDB(*pool.Config().ConnConfig)
	defer db.Close()
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = provider.DownTo(ctx, 19); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO eve_esi_limits(limit_key,capacity,remaining,window_seconds,reset_at) VALUES ('group:legacy:public-egress',20,7,900,now()+interval '10 minutes'),('group:expired:public-egress',20,0,900,now()-interval '1 minute')`); err != nil {
		t.Fatal(err)
	}
	if _, err = provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	u, err := store.New(pool).ESIBudgetUsage(ctx, "group:legacy:public-egress")
	if err != nil || u.Used != 13 {
		t.Fatal("upgrade reset existing budget", u, err)
	}
	if delta := time.Until(u.NextExpiry.Time); delta < 590*time.Second || delta > 601*time.Second {
		t.Fatal("legacy expiry changed", delta)
	}
	u, err = store.New(pool).ESIBudgetUsage(ctx, "group:expired:public-egress")
	if err != nil || u.Used != 0 {
		t.Fatal("expired budget was recharged", u, err)
	}
}

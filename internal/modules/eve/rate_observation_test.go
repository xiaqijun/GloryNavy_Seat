package eve

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"glorynavy.local/seat/internal/modules/eve/internal/esiclient"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
)

func TestRateObservationRealHeadersCacheAndSharedGroup(t *testing.T) {
	s, ch := syncFixture(t)
	ctx := context.Background()
	if err := s.auth.Save(ctx, ch); err != nil {
		t.Fatal(err)
	}
	generation := authorizationTarget(t, s).Generation
	status, used := 200, "3" // Actual header overrides the usual status-based cost.
	remaining := "590"
	include := true
	calls := 0
	fail := false
	s.auth.esi.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if fail {
			return nil, errors.New("network failed")
		}
		res := esiResponse(map[string]bool{"ok": true})
		res.StatusCode = status
		res.Header.Set("ETag", "test-etag")
		if include {
			res.Header.Set("X-Ratelimit-Group", "corp-contract")
			res.Header.Set("X-Ratelimit-Limit", "600/15m")
			res.Header.Set("X-Ratelimit-Remaining", remaining)
			res.Header.Set("X-Ratelimit-Used", used)
		}
		if status == 429 {
			res.Header.Set("Retry-After", "17")
		}
		return res, nil
	})
	request := func(path string) {
		t.Helper()
		var out any
		_, _ = s.auth.ESI().Request(ctx, ESIRequest{Method: "GET", Path: path, CharacterID: ch.ID, Generation: generation, Scopes: []string{CorporationRolesScope}}, &out)
	}
	reset := func() {
		t.Helper()
		_, err := s.pool.Exec(ctx, "UPDATE eve_esi_limits SET next_request_at=now(),blocked_until=now(),remaining=500; UPDATE eve_esi_cache SET expires_at=now()-interval '1 minute'")
		if err != nil {
			t.Fatal(err)
		}
	}
	path := "/corporations/98530802/contracts/"
	request(path)
	request(path)
	q := store.New(s.pool)
	buckets, err := q.ListESIBuckets(ctx, store.ListESIBucketsParams{})
	if err != nil || len(buckets) != 1 {
		t.Fatal("bucket missing", err)
	}
	b := buckets[0]
	if b.Remaining.Int64 != 590 || b.LocalRemaining != 590 || b.UsedTokens != 3 || b.NetworkRequests != 1 || calls != 1 {
		t.Fatal("raw/local/cache accounting mixed", b)
	}
	reset()
	status = 304
	used = "1"
	remaining = "599"
	request(path)
	reset()
	status = 403
	used = "5"
	request("/corporations/98530802/contracts/236011673/items/?page=2")
	reset()
	status = 500
	used = "0"
	request(path)
	reset()
	status = 429
	request(path)
	// Local blocked request is not another upstream request or token charge.
	request(path)
	reset()
	status = 200
	include = false
	request(path)
	reset()
	fail = true
	request(path)
	buckets, err = q.ListESIBuckets(ctx, store.ListESIBucketsParams{})
	if err != nil || len(buckets) != 1 {
		t.Fatal(err)
	}
	b = buckets[0]
	if b.UsedTokens != 9 || b.NetworkRequests != 7 || b.UnmeasuredRequests != 2 {
		t.Fatal("consumption incorrect", b)
	}
	if b.Remaining.Valid {
		t.Fatal("missing header fabricated remaining")
	}
	if delta := b.RetryAt.Time.Sub(time.Now()); delta < 10*time.Second || delta > 18*time.Second {
		t.Fatal("Retry-After snapshot was clamped to local delay", delta)
	}
	rows, err := q.ListESIRouteUsage(ctx, store.ListESIRouteUsageParams{BucketID: b.ID})
	if err != nil || len(rows) != 2 {
		t.Fatal("same group interfaces not joined", err)
	}
	var sumCache, sumWait, sumLimits int64
	for _, r := range rows {
		sumCache += r.CacheHits
		sumWait += r.LocalWaits
		sumLimits += r.UpstreamLimits
		if strings.Contains(r.Route, "98530802") || strings.Contains(r.Route, "236011673") || strings.Contains(r.Route, "page=") {
			t.Fatal("raw path persisted")
		}
	}
	if sumCache != 1 || sumWait != 1 || sumLimits != 1 {
		t.Fatal("cache/wait/HTTP limit mixed", rows)
	}
	// Public requests are isolated even when ESI names the same route group.
	fail = false
	include = true
	status = 200
	used = "2"
	var out any
	publicCtx := context.WithValue(ctx, esiclient.CredentialKey{}, esiclient.Credential{ID: ch.ID, Generation: generation})
	if _, err := s.auth.ESI().Request(publicCtx, ESIRequest{Method: "GET", Path: "/status/"}, &out); err != nil {
		t.Fatal(err)
	}
	buckets, err = q.ListESIBuckets(ctx, store.ListESIBucketsParams{})
	if err != nil || len(buckets) != 2 || buckets[1].CharacterID != 0 {
		t.Fatal("public/private bucket sharing", err)
	}
	h := SyncHTTP{Service: s}
	router := chi.NewRouter()
	router.Get("/rate-limits", h.rateLimits)
	router.Get("/rate-limits/{id}/routes", h.rateRoutes)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("GET", "/rate-limits", nil))
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "test-access") || strings.Contains(rec.Body.String(), "limit_key") {
		t.Fatal("unsafe metadata response", rec.Code)
	}
	var decoded map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]int{"/rate-limits?after=bad": 400, "/rate-limits/999/routes": 404, "/rate-limits/1/routes?after=" + strings.Repeat("x", 301): 400} {
		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != want {
			t.Fatal(path, rec.Code)
		}
	}
}

func TestRateObservationRetentionAndSnapshotOrdering(t *testing.T) {
	s, _ := syncFixture(t)
	ctx := context.Background()
	q := store.New(s.pool)
	newer := timestamp(time.Now())
	older := timestamp(time.Now().Add(-time.Minute))
	args := store.ObserveESIBucketParams{LimitKey: "group:test:public-egress", GroupName: "test", HeaderAt: newer}
	id, err := q.ObserveESIBucket(ctx, args)
	if err != nil {
		t.Fatal(err)
	}
	args.HeaderAt = older
	if _, err = q.ObserveESIBucket(ctx, args); err != nil {
		t.Fatal(err)
	}
	rows, err := q.ListESIBuckets(ctx, store.ListESIBucketsParams{})
	if err != nil || len(rows) != 1 || rows[0].HeaderAt.Time.Before(newer.Time.Add(-time.Millisecond)) {
		t.Fatal("late observation replaced snapshot", err)
	}
	if _, err = s.pool.Exec(ctx, "UPDATE eve_esi_buckets SET updated_at=now()-interval '31 days' WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
	if err = q.CleanupESIBuckets(ctx); err != nil {
		t.Fatal(err)
	}
	exists, err := q.ESIBucketExists(ctx, id)
	if err != nil || exists {
		t.Fatal("inactive metadata not expired", err)
	}
}

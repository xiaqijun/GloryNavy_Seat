package eve

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"glorynavy.local/seat/internal/modules/eve/internal/esiclient"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func tokenRow(t *testing.T, s *SyncService) store.ListTokenObservationsRow {
	t.Helper()
	rows, err := store.New(s.pool).ListTokenObservations(context.Background(), store.ListTokenObservationsParams{})
	if err != nil || len(rows) != 1 {
		t.Fatal("observation missing", err)
	}
	return rows[0]
}

func TestTokenObservationRefreshFailureSurvivesRollbackAndRecovers(t *testing.T) {
	s, ch := syncFixture(t)
	ctx := context.Background()
	client, key, m := fixture(t)
	s.auth.client = client
	var logs bytes.Buffer
	s.auth.logger = slog.New(slog.NewTextHandler(&logs, nil))
	previous := client.http.Transport
	fail := true
	client.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != m.Token {
			return previous.RoundTrip(r)
		}
		if fail {
			res := jsonResponse(map[string]string{"error": "secret-upstream-payload"})
			res.StatusCode = 503
			return res, nil
		}
		claims := validClaims()
		claims.Scopes = []string{CorporationRolesScope}
		claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(time.Hour))
		return jsonResponse(tokenResponse{sign(t, key, claims), "secret-rotated-refresh", "Bearer"}), nil
	})
	ch.authorization.ExpiresAt = time.Now().Add(-time.Minute)
	if err := s.auth.Save(ctx, ch); err != nil {
		t.Fatal(err)
	}
	generation := authorizationTarget(t, s).Generation
	if _, err := s.auth.currentToken(ctx, ch.ID, generation, ""); !errors.Is(err, ErrSSO) {
		t.Fatal(err)
	}
	row := tokenRow(t, s)
	if row.RefreshFailures != 1 || row.ConsecutiveFailures != 1 || row.LastRefreshReason != "sso_unavailable" || row.LastRefreshSuccessAt.Valid {
		t.Fatal("refresh failure not durable", row)
	}
	if tokenDTO(row, time.Now()).State != "refresh_failed" {
		t.Fatal("refresh error hidden")
	}
	fail = false
	if _, err := s.auth.currentToken(ctx, ch.ID, generation, ""); err != nil {
		t.Fatal(err)
	}
	row = tokenRow(t, s)
	if row.RefreshFailures != 1 || row.RefreshSuccesses != 1 || row.ConsecutiveFailures != 0 || row.LastRefreshReason != "" || !row.LastRefreshSuccessAt.Valid {
		t.Fatal("refresh recovery missing")
	}
	events, err := store.New(s.pool).ListTokenEvents(ctx, ch.ID)
	if err != nil || len(events) != 3 || events[0].Outcome != "refresh_success" || events[1].Outcome != "refresh_failed" {
		t.Fatal("timeline incorrect", err)
	}
	if strings.Contains(logs.String(), "secret-") || strings.Contains(logs.String(), ch.authorization.RefreshToken) {
		t.Fatal("refresh log leaked secrets")
	}
}

func TestTokenObservationRequestCountersAndGrantFence(t *testing.T) {
	s, ch := syncFixture(t)
	ctx := context.Background()
	if err := s.auth.Save(ctx, ch); err != nil {
		t.Fatal(err)
	}
	generation := authorizationTarget(t, s).Generation
	calls := 0
	s.auth.esi.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("Authorization") != "Bearer test-access" {
			t.Error("missing credential")
		}
		return esiResponse(map[string]bool{"ok": true}), nil
	})
	req := ESIRequest{Method: "GET", Path: "/characters/123/roles/", CharacterID: ch.ID, Generation: generation, Scopes: []string{CorporationRolesScope}}
	var result map[string]bool
	for range 2 {
		if _, err := s.auth.ESI().Request(ctx, req, &result); err != nil {
			t.Fatal(err)
		}
	}
	row := tokenRow(t, s)
	if calls != 1 || row.NetworkRequests != 1 || row.CacheHits != 1 || row.ReuseCount != 2 {
		t.Fatal("cache/reuse not separated")
	}
	if _, err := s.pool.Exec(ctx, "UPDATE eve_esi_limits SET blocked_until=now()+interval '5 minutes'"); err != nil {
		t.Fatal(err)
	}
	req.Path = "/characters/123/roles/?alternate=1"
	var retry retryError
	if _, err := s.auth.ESI().Request(ctx, req, &result); !errors.As(err, &retry) {
		t.Fatal("budget ignored", err)
	}
	row = tokenRow(t, s)
	if row.RateLimitWaits != 1 || row.NetworkRequests != 1 || row.RequestFailures != 0 {
		t.Fatal("local wait counted as a failed/network request")
	}
	if err := s.auth.Save(ctx, ch); err != nil {
		t.Fatal(err)
	}
	s.auth.observeESI(ctx, esiclient.Credential{ID: ch.ID, Generation: generation}, esiclient.RequestEvent{Network: true, Failed: true, Status: 401})
	s.auth.observeRefreshFailure(ctx, ch.ID, generation, time.Now(), "sso_unavailable")
	row = tokenRow(t, s)
	if row.NetworkRequests != 0 || row.RefreshFailures != 0 || row.ReuseCount != 0 || row.Generation == generation {
		t.Fatal("old grant contaminated new telemetry")
	}
	req.Generation = generation
	if _, err := s.auth.ESI().Request(ctx, req, &result); err == nil {
		t.Fatal("old grant requested data")
	}
	if calls != 1 {
		t.Fatal("obsolete request reached ESI")
	}
	if err := store.New(s.pool).DeleteCredential(ctx, ch.ID); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.pool.QueryRow(ctx, "SELECT (SELECT count(*) FROM eve_token_observations)+(SELECT count(*) FROM eve_token_events)").Scan(&count); err != nil || count != 0 {
		t.Fatal("unlink left usable observation", err)
	}
}

func TestESIGatewayRetries401OnceAndPreserves403(t *testing.T) {
	s, ch := syncFixture(t)
	ctx := context.Background()
	client, key, m := fixture(t)
	s.auth.client = client
	previous := client.http.Transport
	refreshes := 0
	client.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != m.Token {
			return previous.RoundTrip(r)
		}
		refreshes++
		claims := validClaims()
		claims.Scopes = []string{CorporationRolesScope}
		claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(time.Hour))
		if _, err := s.pool.Exec(ctx, "UPDATE eve_esi_limits SET next_request_at=now()"); err != nil {
			t.Fatal(err)
		}
		return jsonResponse(tokenResponse{sign(t, key, claims), "rotated-refresh", "Bearer"}), nil
	})
	if err := s.auth.Save(ctx, ch); err != nil {
		t.Fatal(err)
	}
	calls := 0
	status := 401
	s.auth.esi.http.Transport = transportFunc(func(*http.Request) (*http.Response, error) {
		calls++
		res := jsonResponse(nil)
		res.StatusCode = status
		return res, nil
	})
	req := ESIRequest{Method: "GET", Path: "/characters/123/roles/", CharacterID: ch.ID, Generation: authorizationTarget(t, s).Generation, Scopes: []string{CorporationRolesScope}}
	var out any
	var fault syncFault
	if _, err := s.auth.ESI().Request(ctx, req, &out); !errors.As(err, &fault) || fault.Status != 401 {
		t.Fatal("unexpected retry result", err)
	}
	if calls != 2 || refreshes != 1 {
		t.Fatal("401 retry was not bounded", calls, refreshes)
	}
	status = 403
	req.Path = "/denied/"
	if _, err := s.auth.ESI().Request(ctx, req, &out); !errors.As(err, &fault) || fault.Status != 403 {
		t.Fatal(err)
	}
	credential, err := store.New(s.pool).GetCredential(ctx, ch.ID)
	if err != nil || credential.State == "reauthorize" || len(credential.Sealed) == 0 || refreshes != 1 {
		t.Fatal("403 revoked/refreshed credential")
	}
	row := tokenRow(t, s)
	if row.NetworkRequests != 3 || row.RequestFailures != 3 || row.LastRequestStatus != 403 || row.RefreshSuccesses != 1 {
		t.Fatal("HTTP diagnostics incorrect")
	}
}

func TestTokenHTTPMetadataAllowlistAndReadOnlyRoutes(t *testing.T) {
	s, ch := syncFixture(t)
	ctx := context.Background()
	if err := s.auth.Save(ctx, ch); err != nil {
		t.Fatal(err)
	}
	h := SyncHTTP{Service: s}
	router := chi.NewRouter()
	for _, route := range h.Routes() {
		if strings.Contains(route.Path, "/tokens") {
			if route.Permission != "eve.sync.manage" || route.Public || route.Method != "GET" {
				t.Fatal("unsafe observation route")
			}
			router.Method(route.Method, "/api/v1/eve"+route.Path, route.Handler)
		}
	}
	for _, path := range []string{"/sync/tokens", "/sync/tokens/123/events"} {
		res := httptest.NewRecorder()
		router.ServeHTTP(res, httptest.NewRequest("GET", "/api/v1/eve"+path, nil))
		if res.Code != 200 {
			t.Fatal(res.Code, res.Body.String())
		}
		for _, secret := range []string{"test-access", "test-refresh", "sealed", "owner_hash", "refresh_token", "access_token"} {
			if strings.Contains(res.Body.String(), secret) {
				t.Fatal("metadata leak", secret)
			}
		}
	}
	for _, tc := range []struct {
		path string
		code int
	}{{"/sync/tokens?state=bad", 400}, {"/sync/tokens?after=-1", 400}, {"/sync/tokens/999/events", 404}, {"/sync/tokens/0123/events", 404}} {
		res := httptest.NewRecorder()
		router.ServeHTTP(res, httptest.NewRequest("GET", "/api/v1/eve"+tc.path, nil))
		if res.Code != tc.code {
			t.Fatal(tc.path, res.Code)
		}
	}
	if _, err := s.pool.Exec(ctx, "UPDATE eve_token_observations SET access_expires_at=NULL,observed_since=NULL"); err != nil {
		t.Fatal(err)
	}
	if tokenDTO(tokenRow(t, s), time.Now()).State != "unknown" {
		t.Fatal("legacy token incorrectly marked valid")
	}
	if _, err := s.pool.Exec(ctx, "UPDATE eve_token_events SET occurred_at=now()-interval '31 days'"); err != nil {
		t.Fatal(err)
	}
	if err := qCleanup(ctx, s.pool); err != nil {
		t.Fatal(err)
	}
	events, err := store.New(s.pool).ListTokenEvents(ctx, ch.ID)
	if err != nil || len(events) != 0 {
		t.Fatal("event retention failed", err)
	}
}

func TestESIGatewayRejectsUnsafeRequestsBeforeNetwork(t *testing.T) {
	// No pool is needed: malformed requests must fail before cache/credential access.
	c := newESI(nil, nil)
	c.http.Transport = transportFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid request reached network")
		return nil, io.EOF
	})
	for _, req := range []ESIRequest{{Method: "GET", Path: "@other.example/"}, {Method: "DELETE", Path: "/characters/123/"}, {Method: "GET", Path: "//other.example/"}, {Method: "GET", Path: "/test/#fragment"}, {Method: "GET", Path: "/test/", CharacterID: 123}} {
		var out any
		if _, err := c.Request(context.Background(), req, &out); err == nil {
			t.Fatal("invalid request accepted")
		}
	}
	raw, _ := json.Marshal(ESIRequest{})
	if strings.Contains(string(raw), "Token") {
		t.Fatal("public service accepts token material")
	}
}

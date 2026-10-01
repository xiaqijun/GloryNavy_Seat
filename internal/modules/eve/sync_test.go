package eve

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"github.com/golang-jwt/jwt/v5"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/riverqueue/river"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
	"glorynavy.local/seat/internal/testutil"
)

func syncFixture(t *testing.T) (*SyncService, Character) {
	t.Helper()
	pool := testutil.Database(t)
	client, _, _ := fixture(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	auth, err := NewAuthorization(pool, client, base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)), logger)
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewSync(pool, auth, logger, nil)
	if err != nil {
		t.Fatal(err)
	}
	ch := Character{ID: 123, Name: "Pilot", Owner: "owner", authorization: &authorization{AccessToken: "test-access", RefreshToken: "test-refresh", ExpiresAt: time.Now().Add(time.Hour), Scopes: []string{CorporationRolesScope}}}
	return service, ch
}
func authorizationTarget(t *testing.T, s *SyncService) store.EveSyncTarget {
	t.Helper()
	rows, err := store.New(s.pool).ListCharacterSync(context.Background(), 123)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.Resource == "authorization" {
			return row
		}
	}
	t.Fatal("authorization target missing")
	return store.EveSyncTarget{}
}
func esiResponse(v any) *http.Response {
	r := jsonResponse(v)
	r.Header.Set("Expires", time.Now().Add(10*time.Minute).UTC().Format(http.TimeFormat))
	return r
}

func TestSyncTransactionalDispatchAndDuplicateScanners(t *testing.T) {
	s, ch := syncFixture(t)
	ctx := context.Background()
	tx, _ := s.pool.Begin(ctx)
	if err := s.auth.SaveTx(ctx, tx, ch); err != nil {
		t.Fatal(err)
	}
	_ = tx.Rollback(ctx)
	var count int
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM river_job").Scan(&count); err != nil || count != 0 {
		t.Fatal("rolled back credential created jobs", err, count)
	}
	if err := s.auth.Save(ctx, ch); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			if err := s.dispatch(ctx); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM river_job").Scan(&count); err != nil || count != 2 {
		t.Fatal("duplicate jobs", err, count)
	}
}

func TestDueOnlineTargetPrecedesOlderBackgroundResource(t *testing.T) {
	s, ch := syncFixture(t)
	ctx := context.Background()
	ch.authorization.Scopes = append(ch.authorization.Scopes, OnlineReadScope)
	if err := s.auth.Save(ctx, ch); err != nil {
		t.Fatal(err)
	}
	if err := store.New(s.pool).SeedOnlineTargets(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE eve_sync_targets SET next_due_at=now()-interval '10 minutes' WHERE resource='authorization'`); err != nil {
		t.Fatal(err)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	rows, err := store.New(tx).LockDueTargets(ctx, store.LockDueTargetsParams{OnlineEnabled: true})
	if err != nil || len(rows) == 0 || rows[0].Resource != "online" {
		t.Fatal("online observation delayed behind background work", rows, err)
	}
}

func TestSyncWorkerCommitsAndReplaysWithoutESI(t *testing.T) {
	s, ch := syncFixture(t)
	ctx := context.Background()
	var calls atomic.Int32
	s.auth.esi.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		switch r.URL.Path {
		case "/characters/affiliation/":
			return esiResponse([]map[string]int64{{"character_id": 123, "corporation_id": 10}}), nil
		case "/characters/123/roles/":
			return esiResponse(roleData{Roles: []string{"Director"}}), nil
		case "/corporations/10/":
			return esiResponse(corporationData{Name: "Corp", CEO: 999, Alliance: 100}), nil
		}
		t.Errorf("unexpected path %s", r.URL.Path)
		return nil, errESI
	})
	if err := s.auth.Save(ctx, ch); err != nil {
		t.Fatal(err)
	}
	target := authorizationTarget(t, s)
	args := syncArgs{target.ID, target.Generation}
	if err := s.work(ctx, args, target.ActiveJobID.Int64, "authorization"); err != nil {
		t.Fatal(err)
	}
	got, err := s.auth.Get(ctx, ch.ID)
	if err != nil || got.State != "ready" || len(got.Roles) != 1 {
		t.Fatalf("snapshot missing: %+v %v", got, err)
	}
	if err = s.work(ctx, args, target.ActiveJobID.Int64, "authorization"); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Fatalf("replay requested ESI again: %d", calls.Load())
	}
	var raw []byte
	if err = s.pool.QueryRow(ctx, "SELECT body FROM eve_esi_cache WHERE character_id=123").Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("Director")) || bytes.Contains(raw, []byte("test-access")) {
		t.Fatal("private cache stored plaintext")
	}
}

func TestReauthorizationAndLeaseFenceRejectOldResult(t *testing.T) {
	s, ch := syncFixture(t)
	ctx := context.Background()
	if err := s.auth.Save(ctx, ch); err != nil {
		t.Fatal(err)
	}
	old := authorizationTarget(t, s)
	q := store.New(s.pool)
	c, err := q.GetCredential(ctx, ch.ID)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := q.ClaimSync(ctx, store.ClaimSyncParams{ID: old.ID, Generation: old.Generation, ActiveJobID: old.ActiveJobID})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.auth.Save(ctx, ch); err != nil {
		t.Fatal(err)
	}
	result := syncResult{snapshot: &esiSnapshot{CorporationID: 10, Corporation: corporationData{Name: "Old", CEO: 999}, Roles: roleData{Roles: []string{"Director"}}}, next: time.Now().Add(time.Hour)}
	if err = s.finish(ctx, claimed, c, old.ActiveJobID.Int64, result, nil); err != nil {
		t.Fatal(err)
	}
	got, _ := s.auth.Get(ctx, ch.ID)
	if len(got.Roles) > 0 {
		t.Fatal("old generation published facts")
	}
	now := authorizationTarget(t, s)
	if now.Generation <= old.Generation {
		t.Fatal("generation did not change")
	}
	current, _ := q.GetCredential(ctx, ch.ID)
	claim, err := q.ClaimSync(ctx, store.ClaimSyncParams{ID: now.ID, Generation: now.Generation, ActiveJobID: now.ActiveJobID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.pool.Exec(ctx, "UPDATE eve_sync_targets SET lease_until=now()-interval '1 second' WHERE id=$1", now.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.finish(ctx, claim, current, now.ActiveJobID.Int64, result, nil); err != nil {
		t.Fatal(err)
	}
	got, _ = s.auth.Get(ctx, ch.ID)
	if len(got.Roles) > 0 {
		t.Fatal("expired lease published facts")
	}
}

func TestESIConditionalCacheAndSharedRateLimit(t *testing.T) {
	s, _ := syncFixture(t)
	ctx := context.Background()
	var calls int
	s.auth.esi.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			res := esiResponse(map[string]string{"name": "unchanged"})
			res.Header.Set("ETag", "version-1")
			return res, nil
		}
		if r.Header.Get("If-None-Match") != "version-1" {
			t.Error("missing ETag")
		}
		res := esiResponse(nil)
		res.StatusCode = 304
		return res, nil
	})
	var value map[string]string
	if _, err := s.auth.esi.request(ctx, "GET", "/test/", "", nil, &value); err != nil {
		t.Fatal(err)
	}
	if _, err := s.auth.esi.request(ctx, "GET", "/test/", "", nil, &value); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("fresh cache requested ESI")
	}
	_, err := s.pool.Exec(ctx, "UPDATE eve_esi_cache SET expires_at=now()-interval '1 second'; UPDATE eve_esi_limits SET next_request_at=now()")
	if err != nil {
		t.Fatal(err)
	}
	var before time.Time
	_ = s.pool.QueryRow(ctx, "SELECT content_updated_at FROM eve_esi_cache").Scan(&before)
	if _, err = s.auth.esi.request(ctx, "GET", "/test/", "", nil, &value); err != nil {
		t.Fatal(err)
	}
	var after time.Time
	_ = s.pool.QueryRow(ctx, "SELECT content_updated_at FROM eve_esi_cache").Scan(&after)
	if value["name"] != "unchanged" || !before.Equal(after) {
		t.Fatal("304 changed content/version")
	}
	if err = store.New(s.pool).BlockESILimit(ctx, store.BlockESILimitParams{LimitKey: "egress", BlockedUntil: timestamp(time.Now().Add(time.Minute))}); err != nil {
		t.Fatal(err)
	}
	other := newESI(s.pool, s.auth.box)
	var retry retryError
	if err = other.shared.Reserve(ctx, "another-route"); !errors.As(err, &retry) {
		t.Fatal("another worker bypassed shared limit", err)
	}
}

func TestAuthorizationCannotRenewTrustFromOldCache(t *testing.T) {
	s, _ := syncFixture(t)
	ctx := context.Background()
	calls := 0
	s.auth.esi.http.Transport = transportFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return esiResponse(map[string]string{"name": "cached"}), nil
	})
	var value map[string]string
	if _, err := s.auth.esi.request(ctx, "GET", "/test/", "", nil, &value); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, "UPDATE eve_esi_cache SET updated_at=now()-interval '3 hours',expires_at=now()+interval '1 hour'"); err != nil {
		t.Fatal(err)
	}
	ctx = context.WithValue(ctx, esiObservationKey{}, &esiObservation{LimitTrust: true})
	var retry retryError
	if _, err := s.auth.esi.request(ctx, "GET", "/test/", "", nil, &value); !errors.As(err, &retry) {
		t.Fatal("old cache renewed authorization trust", err)
	}
	if calls != 1 {
		t.Fatal("requested ESI before upstream cache expired")
	}
}

func TestESI403BlocksResourceWithoutDeletingCredential(t *testing.T) {
	s, ch := syncFixture(t)
	ctx := context.Background()
	if err := s.auth.Save(ctx, ch); err != nil {
		t.Fatal(err)
	}
	s.auth.esi.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/characters/affiliation/" {
			return esiResponse([]map[string]int64{{"character_id": 123, "corporation_id": 10}}), nil
		}
		res := esiResponse(map[string]string{"error": "forbidden"})
		res.StatusCode = 403
		return res, nil
	})
	target := authorizationTarget(t, s)
	if err := s.work(ctx, syncArgs{target.ID, target.Generation}, target.ActiveJobID.Int64, "authorization"); err != nil {
		t.Fatal(err)
	}
	target = authorizationTarget(t, s)
	credential, err := store.New(s.pool).GetCredential(ctx, 123)
	if err != nil || target.State != "blocked" || target.Reason != "access_denied" || len(credential.Sealed) == 0 {
		t.Fatal("403 incorrectly revoked credential", err, target.State, target.Reason)
	}
}

func TestSyncHTTPRejectsForeignCharacterAndRespectsDueTime(t *testing.T) {
	s, ch := syncFixture(t)
	ctx := context.Background()
	s.SetSkillsEnabled(true)
	s.SetLossesEnabled(true)
	ch.authorization.Scopes = append(ch.authorization.Scopes, LossReadScope)
	ch.authorization.Scopes = append(ch.authorization.Scopes, SkillsReadScope, "esi-skills.read_skillqueue.v1")
	if err := s.auth.Save(ctx, ch); err != nil {
		t.Fatal(err)
	}
	if err := s.dispatch(ctx); err != nil {
		t.Fatal(err)
	}
	s.available.Store(true)
	h := SyncHTTP{Service: s, User: func(*http.Request) string { return "01994763-4111-7000-8000-111111111111" }, Owns: func(_ context.Context, _ string, id int64) (bool, error) { return id == 123, nil }}
	router := chi.NewRouter()
	for _, route := range h.Routes() {
		router.Method(route.Method, "/api/v1/eve"+route.Path, route.Handler)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest("GET", "/api/v1/eve/sync/characters/999", nil))
	if recorder.Code != 404 {
		t.Fatal("foreign character exposed", recorder.Code)
	}
	_, err := s.pool.Exec(ctx, "UPDATE eve_sync_targets SET next_due_at=now()+interval '30 minutes'")
	if err != nil {
		t.Fatal(err)
	}
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest("POST", "/api/v1/eve/sync/characters/123/refresh", strings.NewReader(`{"resource":"profile"}`)))
	if recorder.Code != 202 || !strings.Contains(recorder.Body.String(), "already_pending") {
		t.Fatal("did not reuse active task", recorder.Code, recorder.Body.String())
	}
	var due bool
	_ = s.pool.QueryRow(ctx, "SELECT bool_and(next_due_at>now()) FROM eve_sync_targets").Scan(&due)
	if !due {
		t.Fatal("manual refresh bypassed cache")
	}
	for _, resource := range []string{"skillqueue", "killmails", "unknown"} {
		recorder = httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest("POST", "/api/v1/eve/sync/characters/123/refresh", strings.NewReader(`{"resource":"`+resource+`"}`)))
		if resource != "unknown" && (recorder.Code != 202 || !strings.Contains(recorder.Body.String(), "already_pending")) {
			t.Fatal("training queue rejected", recorder.Code, recorder.Body.String())
		}
		if resource == "unknown" && recorder.Code != 400 {
			t.Fatal("unknown resource accepted", recorder.Code)
		}
	}
}

func TestSyncRiverProcessesJobsAfterRestart(t *testing.T) {
	s, ch := syncFixture(t)
	ctx := context.Background()
	s.auth.esi.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/characters/123/":
			return esiResponse(characterProfile{Name: "Pilot", Corporation: 10}), nil
		case "/characters/affiliation/":
			return esiResponse([]map[string]int64{{"character_id": 123, "corporation_id": 10}}), nil
		case "/characters/123/roles/":
			return esiResponse(roleData{Roles: []string{}}), nil
		default:
			return esiResponse(corporationData{Name: "Corp", CEO: 999}), nil
		}
	})
	if err := s.auth.Save(ctx, ch); err != nil {
		t.Fatal(err)
	}
	// A freshly constructed runtime consumes work inserted by the prior runtime.
	next, err := NewSync(s.pool, s.auth, s.logger, nil)
	if err != nil {
		t.Fatal(err)
	}
	events, unsubscribe := next.queue.Subscribe(river.EventKindJobCompleted)
	defer unsubscribe()
	if err = next.queue.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() {
		stop, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		_ = next.queue.StopAndCancel(stop)
	}()
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	completed := 0
	for completed < 2 {
		select {
		case event := <-events:
			if event.Job.Kind == "eve.character-profile.v1" || event.Job.Kind == "eve.character-authorization.v1" {
				completed++
			}
		case <-deadline.C:
			t.Fatal("persisted jobs did not complete after restart")
		}
	}
	var count int
	if err = s.pool.QueryRow(ctx, "SELECT count(*) FROM eve_sync_targets WHERE last_success_at IS NOT NULL").Scan(&count); err != nil || count != 2 {
		t.Fatal("missing completed resources", count, err)
	}
}

func TestSyncRefreshSerializedAndSavedBeforeESIFailure(t *testing.T) {
	s, ch := syncFixture(t)
	ctx := context.Background()
	client, key, m := fixture(t)
	s.auth.client = client
	var refreshes atomic.Int32
	previous := client.http.Transport
	client.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != m.Token {
			return previous.RoundTrip(r)
		}
		refreshes.Add(1)
		claims := validClaims()
		claims.Scopes = []string{CorporationRolesScope}
		claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(time.Hour))
		return jsonResponse(tokenResponse{sign(t, key, claims), "rotated-refresh", "Bearer"}), nil
	})
	ch.authorization.ExpiresAt = time.Now().Add(-time.Minute)
	if err := s.auth.Save(ctx, ch); err != nil {
		t.Fatal(err)
	}
	target := authorizationTarget(t, s)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			if _, err := s.auth.currentToken(ctx, ch.ID, target.Generation, ""); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if refreshes.Load() != 1 {
		t.Fatal("concurrent refresh duplicated", refreshes.Load())
	}
	observations, err := store.New(s.pool).ListTokenObservations(ctx, store.ListTokenObservationsParams{})
	if err != nil || len(observations) != 1 || observations[0].RefreshSuccesses != 1 || observations[0].ReuseCount != 1 || !observations[0].AccessExpiresAt.Time.After(time.Now()) {
		t.Fatal("serialized token observation incorrect", err)
	}
	s.auth.esi.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/characters/affiliation/" {
			return esiResponse([]map[string]int64{{"character_id": 123, "corporation_id": 10}}), nil
		}
		res := esiResponse(nil)
		res.StatusCode = 503
		return res, nil
	})
	var snooze *river.JobSnoozeError
	if err := s.work(ctx, syncArgs{target.ID, target.Generation}, target.ActiveJobID.Int64, "authorization"); !errors.As(err, &snooze) {
		t.Fatal("temporary failure not delayed", err)
	}
	credential, err := store.New(s.pool).GetCredential(ctx, ch.ID)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := s.auth.open(ch.ID, credential.OwnerHash, credential.Sealed)
	if err != nil || saved.RefreshToken != "rotated-refresh" {
		t.Fatal("rotation lost after ESI failure", err)
	}
}

func TestSyncCorporationMoveInvalidatesOldFactsAndWaits(t *testing.T) {
	s, ch := syncFixture(t)
	ctx := context.Background()
	if err := s.auth.Save(ctx, ch); err != nil {
		t.Fatal(err)
	}
	_, err := s.pool.Exec(ctx, `INSERT INTO eve_role_snapshots(character_id,owner_hash,corporation_id,corporation_name,alliance_id,ceo_id,roles,roles_at_hq,roles_at_base,roles_at_other,synced_at,valid_until)
 SELECT character_id,owner_hash,10,'Old Corp',0,999,ARRAY['Director'],'{}','{}','{}',now(),now()+interval '1 hour' FROM eve_credentials`)
	if err != nil {
		t.Fatal(err)
	}
	s.auth.esi.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/characters/affiliation/":
			return esiResponse([]map[string]int64{{"character_id": 123, "corporation_id": 20}}), nil
		case "/characters/123/roles/":
			return esiResponse(roleData{Roles: []string{"Director"}}), nil
		default:
			return esiResponse(corporationData{Name: "New Corp", CEO: 999}), nil
		}
	})
	target := authorizationTarget(t, s)
	var snooze *river.JobSnoozeError
	if err = s.work(ctx, syncArgs{target.ID, target.Generation}, target.ActiveJobID.Int64, "authorization"); !errors.As(err, &snooze) {
		t.Fatal("move not deferred", err)
	}
	auth, _ := s.auth.Get(ctx, ch.ID)
	if len(auth.Roles) != 0 || auth.CorporationID != 0 {
		t.Fatal("old roles crossed corporation boundary")
	}
	credential, _ := store.New(s.pool).GetCredential(ctx, ch.ID)
	if !credential.RolesNotBefore.Time.After(time.Now().Add(9 * time.Minute)) {
		t.Fatal("cache barrier not saved")
	}
	if err = s.auth.Save(ctx, ch); err != nil {
		t.Fatal(err)
	}
	credential, _ = store.New(s.pool).GetCredential(ctx, ch.ID)
	if !credential.RolesNotBefore.Time.After(time.Now()) {
		t.Fatal("reauthorization bypassed corporation barrier")
	}
}

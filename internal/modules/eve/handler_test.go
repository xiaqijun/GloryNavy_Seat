package eve

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/module"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/testutil"
)

type fakeSSO struct {
	exchanges int
	verifier  string
	fail      bool
	required  []string
	granted   []string
	id        int64
}

func (f *fakeSSO) RequestedScopes() []string {
	if f.required == nil {
		return []string{}
	}
	return f.required
}

func (f *fakeSSO) Configured() bool { return true }
func (f *fakeSSO) AuthorizationURL(_ context.Context, state, verifier string) (string, error) {
	f.verifier = verifier
	return "https://login.eveonline.com/authorize?state=" + state, nil
}
func (f *fakeSSO) Exchange(_ context.Context, code, verifier string) (Character, error) {
	f.exchanges++
	if verifier != f.verifier || f.fail {
		return Character{}, ErrSSO
	}
	id := f.id
	if id == 0 {
		id = 123
	}
	return Character{ID: id, Name: "Pilot", Owner: "owner", authorization: &authorization{Scopes: f.granted, RefreshToken: "mock-refresh", AccessToken: "mock-access"}}, nil
}

func TestBrowserLoginFlow(t *testing.T) {
	pool := testutil.Database(t)
	sso := &fakeSSO{}
	accounts := identity.New(pool)
	origin := "https://example.com"
	h := NewHandler(pool, sso, func(ctx context.Context, c Character, previous string) (string, error) {
		return accounts.SignIn(ctx, c.ID, c.Name, c.Owner, previous)
	}, origin, true, slog.New(slog.NewTextHandler(io.Discard, nil)))
	auth := identity.Handler{Service: accounts, Origin: origin, Secure: true}
	reg, err := module.New([]module.Definition{h.Module(), auth.Module()}, []string{"eve", "identity"}, auth.Authorize)
	if err != nil {
		t.Fatal(err)
	}
	router := httpapi.New(slog.New(slog.NewTextHandler(io.Discard, nil)), reg, http.NotFoundHandler())
	request := func(method, path string, cookies []*http.Cookie, originHeader string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		if originHeader != "" {
			req.Header.Set("Origin", originHeader)
		}
		for _, c := range cookies {
			req.AddCookie(c)
		}
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		return res
	}
	if res := request("POST", "/api/v1/eve/login", nil, "https://evil.example"); res.Code != 403 {
		t.Fatal("cross-site login accepted")
	}
	start := func() (string, []*http.Cookie) {
		res := request("POST", "/api/v1/eve/login", nil, origin)
		if res.Code != 303 {
			t.Fatalf("start: %d %s", res.Code, res.Body.String())
		}
		location, _ := url.Parse(res.Header().Get("Location"))
		cookies := res.Result().Cookies()
		if len(cookies) != 1 || !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].SameSite != http.SameSiteLaxMode {
			t.Fatal("unsafe flow cookie")
		}
		return location.Query().Get("state"), cookies
	}
	state, cookies := start()
	path := "/api/v1/eve/callback?code=test-code&state=" + state
	if res := request("GET", path, nil, ""); !strings.Contains(res.Header().Get("Location"), "invalid_state") {
		t.Fatal("unbound callback accepted")
	}
	changed := append([]*http.Cookie{}, cookies...)
	changed = append(changed, &http.Cookie{Name: httpapi.SessionCookie, Value: "different-session"})
	if res := request("GET", path, changed, ""); !strings.Contains(res.Header().Get("Location"), "invalid_state") {
		t.Fatal("session-switched callback accepted")
	}
	result := request("GET", path, cookies, "")
	if result.Code != 303 || result.Header().Get("Location") != "/account" {
		t.Fatalf("callback: %d %s", result.Code, result.Header().Get("Location"))
	}
	var sessionCookie *http.Cookie
	for _, c := range result.Result().Cookies() {
		if c.Name == httpapi.SessionCookie {
			sessionCookie = c
		}
	}
	if sessionCookie == nil || sessionCookie.Value == "" || !sessionCookie.HttpOnly || !sessionCookie.Secure || sessionCookie.MaxAge != 604800 {
		t.Fatal("invalid session cookie")
	}
	current := request("GET", "/api/v1/identity/session", []*http.Cookie{sessionCookie}, "")
	if current.Code != 200 || !strings.Contains(current.Body.String(), `"name":"Pilot"`) || strings.Contains(current.Body.String(), sessionCookie.Value) {
		t.Fatalf("unsafe current session: %s", current.Body.String())
	}
	if replay := request("GET", path, cookies, ""); !strings.Contains(replay.Header().Get("Location"), "invalid_state") || sso.exchanges != 1 {
		t.Fatal("callback replay accepted")
	}
	state, cookies = start()
	if _, err := pool.Exec(context.Background(), "UPDATE eve_login_flows SET expires_at=now()-interval '1 second'"); err != nil {
		t.Fatal(err)
	}
	if res := request("GET", "/api/v1/eve/callback?code=expired&state="+state, cookies, ""); !strings.Contains(res.Header().Get("Location"), "invalid_state") {
		t.Fatal("expired flow accepted")
	}
	state, cookies = start()
	cancelled := request("GET", "/api/v1/eve/callback?error=access_denied&state="+state, cookies, "")
	if cancelled.Header().Get("Location") != "/login?error=cancelled" || sso.exchanges != 1 {
		t.Fatal("cancelled flow exchanged token")
	}
	state, cookies = start()
	sso.fail = true
	failed := request("GET", "/api/v1/eve/callback?code=secret&state="+state, cookies, "")
	if failed.Header().Get("Location") != "/login?error=unavailable" || strings.Contains(failed.Body.String(), "secret") {
		t.Fatal("upstream failure leaked")
	}
	sso.fail = false
	sso.required = []string{CorporationRolesScope, "esi-wallet.read_character_wallet.v1"}
	state, cookies = start()
	// Scope requirements are bound to the original flow, even if the configured
	// profile changes while the player is on the CCP consent screen.
	sso.required = []string{CorporationRolesScope}
	sso.granted = []string{CorporationRolesScope}
	missing := request("GET", "/api/v1/eve/callback?code=scope-test&state="+state, cookies, "")
	if missing.Header().Get("Location") != "/login?error=authorization_required" {
		t.Fatal("requested scope snapshot not enforced")
	}
	for _, cookie := range missing.Result().Cookies() {
		if cookie.Name == httpapi.SessionCookie && cookie.Value != "" {
			t.Fatal("partial grant created a session")
		}
	}
}

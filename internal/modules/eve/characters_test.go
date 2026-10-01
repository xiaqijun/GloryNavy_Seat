package eve

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/module"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/testutil"
)

func TestCharacterHTTPFlowsAndCleanup(t *testing.T) {
	pool := testutil.Database(t)
	ctx := context.Background()
	accounts := identity.New(pool)
	origin := "https://example.com"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	credentials, err := NewAuthorization(pool, NewClient("client", "secret", origin+"/api/v1/eve/callback"), base64.StdEncoding.EncodeToString(make([]byte, 32)), logger)
	if err != nil {
		t.Fatal(err)
	}
	sso := &fakeSSO{id: 202, required: []string{CorporationRolesScope}, granted: []string{CorporationRolesScope}}
	h := NewHandler(pool, sso, nil, origin, true, logger)
	h.Accounts = accounts
	h.Complete = func(ctx context.Context, ch Character, token string, intent identity.LoginIntent) (string, error) {
		return accounts.Complete(ctx, ch.ID, ch.Name, ch.Owner, token, intent, func(ctx context.Context, tx pgx.Tx) error { return credentials.SaveTx(ctx, tx, ch) })
	}
	auth := identity.Handler{Service: accounts, Origin: origin, Secure: true, Cleanup: RemoveCharacterTx, Check: func(_ context.Context, _ *identity.Session, p string, _ *http.Request) (bool, error) {
		return p == "eve.characters.manage", nil
	}}
	registry, err := module.New([]module.Definition{h.Module(), auth.Module()}, []string{"eve", "identity"}, auth.Authorize)
	if err != nil {
		t.Fatal(err)
	}
	router := httpapi.New(logger, registry, http.NotFoundHandler())
	token, err := accounts.SignIn(ctx, 101, "Main", "main", "")
	if err != nil {
		t.Fatal(err)
	}
	session, _ := accounts.Session(ctx, token)
	request := func(method, path, sessionToken, csrf string, flowCookie *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, nil)
		r.Header.Set("Origin", origin)
		r.Header.Set("X-CSRF-Token", csrf)
		if sessionToken != "" {
			r.AddCookie(&http.Cookie{Name: httpapi.SessionCookie, Value: sessionToken})
		}
		if flowCookie != nil {
			r.AddCookie(flowCookie)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	for _, tc := range []struct {
		token, csrf string
		status      int
	}{{"", "", 401}, {token, "", 403}, {token, "wrong", 403}} {
		if res := request("POST", "/api/v1/eve/characters/link", tc.token, tc.csrf, nil); res.Code != tc.status {
			t.Fatalf("link guard: %d", res.Code)
		}
	}
	start := func(path string) (string, *http.Cookie) {
		res := request("POST", path, token, session.CSRFToken, nil)
		if res.Code != 200 {
			t.Fatalf("start %d %s", res.Code, res.Body.String())
		}
		var payload struct {
			Data struct {
				URL string `json:"url"`
			} `json:"data"`
		}
		if json.Unmarshal(res.Body.Bytes(), &payload) != nil {
			t.Fatal("start payload")
		}
		target, err := url.Parse(payload.Data.URL)
		if err != nil {
			t.Fatal(err)
		}
		return "/api/v1/eve/callback?code=mock&state=" + target.Query().Get("state"), res.Result().Cookies()[0]
	}
	path, cookie := start("/api/v1/eve/characters/link")
	res := request("GET", path, token, "", cookie)
	if res.Header().Get("Location") != "/account" {
		t.Fatalf("link: %s", res.Header().Get("Location"))
	}
	for _, c := range res.Result().Cookies() {
		if c.Name == httpapi.SessionCookie {
			t.Fatal("link rotated authentication")
		}
	}
	chars, _ := accounts.Characters(ctx, session.UserID)
	if len(chars) != 2 {
		t.Fatalf("characters %+v", chars)
	}
	var n int
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM eve_credentials WHERE character_id=202").Scan(&n)
	if n != 1 {
		t.Fatal("credential not committed")
	}
	replay := request("GET", path, token, "", cookie)
	if !strings.Contains(replay.Header().Get("Location"), "invalid_state") {
		t.Fatal("link replay accepted")
	}
	// Reauthorization must not bind the different character selected in SSO.
	path, cookie = start("/api/v1/eve/characters/101/reauthorize")
	res = request("GET", path, token, "", cookie)
	if res.Header().Get("Location") != "/account?error=wrong_character" {
		t.Fatal("wrong character accepted")
	}
	// A separately registered user cannot be silently merged or stripped of credentials.
	_, err = accounts.SignIn(ctx, 303, "Other", "owner", "")
	if err != nil {
		t.Fatal(err)
	}
	sso.id = 303
	path, cookie = start("/api/v1/eve/characters/link")
	res = request("GET", path, token, "", cookie)
	if res.Header().Get("Location") != "/account?error=character_conflict" {
		t.Fatalf("conflict %s", res.Header().Get("Location"))
	}
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM eve_credentials WHERE character_id=303").Scan(&n)
	if n != 0 {
		t.Fatal("conflict wrote token")
	}
	// Ordinary login in a logged-in browser stays separate.
	sso.id = 404
	res = request("POST", "/api/v1/eve/login", token, "", nil)
	target, _ := url.Parse(res.Header().Get("Location"))
	cookie = res.Result().Cookies()[0]
	res = request("GET", "/api/v1/eve/callback?code=mock&state="+target.Query().Get("state"), token, "", cookie)
	if res.Header().Get("Location") != "/account" {
		t.Fatal("ordinary login failed")
	}
	var newToken string
	for _, c := range res.Result().Cookies() {
		if c.Name == httpapi.SessionCookie {
			newToken = c.Value
		}
	}
	newer, _ := accounts.Session(ctx, newToken)
	if newer == nil || newer.UserID == session.UserID {
		t.Fatal("ordinary login implicitly linked")
	}
	token, err = accounts.SignIn(ctx, 101, "Main", "main", "")
	if err != nil {
		t.Fatal(err)
	}
	session, _ = accounts.Session(ctx, token)
	// Unlink invalidates credential, snapshot, associated sessions and pending user flows.
	altToken, err := accounts.SignIn(ctx, 202, "Alt", "owner", "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, "INSERT INTO eve_role_snapshots(character_id,owner_hash,corporation_id,corporation_name,ceo_id,roles,roles_at_hq,roles_at_base,roles_at_other,synced_at,valid_until) VALUES(202,$1,10,'Corp',999,'{Director}','{}','{}','{}',now(),now()+interval '1 hour')", httpapi.Hash("owner"))
	if err != nil {
		t.Fatal(err)
	}
	path, cookie = start("/api/v1/eve/characters/202/reauthorize")
	if res = request("DELETE", "/api/v1/identity/characters/101", token, session.CSRFToken, nil); res.Code != 409 {
		t.Fatal("main unlink allowed")
	}
	if res = request("POST", "/api/v1/identity/characters/303/main", token, session.CSRFToken, nil); res.Code != 404 {
		t.Fatal("foreign main allowed")
	}
	if res = request("DELETE", "/api/v1/identity/characters/202", token, session.CSRFToken, nil); res.Code != 200 {
		t.Fatalf("unlink %d %s", res.Code, res.Body.String())
	}
	if alt, _ := accounts.Session(ctx, altToken); alt != nil {
		t.Fatal("unlinked session remains")
	}
	for _, table := range []string{"eve_credentials", "eve_role_snapshots"} {
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM "+table+" WHERE character_id=202").Scan(&n)
		if n != 0 {
			t.Fatal("unlink retained ESI state")
		}
	}
	res = request("GET", path, token, "", cookie)
	if !strings.Contains(res.Header().Get("Location"), "invalid_state") {
		t.Fatal("unlinked pending flow survived")
	}
	// Original session must still exist even when the browser presents its old cookie.
	path, cookie = start("/api/v1/eve/characters/link")
	_ = accounts.Logout(ctx, token)
	before := sso.exchanges
	res = request("GET", path, token, "", cookie)
	if res.Header().Get("Location") != "/account?error=session_expired" || sso.exchanges != before {
		t.Fatal("expired session exchanged or bound")
	}
}

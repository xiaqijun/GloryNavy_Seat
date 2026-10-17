package eve

import (
	"context"
	"encoding/json"
	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/module"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/testutil"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestMailAuthorizationIsExplicitOwnedSessionBoundAndRechecksAdmin(t *testing.T) {
	ctx := context.Background()
	pool := testutil.Database(t)
	accounts := identity.New(pool)
	var logs strings.Builder
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	token, e := accounts.SignIn(ctx, 101, "Main", "owner", "")
	if e != nil {
		t.Fatal(e)
	}
	session, _ := accounts.Session(ctx, token)
	sso := &fakeSSO{id: 101, granted: []string{MailSendScope}}
	h := NewHandler(pool, sso, nil, "https://example.com", true, logger)
	h.Accounts = accounts
	completed := 0
	h.Complete = func(context.Context, Character, string, identity.LoginIntent) (string, error) {
		completed++
		return token, nil
	}
	allowed := true
	h.MailAuthorizationAllowed = func(context.Context, string) (bool, error) { return allowed, nil }
	auth := identity.Handler{Service: accounts, Origin: "https://example.com", Secure: true, Check: func(context.Context, *identity.Session, string, *http.Request) (bool, error) { return true, nil }}
	reg, e := module.New([]module.Definition{h.Module(), auth.Module()}, []string{"eve", "identity"}, auth.Authorize)
	if e != nil {
		t.Fatal(e)
	}
	router := httpapi.New(logger, reg, http.NotFoundHandler())
	call := func(method, path string, flow *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, nil)
		r.Header.Set("Origin", "https://example.com")
		r.Header.Set("X-CSRF-Token", session.CSRFToken)
		r.AddCookie(&http.Cookie{Name: httpapi.SessionCookie, Value: token})
		if flow != nil {
			r.AddCookie(flow)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	base := "/api/v1/eve/characters/101/reauthorize"
	regular := call("POST", base, nil)
	var reply struct {
		Data struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	json.Unmarshal(regular.Body.Bytes(), &reply)
	u, _ := url.Parse(reply.Data.URL)
	if strings.Contains(u.Query().Get("scope"), MailSendScope) {
		t.Fatal("normal scope broadened")
	}
	if w := call("POST", "/api/v1/eve/characters/202/reauthorize?purpose=reminder-mail", nil); w.Code != 404 {
		t.Fatal("foreign sender", w.Code)
	}
	start := func() (string, *http.Cookie) {
		w := call("POST", base+"?purpose=reminder-mail", nil)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		json.Unmarshal(w.Body.Bytes(), &reply)
		u, _ := url.Parse(reply.Data.URL)
		if !strings.Contains(u.Query().Get("scope"), MailSendScope) {
			t.Fatal("scope missing")
		}
		return "/api/v1/eve/callback?code=test&state=" + u.Query().Get("state"), w.Result().Cookies()[0]
	}
	path, cookie := start()
	allowed = false
	if w := call("GET", path, cookie); w.Header().Get("Location") != "/reminders?error=forbidden" || completed != 0 {
		t.Fatal("revoked admin completed", w.Header(), completed)
	}
	if w := call("POST", base+"?purpose=reminder-mail", nil); w.Code != 403 {
		t.Fatal(w.Code)
	}
	allowed = true
	path, cookie = start()
	sso.granted = nil
	if w := call("GET", path, cookie); w.Header().Get("Location") != "/reminders?error=authorization_required" {
		t.Fatal(w.Header())
	}
	sso.granted = []string{MailSendScope}
	path, cookie = start()
	sso.id = 202
	if w := call("GET", path, cookie); w.Header().Get("Location") != "/reminders?error=wrong_character" {
		t.Fatal(w.Header())
	}
	sso.id = 101
	path, cookie = start()
	if w := call("GET", path, cookie); w.Header().Get("Location") != "/reminders" || completed != 1 {
		t.Fatal(w.Header(), completed)
	}
	if w := call("GET", path, cookie); !strings.Contains(w.Header().Get("Location"), "invalid_state") || completed != 1 {
		t.Fatal("replay")
	}
	for _, reason := range []string{"forbidden", "authorization_required", "wrong_character"} {
		if !strings.Contains(logs.String(), "reason="+reason) {
			t.Fatal("missing rejection reason", reason)
		}
	}
	if !strings.Contains(logs.String(), "expected_character_id=101 authorized_character_id=202") {
		t.Fatal("missing character mismatch diagnostic")
	}
	for _, secret := range []string{token, session.CSRFToken, sso.verifier, "mock-access", "mock-refresh", "code=test"} {
		if secret != "" && strings.Contains(logs.String(), secret) {
			t.Fatal("authorization secret logged")
		}
	}
}

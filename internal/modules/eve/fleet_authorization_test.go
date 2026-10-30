package eve

import (
	"context"
	"encoding/json"
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

func TestFleetAuthorizationRequiresOwnedCharacterAndExplicitScope(t *testing.T) {
	ctx := context.Background()
	pool := testutil.Database(t)
	accounts := identity.New(pool)
	token, err := accounts.SignIn(ctx, 101, "Main", "owner", "")
	if err != nil {
		t.Fatal(err)
	}
	session, _ := accounts.Session(ctx, token)
	sso := &fakeSSO{id: 101, granted: []string{FleetWriteScope}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewHandler(pool, sso, nil, "https://example.com", true, logger)
	h.Accounts = accounts
	completed := 0
	h.Complete = func(context.Context, Character, string, identity.LoginIntent) (string, error) {
		completed++
		return token, nil
	}
	auth := identity.Handler{Service: accounts, Origin: "https://example.com", Secure: true, Check: func(context.Context, *identity.Session, string, *http.Request) (bool, error) { return true, nil }}
	reg, err := module.New([]module.Definition{h.Module(), auth.Module()}, []string{"eve", "identity"}, auth.Authorize)
	if err != nil {
		t.Fatal(err)
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
	if w := call("POST", "/api/v1/eve/characters/202/reauthorize?purpose=fleet-write", nil); w.Code != 404 {
		t.Fatal("foreign", w.Code)
	}
	w := call("POST", "/api/v1/eve/characters/101/reauthorize?purpose=fleet-write", nil)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var response struct {
		Data struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(response.Data.URL)
	if err != nil || !strings.Contains(u.Query().Get("scope"), FleetWriteScope) {
		t.Fatal(u, err)
	}
	callback := "/api/v1/eve/callback?code=test&state=" + u.Query().Get("state")
	result := call("GET", callback, w.Result().Cookies()[0])
	if result.Header().Get("Location") != "/attendance" || completed != 1 {
		t.Fatal(result.Header(), completed)
	}
}

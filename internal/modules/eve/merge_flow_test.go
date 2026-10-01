package eve

import (
	"context"
	"encoding/json"
	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/module"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/testutil"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestMergeSSOFlowOnlyCreatesVerifiedPreview(t *testing.T) {
	ctx := context.Background()
	pool := testutil.Database(t)
	accounts := identity.New(pool)
	token, err := accounts.SignIn(ctx, 101, "Target", "target", "")
	if err != nil {
		t.Fatal(err)
	}
	session, _ := accounts.Session(ctx, token)
	if _, err = accounts.SignIn(ctx, 202, "Source", "owner", ""); err != nil {
		t.Fatal(err)
	}
	sso := &fakeSSO{id: 202}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewHandler(pool, sso, nil, "https://example.com", true, logger)
	h.Accounts = accounts
	h.Complete = func(ctx context.Context, c Character, previous string, intent identity.LoginIntent) (string, error) {
		if intent.Kind != "merge" {
			t.Fatal("wrong intent")
		}
		return accounts.ProveMerge(ctx, c.ID, c.Owner, previous, intent.UserID, nil)
	}
	auth := identity.Handler{Service: accounts, Origin: "https://example.com", Secure: true, Check: func(context.Context, *identity.Session, string, *http.Request) (bool, error) { return true, nil }}
	registry, err := module.New([]module.Definition{h.Module(), auth.Module()}, []string{"eve", "identity"}, auth.Authorize)
	if err != nil {
		t.Fatal(err)
	}
	router := httpapi.New(logger, registry, http.NotFoundHandler())
	start := func(csrf string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "https://example.com/api/v1/eve/accounts/merge", nil)
		req.Header.Set("Origin", "https://example.com")
		req.Header.Set("X-CSRF-Token", csrf)
		req.AddCookie(&http.Cookie{Name: httpapi.SessionCookie, Value: token})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	if start("wrong").Code != 403 {
		t.Fatal("missing CSRF allowed")
	}
	w := start(session.CSRFToken)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var data struct {
		Data struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &data)
	target, _ := url.Parse(data.Data.URL)
	callback := func(sessionToken string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "https://example.com/api/v1/eve/callback?state="+target.Query().Get("state")+"&code=verified", nil)
		req.AddCookie(&http.Cookie{Name: httpapi.SessionCookie, Value: sessionToken})
		for _, c := range w.Result().Cookies() {
			req.AddCookie(c)
		}
		out := httptest.NewRecorder()
		router.ServeHTTP(out, req)
		return out
	}
	out := callback(token)
	if out.Code != 303 || !strings.HasPrefix(out.Header().Get("Location"), "/account?merge=") {
		t.Fatal(out.Code, out.Header().Get("Location"))
	}
	owner, _ := accounts.UserForCharacter(ctx, 202)
	if owner == session.UserID {
		t.Fatal("callback moved account before confirmation")
	}
	proofID := strings.TrimPrefix(out.Header().Get("Location"), "/account?merge=")
	for _, test := range []struct {
		method, cookie, csrf string
		status               int
	}{{"GET", "", "", 401}, {"POST", token, "wrong", 403}, {"DELETE", token, "wrong", 403}} {
		req := httptest.NewRequest(test.method, "https://example.com/api/v1/identity/merges/"+proofID, strings.NewReader(`{"token":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`))
		req.Header.Set("Origin", "https://example.com")
		req.Header.Set("X-CSRF-Token", test.csrf)
		req.AddCookie(&http.Cookie{Name: httpapi.SessionCookie, Value: test.cookie})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != test.status {
			t.Fatalf("%s: %d", test.method, rec.Code)
		}
	}
	for _, c := range out.Result().Cookies() {
		if c.Name == httpapi.SessionCookie {
			t.Fatal("merge changed authenticating session")
		}
	}
	replay := callback(token)
	if replay.Header().Get("Location") != "/login?error=invalid_state" {
		t.Fatal("flow replay accepted")
	}
}

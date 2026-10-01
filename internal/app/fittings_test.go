package app

import (
	"context"
	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/testutil"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFittingsHostAuthorizationAndCSRF(t *testing.T) {
	pool := testutil.Database(t)
	ctx := context.Background()
	accounts := identity.New(pool)
	token, err := accounts.SignIn(ctx, 101, "Pilot", "owner", "")
	if err != nil {
		t.Fatal(err)
	}
	session, err := accounts.Session(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := New(pool, slog.New(slog.NewTextHandler(io.Discard, nil)), "test", []string{"system", "identity", "eve", "access", "fittings"}, AuthConfig{Origin: "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	call := func(method, path, body, csrf string, login bool) int {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Origin", "https://example.com")
		req.Header.Set("X-CSRF-Token", csrf)
		if login {
			req.AddCookie(&http.Cookie{Name: httpapi.SessionCookie, Value: token})
		}
		r := httptest.NewRecorder()
		handler.ServeHTTP(r, req)
		return r.Code
	}
	for _, path := range []string{"/api/v1/fittings/context", "/api/v1/fittings/drafts", "/api/v1/fittings/saved?character_id=101", "/api/v1/fittings/names?ids=587", "/api/v1/fittings/library/context", "/api/v1/fittings/library?corporation_id=900", "/api/v1/fittings/library/1/requirements"} {
		if code := call("GET", path, "", "", false); code != 401 {
			t.Fatal(path, code)
		}
	}
	if code := call("GET", "/api/v1/fittings/context", "", "", true); code != 200 {
		t.Fatal("self", code)
	}
	if code := call("GET", "/api/v1/fittings/saved?character_id=999", "", "", true); code != 404 {
		t.Fatal("foreign character", code)
	}
	if code := call("GET", "/api/v1/fittings/context?member=22222222-2222-4222-8222-222222222222", "", "", true); code != 404 {
		t.Fatal("foreign account", code)
	}
	body := `{"request_key":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","fit":{"name":"Rifter","ship_type_id":"587","skill_mode":"all5","items":[]}}`
	if code := call("POST", "/api/v1/fittings/drafts", body, "", true); code != 403 {
		t.Fatal("missing csrf", code)
	}
	for _, path := range []string{"/api/v1/fittings/library", "/api/v1/fittings/library/1/save-to-game"} {
		if code := call("POST", path, `{}`, "", true); code != 403 {
			t.Fatal("library CSRF", path, code)
		}
	}
	if code := call("POST", "/api/v1/fittings/drafts", body, session.CSRFToken, true); code != 200 {
		t.Fatal("own draft", code)
	}
}

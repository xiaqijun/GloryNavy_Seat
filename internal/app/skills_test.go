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

func TestSkillsHostAuthorizationAndCSRF(t *testing.T) {
	pool := testutil.Database(t)
	accounts := identity.New(pool)
	ctx := context.Background()
	token, e := accounts.SignIn(ctx, 101, "Pilot", "owner", "")
	if e != nil {
		t.Fatal(e)
	}
	session, e := accounts.Session(ctx, token)
	if e != nil {
		t.Fatal(e)
	}
	h, e := New(pool, slog.New(slog.NewTextHandler(io.Discard, nil)), "test", []string{"system", "identity", "eve", "access", "skills"}, AuthConfig{Origin: "https://example.com"})
	if e != nil {
		t.Fatal(e)
	}
	call := func(method, path, csrf string, login bool) int {
		req := httptest.NewRequest(method, path, strings.NewReader(`{"corporation_id":"900","name":"Test","requirements":[{"skill_id":"3300","level":5}],"request_key":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"}`))
		req.Header.Set("Origin", "https://example.com")
		req.Header.Set("X-CSRF-Token", csrf)
		if login {
			req.AddCookie(&http.Cookie{Name: httpapi.SessionCookie, Value: token})
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w.Code
	}
	for _, p := range []string{"/api/v1/skills/context", "/api/v1/skills/catalog", "/api/v1/skills/characters/101", "/api/v1/skills/plans?corporation_id=900"} {
		if code := call("GET", p, "", false); code != 401 {
			t.Fatal(p, code)
		}
	}
	if code := call("GET", "/api/v1/skills/characters/101", "", true); code != 200 {
		t.Fatal("self", code)
	}
	if code := call("GET", "/api/v1/skills/characters/999", "", true); code != 404 {
		t.Fatal("foreign", code)
	}
	if code := call("POST", "/api/v1/skills/plans", "", true); code != 403 {
		t.Fatal("csrf", code)
	}
	if code := call("POST", "/api/v1/skills/plans", session.CSRFToken, true); code != 404 {
		t.Fatal("self permission must not grant management", code)
	}
	if _, e = accounts.SignIn(ctx, 102, "Other", "other-owner", ""); e != nil {
		t.Fatal(e)
	}
	if code := call("GET", "/api/v1/skills/characters/102", "", true); code != 404 {
		t.Fatal("other active owner", code)
	}
	if _, e = pool.Exec(ctx, "INSERT INTO access_administrators(user_id) VALUES($1)", session.UserID); e != nil {
		t.Fatal(e)
	}
	if code := call("GET", "/api/v1/skills/characters/102", "", true); code != 200 {
		t.Fatal("current administrator", code)
	}
	if _, e = pool.Exec(ctx, "DELETE FROM access_administrators WHERE user_id=$1", session.UserID); e != nil {
		t.Fatal(e)
	}
	if code := call("GET", "/api/v1/skills/characters/102", "", true); code != 404 {
		t.Fatal("revoked administrator", code)
	}
}

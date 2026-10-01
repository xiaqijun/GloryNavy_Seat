package app

import (
	"context"
	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/modules/access"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/testutil"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetadataVisibility(t *testing.T) {
	pool := testutil.Database(t)
	ctx := context.Background()
	users := identity.New(pool)
	token, err := users.SignIn(ctx, 1001, "Pilot", "owner", "")
	if err != nil {
		t.Fatal(err)
	}
	session, err := users.Session(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	app, err := New(pool, slog.New(slog.NewTextHandler(io.Discard, nil)), "private-release", []string{"system", "identity", "eve", "access"}, AuthConfig{Origin: "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	call := func(path string, login bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		if login {
			r.AddCookie(&http.Cookie{Name: httpapi.SessionCookie, Value: token})
		}
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		return w
	}
	for _, path := range []string{"/api/v1/modules", "/api/v1/system/status", "/api/v1/eve/status"} {
		if w := call(path, false); w.Code != 401 || strings.Contains(w.Body.String(), "scopes") || strings.Contains(w.Body.String(), "private-release") {
			t.Fatal(path, w.Code, w.Body.String())
		}
	}
	for _, path := range []string{"/health/ready", "/api/v1/eve/login-status"} {
		w := call(path, false)
		if w.Code != 200 || strings.Contains(w.Body.String(), "version") || strings.Contains(w.Body.String(), "scopes") {
			t.Fatal(path, w.Code, w.Body.String())
		}
	}
	for _, path := range []string{"/api/v1/modules", "/api/v1/eve/status"} {
		if w := call(path, true); w.Code != 200 {
			t.Fatal(path, w.Code, w.Body.String())
		}
	}
	if w := call("/api/v1/system/status", true); w.Code != 403 {
		t.Fatal(w.Code)
	}
	permissions := access.New(pool, nil, nil)
	if err = permissions.SetAdministrator(ctx, session.UserID, true); err != nil {
		t.Fatal(err)
	}
	if w := call("/api/v1/system/status", true); w.Code != 200 || !strings.Contains(w.Body.String(), "private-release") {
		t.Fatal(w.Code, w.Body.String())
	}
}

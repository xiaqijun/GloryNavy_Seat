package app

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/modules/access"
	"glorynavy.local/seat/internal/modules/community"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/testutil"
)

func TestOneBotManagementRequiresCurrentAdministratorAndCSRF(t *testing.T) {
	pool := testutil.Database(t)
	ctx := context.Background()
	users := identity.New(pool)
	token, err := users.SignIn(ctx, 101, "Admin", "owner", "")
	if err != nil {
		t.Fatal(err)
	}
	session, err := users.Session(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = community.New(pool).Update(ctx, session.UserID, "123456789", "Admin", "0"); err != nil {
		t.Fatal(err)
	}
	a, err := New(pool, slog.New(slog.NewTextHandler(io.Discard, nil)), "test", []string{"system", "identity", "eve", "access", "community"}, AuthConfig{Origin: "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	call := func(method, path, cookie, csrf string) int {
		r := httptest.NewRequest(method, path, strings.NewReader(`{}`))
		r.Header.Set("Origin", "https://example.com")
		r.Header.Set("X-CSRF-Token", csrf)
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: httpapi.SessionCookie, Value: cookie})
		}
		w := httptest.NewRecorder()
		a.ServeHTTP(w, r)
		return w.Code
	}
	routes := []struct{ method, path string }{{"GET", "settings"}, {"PUT", "settings"}, {"GET", "status"}, {"POST", "test"}}
	for _, route := range routes {
		path := "/api/v1/community/qq/onebot/" + route.path
		if status := call(route.method, path, "", ""); status != 401 {
			t.Fatalf("anonymous %s %d", path, status)
		}
		if status := call(route.method, path, token, session.CSRFToken); status != 403 {
			t.Fatalf("member %s %d", path, status)
		}
	}
	permissions := access.New(pool, nil, nil)
	if err = permissions.SetAdministrator(ctx, session.UserID, true); err != nil {
		t.Fatal(err)
	}
	if status := call("GET", "/api/v1/community/qq/onebot/settings", token, ""); status != 200 {
		t.Fatal("admin settings", status)
	}
	for _, path := range []string{"settings", "test"} {
		method := "PUT"
		if path == "test" {
			method = "POST"
		}
		if status := call(method, "/api/v1/community/qq/onebot/"+path, token, ""); status != 403 {
			t.Fatal("csrf", status)
		}
	}
	if err = permissions.SetAdministrator(ctx, session.UserID, false); err != nil {
		t.Fatal(err)
	}
	if status := call("GET", "/api/v1/community/qq/onebot/settings", token, ""); status != 403 {
		t.Fatal("revoked admin retained access", status)
	}
}

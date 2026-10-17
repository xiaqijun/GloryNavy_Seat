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

func TestReminderManagementRequiresCurrentAdministratorAndCSRF(t *testing.T) {
	pool := testutil.Database(t)
	ctx := context.Background()
	users := identity.New(pool)
	token, e := users.SignIn(ctx, 101, "Admin", "owner", "")
	if e != nil {
		t.Fatal(e)
	}
	session, _ := users.Session(ctx, token)
	a, e := New(pool, slog.New(slog.NewTextHandler(io.Discard, nil)), "test", []string{"system", "identity", "eve", "access", "approval", "reminder"}, AuthConfig{Origin: "https://example.com"})
	if e != nil {
		t.Fatal(e)
	}
	call := func(method, path, cookie, csrf string) int {
		r := httptest.NewRequest(method, "/api/v1/reminder/"+path, strings.NewReader(`{"enabled":false,"sender_id":"0","revision":"1"}`))
		r.Header.Set("Origin", "https://example.com")
		r.Header.Set("X-CSRF-Token", csrf)
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: httpapi.SessionCookie, Value: cookie})
		}
		w := httptest.NewRecorder()
		a.ServeHTTP(w, r)
		return w.Code
	}
	for _, path := range []string{"settings", "deliveries"} {
		if c := call("GET", path, "", ""); c != 401 {
			t.Fatal(c)
		}
		if c := call("GET", path, token, ""); c != 403 {
			t.Fatal(c)
		}
	}
	if e := access.New(pool, nil, nil).SetAdministrator(ctx, session.UserID, true); e != nil {
		t.Fatal(e)
	}
	if c := call("GET", "settings", token, ""); c != 200 {
		t.Fatal(c)
	}
	if c := call("PUT", "settings", token, ""); c != 403 {
		t.Fatal("CSRF", c)
	}
	if c := call("PUT", "settings", token, session.CSRFToken); c != 200 {
		t.Fatal("save", c)
	}
	if c := call("PUT", "settings", token, session.CSRFToken); c != 409 {
		t.Fatal("revision", c)
	}
	if c := call("GET", "deliveries?before=bad", token, ""); c != 400 {
		t.Fatal("cursor", c)
	}
	if _, e := pool.Exec(ctx, `DELETE FROM access_administrators`); e != nil {
		t.Fatal(e)
	}
	if c := call("GET", "settings", token, ""); c != 403 {
		t.Fatal("stale admin", c)
	}
}

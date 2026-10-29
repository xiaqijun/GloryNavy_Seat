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
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/testutil"
)

func TestQQStructureSettingsHTTPAuthorizationAndCSRF(t *testing.T) {
	pool := testutil.Database(t)
	ctx := context.Background()
	users := identity.New(pool)
	adminToken, err := users.SignIn(ctx, 201, "Admin", "owner-admin", "")
	if err != nil {
		t.Fatal(err)
	}
	adminSession, err := users.Session(ctx, adminToken)
	if err != nil {
		t.Fatal(err)
	}
	memberToken, err := users.SignIn(ctx, 202, "Member", "owner-member", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = access.New(pool, nil, nil).SetAdministrator(ctx, adminSession.UserID, true); err != nil {
		t.Fatal(err)
	}
	a, err := New(pool, slog.New(slog.NewTextHandler(io.Discard, nil)), "test", []string{"system", "identity", "eve", "access", "community", "structures"}, AuthConfig{Origin: "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path, token, csrf, body string) int {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Origin", "https://example.com")
		if token != "" {
			r.AddCookie(&http.Cookie{Name: httpapi.SessionCookie, Value: token})
		}
		if csrf != "" {
			r.Header.Set("X-CSRF-Token", csrf)
		}
		w := httptest.NewRecorder()
		a.ServeHTTP(w, r)
		return w.Code
	}
	path := "/api/v1/community/qq/structure/settings"
	if got := request("GET", path, "", "", ""); got != 401 {
		t.Fatalf("anonymous: %d", got)
	}
	if got := request("GET", path, memberToken, "", ""); got != 403 {
		t.Fatalf("member: %d", got)
	}
	if got := request("GET", path, adminToken, "", ""); got != 200 {
		t.Fatalf("admin: %d", got)
	}
	body := `{"enabled":false,"group_openid":"","corporation_id":"0","revision":"1"}`
	if got := request("PUT", path, adminToken, "", body); got != 403 {
		t.Fatalf("missing CSRF: %d", got)
	}
	if got := request("PUT", path, adminToken, adminSession.CSRFToken, body); got != 200 {
		t.Fatalf("admin save: %d", got)
	}
}

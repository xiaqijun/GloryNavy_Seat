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
	"testing"
)

func TestSyncManagementPermissionAndCSRF(t *testing.T) {
	pool := testutil.Database(t)
	ctx := context.Background()
	users := identity.New(pool)
	token, err := users.SignIn(ctx, 101, "Pilot", "owner", "")
	if err != nil {
		t.Fatal(err)
	}
	session, err := users.Session(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	permissions := access.New(pool, nil, nil)
	role := access.Role{ID: "01994763-4111-7000-8000-111111111111", Name: "Manager", Grants: []access.Grant{{Permission: "access.manage"}}}
	role, err = permissions.SaveRole(ctx, "local-operator", role)
	if err != nil {
		t.Fatal(err)
	}
	if err = permissions.Assign(ctx, "local-operator", session.UserID, role.ID, true); err != nil {
		t.Fatal(err)
	}
	application, err := New(pool, slog.New(slog.NewTextHandler(io.Discard, nil)), "test", []string{"system", "identity", "eve", "access"}, AuthConfig{Origin: "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	call := func(method, path string) int {
		req := httptest.NewRequest(method, path, nil)
		req.AddCookie(&http.Cookie{Name: httpapi.SessionCookie, Value: token})
		req.Header.Set("Origin", "https://example.com")
		res := httptest.NewRecorder()
		application.ServeHTTP(res, req)
		return res.Code
	}
	if got := call("GET", "/api/v1/eve/sync/targets"); got != 403 {
		t.Fatalf("access.manage crossed sync permission: %d", got)
	}
	for _, path := range []string{"/api/v1/eve/sync/rate-limits", "/api/v1/eve/sync/rate-limits/1/routes", "/api/v1/eve/sync/tokens", "/api/v1/eve/sync/tokens/101/events"} {
		if got := call("GET", path); got != 403 {
			t.Fatalf("token metadata crossed permission: %d", got)
		}
		res := httptest.NewRecorder()
		application.ServeHTTP(res, httptest.NewRequest("GET", path, nil))
		if res.Code != 401 {
			t.Fatalf("anonymous token metadata exposed: %d", res.Code)
		}
	}
	role.Grants = []access.Grant{{Permission: "eve.sync.manage"}}
	if _, err = permissions.SaveRole(ctx, "local-operator", role); err != nil {
		t.Fatal(err)
	}
	if got := call("GET", "/api/v1/eve/sync/targets"); got != 200 {
		t.Fatalf("sync manager denied: %d", got)
	}
	if got := call("GET", "/api/v1/eve/sync/rate-limits"); got != 200 {
		t.Fatalf("bucket monitor denied: %d", got)
	}
	if got := call("GET", "/api/v1/eve/sync/tokens"); got != 200 {
		t.Fatalf("token monitor denied: %d", got)
	}
	if got := call("POST", "/api/v1/eve/sync/targets/1/retry"); got != 403 {
		t.Fatalf("missing CSRF accepted: %d", got)
	}
	if got := call("GET", "/api/v1/eve/sync/characters/999"); got != 404 {
		t.Fatalf("foreign character disclosed: %d", got)
	}
	if err = permissions.Assign(ctx, "local-operator", session.UserID, role.ID, false); err != nil {
		t.Fatal(err)
	}
	if got := call("GET", "/api/v1/eve/sync/rate-limits"); got != 403 {
		t.Fatalf("revoked bucket monitor accessible: %d", got)
	}
	if got := call("GET", "/api/v1/eve/sync/tokens"); got != 403 {
		t.Fatalf("revoked monitor still accessible: %d", got)
	}
}

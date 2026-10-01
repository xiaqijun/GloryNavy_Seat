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

func TestCommunityProfileHTTPAndBusinessGate(t *testing.T) {
	pool := testutil.Database(t)
	ctx := context.Background()
	users := identity.New(pool)
	token, err := users.SignIn(ctx, 101, "Pilot", "owner", "")
	if err != nil {
		t.Fatal(err)
	}
	session, _ := users.Session(ctx, token)
	_, err = users.SignIn(ctx, 202, "Other", "other", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = access.New(pool, nil, nil).SetAdministrator(ctx, session.UserID, true); err != nil {
		t.Fatal(err)
	}
	app, err := New(pool, slog.New(slog.NewTextHandler(io.Discard, nil)), "test", []string{"system", "identity", "eve", "access", "community"}, AuthConfig{Origin: "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	call := func(method, path, body, cookie, csrf, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Origin", origin)
		r.Header.Set("X-CSRF-Token", csrf)
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: httpapi.SessionCookie, Value: cookie})
		}
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		return w
	}
	get := func(path string) *httptest.ResponseRecorder { return call("GET", path, "", token, "", "") }
	if w := get("/api/v1/access/roles"); w.Code != 403 || !strings.Contains(w.Body.String(), "profile_required") {
		t.Fatal("incomplete admin bypassed profile gate")
	}
	for _, path := range []string{"/api/v1/identity/characters", "/api/v1/access/me", "/api/v1/community/profile"} {
		if w := get(path); w.Code != 200 {
			t.Fatalf("recovery entry gated %s %d", path, w.Code)
		}
	}
	body := `{"qq_number":"123456","kook_name":"舰长","version":"0"}`
	for _, tc := range []struct {
		cookie, csrf, origin string
		status               int
	}{{"", "", "", 401}, {token, "", "https://example.com", 403}, {token, session.CSRFToken, "https://evil.example", 403}} {
		if w := call("PUT", "/api/v1/community/profile", body, tc.cookie, tc.csrf, tc.origin); w.Code != tc.status {
			t.Fatalf("guard %d", w.Code)
		}
	}
	for _, bad := range []string{`{"qq_number":"123456","kook_name":"舰长","version":"0","user_id":"another"}`, `{"qq_number":"123456","kook_name":"舰长","version":"0","qq_confirmed":true}`, body + body, `{"qq_number":"123456","kook_name":"","version":"0"}`} {
		if w := call("PUT", "/api/v1/community/profile", bad, token, session.CSRFToken, "https://example.com"); w.Code != 400 {
			t.Fatalf("invalid profile accepted %d", w.Code)
		}
	}
	w := call("PUT", "/api/v1/community/profile", body, token, session.CSRFToken, "https://example.com")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"complete":true`) || strings.Contains(w.Body.String(), `"confirmation":"confirmed"`) {
		t.Fatalf("save %d %s", w.Code, w.Body.String())
	}
	if w = get("/api/v1/access/roles"); w.Code != 200 {
		t.Fatal("pending community confirmation incorrectly gates complete profile")
	}
	if w = call("PUT", "/api/v1/community/profile", body, token, session.CSRFToken, "https://example.com"); w.Code != 409 {
		t.Fatal("stale profile accepted")
	}
	var count int
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM community_profiles").Scan(&count)
	if count != 1 {
		t.Fatal("another user's profile created")
	}
	if w = call("GET", "/api/v1/community/profile", "", "", "", ""); w.Code != 401 {
		t.Fatal("anonymous profile read")
	}
}

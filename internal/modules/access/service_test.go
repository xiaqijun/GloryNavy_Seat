package access

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/module"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/testutil"
)

func TestRBACPersistenceAndHTTPBoundaries(t *testing.T) {
	pool := testutil.Database(t)
	ctx := context.Background()
	users := identity.New(pool)
	token, err := users.SignIn(ctx, 123, "Pilot", "owner", "")
	if err != nil {
		t.Fatal(err)
	}
	session, err := users.Session(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	s := New(pool, nil, func(context.Context, int64) (Corporation, error) { return Corporation{ID: 10, AllianceID: 100}, nil })
	auth := identity.Handler{Service: users, Origin: "https://example.com", Check: func(ctx context.Context, p *identity.Session, permission string, _ *http.Request) (bool, error) {
		return s.Can(ctx, p.UserID, permission, Corporation{})
	}}
	h := Handler{Service: s, User: func(r *http.Request) string { return identity.Principal(r.Context()).UserID }}
	h.Directory = func(ctx context.Context, search, after string) ([]Member, string, error) {
		rows, next, err := users.SearchMembers(ctx, search, after)
		items := make([]Member, 0, len(rows))
		for _, row := range rows {
			items = append(items, Member{UserID: row.UserID, CharacterID: row.Main.ID, Name: row.Main.Name, CharacterCount: row.CharacterCount})
		}
		return items, next, err
	}
	reg, err := module.New([]module.Definition{auth.Module(), {Manifest: module.Manifest{ID: "eve", Version: "0.1.0", APIVersion: 1}}, h.Module()}, []string{"identity", "eve", "access"}, auth.Authorize)
	if err != nil {
		t.Fatal(err)
	}
	mux := httpapi.New(slog.New(slog.NewTextHandler(io.Discard, nil)), reg, http.NotFoundHandler())
	call := func(method, path, body, cookie, csrf string) int {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Origin", auth.Origin)
		r.Header.Set("X-CSRF-Token", csrf)
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: httpapi.SessionCookie, Value: cookie})
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w.Code
	}
	if code := call("GET", "/api/v1/access/me", "", "", ""); code != 401 {
		t.Fatalf("anonymous %d", code)
	}
	if code := call("GET", "/api/v1/access/roles", "", token, ""); code != 403 {
		t.Fatalf("member management %d", code)
	}
	for _, path := range []string{"/api/v1/access/members", "/api/v1/access/audit", "/api/v1/access/catalog"} {
		if code := call("GET", path, "", token, ""); code != 403 {
			t.Fatalf("member read %s: %d", path, code)
		}
		if code := call("GET", path, "", "", ""); code != 401 {
			t.Fatalf("anonymous read %s: %d", path, code)
		}
	}
	if code := call("GET", "/api/v1/access/corporations/10/summary", "", token, ""); code != 403 {
		t.Fatalf("unscoped summary %d", code)
	}
	if err = s.SetAdministrator(ctx, session.UserID, true); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/api/v1/access/catalog", nil)
	r.AddCookie(&http.Cookie{Name: httpapi.SessionCookie, Value: token})
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "access.manage") || !strings.Contains(w.Body.String(), "corporation.contract") || !strings.Contains(w.Body.String(), "corporation.journal") || !strings.Contains(w.Body.String(), "corporation.wallet_") || strings.Contains(w.Body.String(), "corporation.asset_first_division") {
		t.Fatalf("management catalog must contain only shipped features: %s", w.Body.String())
	}
	role := "01994763-4111-7000-8000-111111111111"
	body := `{"name":"财务","version":"0","grants":[{"permission":"corporation.journal","corporations":["10"],"alliances":[]}]}`
	if code := call("PUT", "/api/v1/access/roles/"+role, body, token, ""); code != 403 {
		t.Fatalf("missing CSRF %d", code)
	}
	if code := call("PUT", "/api/v1/access/roles/"+role, body, token, session.CSRFToken); code != 200 {
		t.Fatalf("role creation %d", code)
	}
	if code := call("PUT", "/api/v1/access/roles/"+role, body, token, session.CSRFToken); code != 409 {
		t.Fatalf("stale HTTP creation %d", code)
	}
	for _, path := range []string{"/api/v1/access/members?q=Pilot", "/api/v1/access/audit"} {
		if code := call("GET", path, "", token, ""); code != 200 {
			t.Fatalf("management read %s %d", path, code)
		}
	}
	for _, path := range []string{"/api/v1/access/members?after=bad", "/api/v1/access/audit?before=-1"} {
		if code := call("GET", path, "", token, ""); code != 400 {
			t.Fatalf("invalid cursor %s %d", path, code)
		}
	}
	if code := call("PUT", "/api/v1/access/users/"+session.UserID+"/roles/"+role, "", token, session.CSRFToken); code != 200 {
		t.Fatalf("assignment %d", code)
	}
	if err = s.SetAdministrator(ctx, session.UserID, false); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		id   int64
		want bool
	}{{10, true}, {20, false}} {
		got, err := s.Can(ctx, session.UserID, "corporation.journal", Corporation{ID: tc.id})
		if err != nil || got != tc.want {
			t.Fatalf("scope %d: %v %v", tc.id, got, err)
		}
	}
	if err = s.Assign(ctx, "test", session.UserID, role, false); err != nil {
		t.Fatal(err)
	}
	got, err := s.Can(ctx, session.UserID, "corporation.journal", Corporation{ID: 10})
	if err != nil || got {
		t.Fatal("removed assignment retained access")
	}
	var count int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM access_audit").Scan(&count); err != nil || count != 5 {
		t.Fatalf("audit count %d %v", count, err)
	}
}

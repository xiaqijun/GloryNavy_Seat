package app

import (
	"context"
	"encoding/base64"
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

func TestReminderEmailHTTPPermissions(t *testing.T) {
	pool := testutil.Database(t)
	ctx := context.Background()
	users := identity.New(pool)
	adminToken, err := users.SignIn(ctx, 101, "Admin", "admin-owner", "")
	if err != nil {
		t.Fatal(err)
	}
	admin, _ := users.Session(ctx, adminToken)
	memberToken, err := users.SignIn(ctx, 202, "Member", "member-owner", "")
	if err != nil {
		t.Fatal(err)
	}
	member, _ := users.Session(ctx, memberToken)
	if err = access.New(pool, nil, nil).SetAdministrator(ctx, admin.UserID, true); err != nil {
		t.Fatal(err)
	}
	app, err := New(pool, slog.New(slog.NewTextHandler(io.Discard, nil)), "test", []string{"system", "identity", "eve", "access", "approval", "reminder"}, AuthConfig{Origin: "https://example.com", TokenKey: base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))})
	if err != nil {
		t.Fatal(err)
	}
	call := func(method, path, token, csrf, body string) int {
		r := httptest.NewRequest(method, "/api/v1/reminder/"+path, strings.NewReader(body))
		r.Header.Set("Origin", "https://example.com")
		r.Header.Set("X-CSRF-Token", csrf)
		if token != "" {
			r.AddCookie(&http.Cookie{Name: httpapi.SessionCookie, Value: token})
		}
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		return w.Code
	}
	if got := call("GET", "email/settings", "", "", ""); got != 401 {
		t.Fatalf("anonymous %d", got)
	}
	if got := call("GET", "email/settings", memberToken, "", ""); got != 403 {
		t.Fatalf("member admin settings %d", got)
	}
	if got := call("GET", "email/settings", adminToken, "", ""); got != 200 {
		t.Fatalf("admin settings %d", got)
	}
	if got := call("GET", "email/account", memberToken, "", ""); got != 200 {
		t.Fatalf("own account %d", got)
	}
	if got := call("POST", "email/challenge", memberToken, "", `{"address":"person@example.com"}`); got != 403 {
		t.Fatalf("missing csrf %d", got)
	}
	if got := call("POST", "email/challenge", memberToken, member.CSRFToken, `{"address":"person@example.com"}`); got != 409 {
		t.Fatalf("disabled service %d", got)
	}
	if _, err = pool.Exec(ctx, `DELETE FROM access_administrators WHERE user_id=$1`, admin.UserID); err != nil {
		t.Fatal(err)
	}
	if got := call("GET", "email/settings", adminToken, "", ""); got != 403 {
		t.Fatalf("revoked admin %d", got)
	}
}

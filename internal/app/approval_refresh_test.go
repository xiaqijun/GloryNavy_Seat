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

func TestApprovalRefreshRequiresSessionCSRFAndCurrentAccess(t *testing.T) {
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
	a, err := New(pool, slog.New(slog.NewTextHandler(io.Discard, nil)), "test", []string{"system", "identity", "eve", "access", "attendance", "exchange", "approval"}, AuthConfig{Origin: "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	call := func(login bool, csrf string) int {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/approval/sources/exchange/refresh", strings.NewReader(""))
		r.Header.Set("Origin", "https://example.com")
		r.Header.Set("X-CSRF-Token", csrf)
		if login {
			r.AddCookie(&http.Cookie{Name: httpapi.SessionCookie, Value: token})
		}
		w := httptest.NewRecorder()
		a.ServeHTTP(w, r)
		return w.Code
	}
	if got := call(false, ""); got != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", got)
	}
	if got := call(true, ""); got != http.StatusForbidden {
		t.Fatalf("csrf: %d", got)
	}
	if got := call(true, session.CSRFToken); got != http.StatusNotFound {
		t.Fatalf("ordinary member: %d", got)
	}
	if err = access.New(pool, nil, nil).SetAdministrator(ctx, session.UserID, true); err != nil {
		t.Fatal(err)
	}
	if got := call(true, session.CSRFToken); got != http.StatusOK {
		t.Fatalf("administrator: %d", got)
	}
	if err = access.New(pool, nil, nil).SetAdministrator(ctx, session.UserID, false); err != nil {
		t.Fatal(err)
	}
	if got := call(true, session.CSRFToken); got != http.StatusNotFound {
		t.Fatalf("revoked: %d", got)
	}
}

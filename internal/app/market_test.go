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

func TestMarketHostSessionCSRFAndAdmin(t *testing.T) {
	p := testutil.Database(t)
	ctx := context.Background()
	accounts := identity.New(p)
	token, err := accounts.SignIn(ctx, 101, "Pilot", "owner", "")
	if err != nil {
		t.Fatal(err)
	}
	session, err := accounts.Session(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	h, err := New(p, slog.New(slog.NewTextHandler(io.Discard, nil)), "test", []string{"system", "identity", "eve", "access", "market"}, AuthConfig{Origin: "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	call := func(method string, login, csrf bool) int {
		r := httptest.NewRequest(method, "/api/v1/market/settings", strings.NewReader(`{"ratio_bps":8500,"version":1}`))
		if login {
			r.AddCookie(&http.Cookie{Name: httpapi.SessionCookie, Value: token})
		}
		if csrf {
			r.Header.Set("Origin", "https://example.com")
			r.Header.Set("X-CSRF-Token", session.CSRFToken)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	if n := call("GET", false, false); n != 401 {
		t.Fatal("anonymous", n)
	}
	if n := call("GET", true, false); n != 200 {
		t.Fatal("member read", n)
	}
	if n := call("POST", true, false); n != 403 {
		t.Fatal("csrf", n)
	}
	if n := call("POST", true, true); n != 403 {
		t.Fatal("member write", n)
	}
	if _, err = p.Exec(ctx, `INSERT INTO access_administrators(user_id) VALUES($1)`, session.UserID); err != nil {
		t.Fatal(err)
	}
	if n := call("POST", true, true); n != 200 {
		t.Fatal("admin write", n)
	}
}

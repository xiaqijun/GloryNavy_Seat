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

func TestExchangeHostPermissionsAndCSRF(t *testing.T) {
	pool := testutil.Database(t)
	ctx := context.Background()
	accounts := identity.New(pool)
	token, err := accounts.SignIn(ctx, 101, "Pilot", "owner", "")
	if err != nil {
		t.Fatal(err)
	}
	session, err := accounts.Session(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	app, err := New(pool, slog.New(slog.NewTextHandler(io.Discard, nil)), "test", []string{"system", "identity", "eve", "access", "attendance", "exchange"}, AuthConfig{Origin: "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	call := func(method, path, body, csrf string, login bool) int {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Origin", "https://example.com")
		req.Header.Set("X-CSRF-Token", csrf)
		if login {
			req.AddCookie(&http.Cookie{Name: httpapi.SessionCookie, Value: token})
		}
		res := httptest.NewRecorder()
		app.ServeHTTP(res, req)
		return res.Code
	}
	for _, path := range []string{"/context", "/wallet", "/rewards", "/rewards/orders", "/rewards/types"} {
		if code := call("GET", "/api/v1/exchange"+path, "", "", false); code != 401 {
			t.Fatal("anonymous", path, code)
		}
	}
	for _, path := range []string{"/sources", "/rewards/rate", "/rewards/items", "/rewards/claim", "/rewards/orders/1"} {
		if code := call("POST", "/api/v1/exchange"+path, `{}`, "", true); code != 403 {
			t.Fatal("CSRF", path, code)
		}
	}
	for _, path := range []string{"/context", "/wallet", "/rewards", "/rewards/orders"} {
		if code := call("GET", "/api/v1/exchange"+path, "", "", true); code != 200 {
			t.Fatal("self", path, code)
		}
	}
	if code := call("GET", "/api/v1/exchange/rewards/orders?scope=all", "", "", true); code != 404 {
		t.Fatal("member read all", code)
	}
	body := `{"id":"pap","minor_per_unit":250,"version":"1","request_key":"11111111-1111-4111-8111-111111111119"}`
	if code := call("POST", "/api/v1/exchange/sources", body, session.CSRFToken, true); code != 404 {
		t.Fatal("member rate", code)
	}
	if _, err = pool.Exec(ctx, "INSERT INTO access_administrators(user_id) VALUES($1)", session.UserID); err != nil {
		t.Fatal(err)
	}
	if code := call("POST", "/api/v1/exchange/sources", body, session.CSRFToken, true); code != 200 {
		t.Fatal("admin rate", code)
	}
	otherToken, err := accounts.SignIn(ctx, 102, "Other", "other-owner", "")
	if err != nil {
		t.Fatal(err)
	}
	other, err := accounts.Session(ctx, otherToken)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/wallet", "/rewards"} {
		if code := call("GET", "/api/v1/exchange"+path+"?member="+other.UserID, "", "", true); code != 200 {
			t.Fatal("admin member read", code)
		}
	}
	if _, err = pool.Exec(ctx, "DELETE FROM access_administrators WHERE user_id=$1", session.UserID); err != nil {
		t.Fatal(err)
	}
	if code := call("POST", "/api/v1/exchange/sources", body, session.CSRFToken, true); code != 404 {
		t.Fatal("revoked admin replay", code)
	}
	if code := call("GET", "/api/v1/exchange/wallet?member="+other.UserID, "", "", true); code != 404 {
		t.Fatal("revoked member read", code)
	}

}

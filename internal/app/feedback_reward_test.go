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

func TestFeedbackRewardSettingsAuthorization(t *testing.T) {
	p := testutil.Database(t)
	ctx := context.Background()
	ids := identity.New(p)
	token, err := ids.SignIn(ctx, 104, "Config Pilot", "config-owner", "")
	if err != nil {
		t.Fatal(err)
	}
	session, err := ids.Session(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	h, err := New(p, slog.New(slog.NewTextHandler(io.Discard, nil)), "test", []string{"system", "identity", "eve", "access", "feedback"}, AuthConfig{Origin: "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	body := `{"request_key":"00000000-0000-4000-8000-000000000104","amount_minor":2500,"version":1}`
	call := func(method, body string, login, csrf bool) int {
		r := httptest.NewRequest(method, "/api/v1/feedback/reward-settings", strings.NewReader(body))
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
	if n := call("GET", "", false, false); n != 401 {
		t.Fatal(n)
	}
	if n := call("GET", "", true, false); n != 403 {
		t.Fatal(n)
	}
	if n := call("POST", body, true, true); n != 403 {
		t.Fatal(n)
	}
	if _, err = p.Exec(ctx, `INSERT INTO access_administrators(user_id) VALUES($1)`, session.UserID); err != nil {
		t.Fatal(err)
	}
	if n := call("POST", body, true, false); n != 403 {
		t.Fatal(n)
	}
	if n := call("GET", "", true, false); n != 200 {
		t.Fatal(n)
	}
	if n := call("POST", body, true, true); n != 200 {
		t.Fatal(n)
	}
	if _, err = p.Exec(ctx, `DELETE FROM access_administrators WHERE user_id=$1`, session.UserID); err != nil {
		t.Fatal(err)
	}
	if n := call("POST", body, true, true); n != 403 {
		t.Fatal(n)
	}
}

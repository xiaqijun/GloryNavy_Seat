package app

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/testutil"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAttendanceHostPermissionsAndCSRF(t *testing.T) {
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
	app, err := New(pool, slog.New(slog.NewTextHandler(io.Discard, nil)), "test", []string{"system", "identity", "eve", "access", "attendance"}, AuthConfig{Origin: "https://example.com"})
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
	for _, path := range []string{"/api/v1/attendance/context", "/api/v1/attendance/events", "/api/v1/attendance/events/1", "/api/v1/attendance/online", "/api/v1/attendance/pap", "/api/v1/attendance/pap/pending", "/api/v1/attendance/events/1/pap"} {
		if code := call("GET", path, "", "", false); code != 401 {
			t.Fatal("anonymous", path, code)
		}
	}
	if code := call("GET", "/api/v1/attendance/context", "", "", true); code != 200 {
		t.Fatal("own context", code)
	}
	if code := call("GET", "/api/v1/attendance/online?member=22222222-2222-4222-8222-222222222222", "", "", true); code != 404 {
		t.Fatal("member data exposed", code)
	}
	if code := call("GET", "/api/v1/attendance/online?corporation_id=10", "", "", true); code != 404 {
		t.Fatal("corporation data exposed", code)
	}
	if code := call("POST", "/api/v1/attendance/events/1/pap", `{}`, "", true); code != 403 {
		t.Fatal("PAP missing CSRF", code)
	}
	if code := call("POST", "/api/v1/attendance/events/1/conversion", `{}`, "", true); code != 403 {
		t.Fatal("conversion missing CSRF", code)
	}
	if code := call("GET", "/api/v1/attendance/events/1/conversion", "", "", true); code != 404 {
		t.Fatal("conversion admin required", code)
	}
	if code := call("GET", "/api/v1/attendance/pap?corporation_id=10", "", "", true); code != 404 {
		t.Fatal("PAP scope", code)
	}
	body := `{"corporation_id":"10","title":"Fleet","starts_at":"` + "2026-09-15T00:00:00Z" + `","request_key":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"}`
	if code := call("POST", "/api/v1/attendance/events", body, "", true); code != 403 {
		t.Fatal("missing csrf", code)
	}
	if code := call("POST", "/api/v1/attendance/events", body, session.CSRFToken, true); code != 404 {
		t.Fatal("member created activity", code)
	}
	if _, err = pool.Exec(ctx, "INSERT INTO access_administrators(user_id) VALUES($1)", session.UserID); err != nil {
		t.Fatal(err)
	}
	// A known event object remains readable by the current administrator.
	if _, err = pool.Exec(ctx, "INSERT INTO attendance_events(corporation_id,title,starts_at,created_by,request_key) VALUES(10,'Fleet',now(),$1,'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa')", session.UserID); err != nil {
		t.Fatal(err)
	}
	if code := call("GET", "/api/v1/attendance/events/1", "", "", true); code != 200 {
		t.Fatal("admin read", code)
	}
	if _, err = pool.Exec(ctx, "DELETE FROM access_administrators WHERE user_id=$1", session.UserID); err != nil {
		t.Fatal(err)
	}
	if code := call("GET", "/api/v1/attendance/events/1", "", "", true); code != 404 {
		t.Fatal("revoked admin retained access", code)
	}
}

func TestAttendanceReportUsesOneDatabaseConnection(t *testing.T) {
	original := testutil.Database(t)
	cfg := original.Config()
	cfg.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	accounts := identity.New(pool)
	token, err := accounts.SignIn(context.Background(), 101, "Pilot", "owner", "")
	if err != nil {
		t.Fatal(err)
	}
	app, err := New(pool, slog.New(slog.NewTextHandler(io.Discard, nil)), "test", []string{"system", "identity", "eve", "access", "attendance"}, AuthConfig{Origin: "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 3 {
		wg.Go(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			req := httptest.NewRequest("GET", "/api/v1/attendance/online", nil).WithContext(ctx)
			req.AddCookie(&http.Cookie{Name: httpapi.SessionCookie, Value: token})
			res := httptest.NewRecorder()
			app.ServeHTTP(res, req)
			if res.Code != 200 {
				t.Errorf("single-connection report blocked: %d", res.Code)
			}
		})
	}
	wg.Wait()
}

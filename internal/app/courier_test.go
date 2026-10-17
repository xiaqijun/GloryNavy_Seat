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

func TestCourierSessionCSRFAndModuleBoundary(t *testing.T) {
	p := testutil.Database(t)
	accounts := identity.New(p)
	token, err := accounts.SignIn(context.Background(), 101, "Pilot", "owner", "")
	if err != nil {
		t.Fatal(err)
	}
	s, err := accounts.Session(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	h, err := New(p, slog.New(slog.NewTextHandler(io.Discard, nil)), "test", []string{"system", "identity", "eve", "access", "courier"}, AuthConfig{Origin: "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		method, path string
		login, csrf  bool
		want         int
	}{
		{"GET", "routes", false, false, 401}, {"POST", "appraisal", false, false, 401}, {"POST", "appraisal", true, false, 403}, {"POST", "appraisal", true, true, 400},
	} {
		r := httptest.NewRequest(c.method, "/api/v1/courier/"+c.path, strings.NewReader(`{"appraisal_text":""}`))
		if c.login {
			r.AddCookie(&http.Cookie{Name: httpapi.SessionCookie, Value: token})
		}
		if c.csrf {
			r.Header.Set("Origin", "https://example.com")
			r.Header.Set("X-CSRF-Token", s.CSRFToken)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != c.want {
			t.Fatal(c, w.Code, w.Body.String())
		}
	}
	if _, err := New(p, slog.New(slog.NewTextHandler(io.Discard, nil)), "test", []string{"system", "courier"}, AuthConfig{}); err == nil {
		t.Fatal("missing auth dependencies accepted")
	}
	off, err := New(p, slog.New(slog.NewTextHandler(io.Discard, nil)), "test", []string{"system"}, AuthConfig{})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	off.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/courier/routes", nil))
	if w.Code != 404 {
		t.Fatal("disabled route available", w.Code)
	}
}

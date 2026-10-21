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
	"testing"
)

func TestLPReadOnlySessionAndModuleBoundary(t *testing.T) {
	p := testutil.Database(t)
	token, e := identity.New(p).SignIn(context.Background(), 101, "Pilot", "owner", "")
	if e != nil {
		t.Fatal(e)
	}
	h, e := New(p, slog.New(slog.NewTextHandler(io.Discard, nil)), "test", []string{"system", "identity", "eve", "access", "lp"}, AuthConfig{})
	if e != nil {
		t.Fatal(e)
	}
	for _, c := range []struct {
		method, path string
		login        bool
		code         int
	}{{"GET", "catalog", false, 401}, {"GET", "offers?corporation=x", false, 401}, {"GET", "prices?region=0&types=1", false, 401}, {"GET", "offers?corporation=x", true, 400}, {"GET", "prices?region=0&types=1", true, 400}, {"POST", "offers", true, 405}} {
		r := httptest.NewRequest(c.method, "/api/v1/lp/"+c.path, nil)
		if c.login {
			r.AddCookie(&http.Cookie{Name: httpapi.SessionCookie, Value: token})
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != c.code {
			t.Fatal(c, w.Code, w.Body.String())
		}
	}
	if _, e := New(p, slog.Default(), "test", []string{"system", "lp"}, AuthConfig{}); e == nil {
		t.Fatal("auth dependencies missing")
	}
	off, e := New(p, slog.Default(), "test", []string{"system"}, AuthConfig{})
	if e != nil {
		t.Fatal(e)
	}
	w := httptest.NewRecorder()
	off.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/lp/catalog", nil))
	if w.Code != 404 {
		t.Fatal(w.Code)
	}
}

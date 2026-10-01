package httpapi_test

import (
	"bytes"
	"context"
	"errors"
	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/module"
	"glorynavy.local/seat/internal/modules/system"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type statusFunc func(context.Context) (system.Status, error)

func (f statusFunc) Status(ctx context.Context) (system.Status, error) { return f(ctx) }

func TestHealthAndFailureContract(t *testing.T) {
	var logs bytes.Buffer
	calls := 0
	router := newRouter(t, statusFunc(func(ctx context.Context) (system.Status, error) {
		calls++
		if _, ok := ctx.Deadline(); !ok {
			t.Error("readiness query must have a deadline")
		}
		return system.Status{}, errors.New("secret database credential")
	}), slog.New(slog.NewJSONHandler(&logs, nil)))
	for _, tc := range []struct {
		path string
		want int
	}{{"/health/live", 200}, {"/health/ready", 503}, {"/api/v1/system/status", 503}, {"/unknown?token=secret", 404}} {
		req := httptest.NewRequest("GET", tc.path, nil)
		req.Header.Set("X-Request-ID", "untrusted")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != tc.want {
			t.Errorf("%s: status %d", tc.path, res.Code)
		}
		id := res.Header().Get("X-Request-ID")
		if id == "" || id == "untrusted" || !strings.Contains(res.Body.String(), id) {
			t.Error("missing server-generated request ID")
		}
		if strings.Contains(res.Body.String(), "secret") {
			t.Error("response leaked internal data")
		}
	}
	if calls != 2 {
		t.Errorf("liveness must not query database; calls=%d", calls)
	}
	if strings.Contains(logs.String(), "secret") {
		t.Error("logs leaked internal data")
	}
}

func TestPanicIsSanitized(t *testing.T) {
	var logs bytes.Buffer
	router := newRouter(t, statusFunc(func(context.Context) (system.Status, error) { panic("secret") }), slog.New(slog.NewJSONHandler(&logs, nil)))
	res := httptest.NewRecorder()
	router.ServeHTTP(res, httptest.NewRequest("GET", "/api/v1/system/status", nil))
	if res.Code != 500 || strings.Contains(res.Body.String()+logs.String(), "secret") {
		t.Fatal("panic response not sanitized")
	}
}

func newRouter(t *testing.T, service system.StatusService, logger *slog.Logger) http.Handler {
	t.Helper()
	status := system.StatusHandler(service, logger)
	registry, err := module.New([]module.Definition{system.Module(status)}, []string{"system"}, func(_ string, h http.Handler) http.Handler { return h })
	if err != nil {
		t.Fatal(err)
	}
	return httpapi.New(logger, registry, system.ReadyHandler(service))
}

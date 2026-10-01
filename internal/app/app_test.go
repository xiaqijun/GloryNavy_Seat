package app

import (
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"
)

func TestCompositionCatalogAndCoreHealth(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	// No database connection is needed to assemble modules, list them or check liveness.
	app, err := New(nil, logger, "test", []string{"system"}, AuthConfig{})
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]int{"/health/live": 200, "/api/v1/modules": 401, "/api/v1/system/status": 401} {
		res := httptest.NewRecorder()
		app.ServeHTTP(res, httptest.NewRequest("GET", path, nil))
		if res.Code != want {
			t.Fatalf("%s: %d", path, res.Code)
		}
	}
	for _, enabled := range [][]string{nil, {"system", "missing"}, {"system", "system"}} {
		if _, err := New(nil, logger, "test", enabled, AuthConfig{}); err == nil {
			t.Fatalf("invalid enabled modules accepted: %v", enabled)
		}
	}
}

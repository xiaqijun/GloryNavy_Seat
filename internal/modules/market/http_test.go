package market

import (
	"context"
	"glorynavy.local/seat/internal/modules/market/internal/store"
	"glorynavy.local/seat/internal/testutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSettingsAdminVersionAndAudit(t *testing.T) {
	p := testutil.Database(t)
	admin := false
	h := Handler{Service: &Service{Pool: p, Administrator: func(context.Context, string) (bool, error) { return admin, nil }}, User: func(*http.Request) string { return "00000000-0000-4000-8000-000000000001" }}
	call := func(body string) int {
		w := httptest.NewRecorder()
		h.configure(w, httptest.NewRequest("POST", "/settings", strings.NewReader(body)))
		return w.Code
	}
	if n := call(`{"ratio_bps":8000,"version":1}`); n != 403 {
		t.Fatal(n)
	}
	admin = true
	if n := call(`{"ratio_bps":8000,"version":1}`); n != 200 {
		t.Fatal(n)
	}
	if n := call(`{"ratio_bps":7000,"version":1}`); n != 409 {
		t.Fatal(n)
	}
	if n := call(`{"ratio_bps":100001,"version":2}`); n != 400 {
		t.Fatal(n)
	}
	v, err := store.Read(context.Background(), p)
	if err != nil || v.RatioBPS != 8000 || v.Version != 2 {
		t.Fatal(v, err)
	}
	var count int
	if err = p.QueryRow(context.Background(), `SELECT count(*) FROM market_settings_audit`).Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
}

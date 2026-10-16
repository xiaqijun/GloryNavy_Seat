package wallet

import (
	"context"
	"glorynavy.local/seat/internal/modules/eve"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWalletTaxFilterScopeAndOriginalAmounts(t *testing.T) {
	calls := 0
	h := Handler{User: func(*http.Request) string { return "actor" }, Authorize: func(context.Context, string, string, int64) (Owner, error) {
		return Owner{Divisions: []int{0}, Journal: true}, nil
	}, Data: func(_ context.Context, f eve.WalletFilter) ([]map[string]any, error) {
		calls++
		if f.Direction != "tax" {
			t.Fatal(f)
		}
		return []map[string]any{{"id": "100", "amount": "85.00", "tax": "15.12345", "tax_receiver_id": "10"}}, nil
	}}
	for _, tc := range []struct {
		query  string
		status int
	}{
		{"owner_kind=character&owner_id=123&part=journal&direction=tax", 200},
		{"owner_kind=corporation&owner_id=10&division=1&part=journal&direction=tax", 400},
		{"owner_kind=character&owner_id=123&part=transactions&direction=tax", 400},
		{"owner_kind=character&owner_id=123&part=balance&direction=tax", 400},
	} {
		w := httptest.NewRecorder()
		h.records(w, httptest.NewRequest("GET", "/?"+tc.query, nil))
		if w.Code != tc.status {
			t.Fatalf("%s: %d %s", tc.query, w.Code, w.Body.String())
		}
	}
	if calls != 1 {
		t.Fatal("invalid tax scope read data", calls)
	}
}

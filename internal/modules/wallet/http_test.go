package wallet

import (
	"context"
	"encoding/json"
	"glorynavy.local/seat/internal/modules/eve"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWalletRejectsInvalidAndUnauthorizedQueriesBeforeRead(t *testing.T) {
	calls := 0
	h := Handler{User: func(*http.Request) string { return "actor" }, Authorize: func(context.Context, string, string, int64) (Owner, error) {
		return Owner{Divisions: []int{2}, Journal: true}, nil
	}, Data: func(context.Context, eve.WalletFilter) ([]map[string]any, error) {
		calls++
		return []map[string]any{}, nil
	}}
	for _, tc := range []struct {
		query  string
		status int
	}{
		{"owner_kind=corporation&owner_id=10&division=1&part=journal", 404},
		{"owner_kind=corporation&owner_id=10&division=2&part=transactions", 404},
		{"owner_kind=corporation&owner_id=10&division=8&part=journal", 400},
		{"owner_kind=character&owner_id=1e5&part=journal", 400},
		{"owner_kind=character&owner_id=001&part=journal", 400},
		{"owner_kind=character&owner_id=123&part=journal&before=9223372036854775808", 400},
		{"owner_kind=character&owner_id=123&part=journal&direction=buy", 400},
		{"owner_kind=character&owner_id=123&part=journal&from=2026-09-02T00:00:00Z&until=2026-09-01T00:00:00Z", 400},
	} {
		w := httptest.NewRecorder()
		h.records(w, httptest.NewRequest("GET", "/?"+tc.query, nil))
		if w.Code != tc.status {
			t.Fatalf("%s: %d", tc.query, w.Code)
		}
	}
	if calls != 0 {
		t.Fatal("unauthorized read")
	}
}
func TestWalletSummaryUsesOnlyCurrentAccountCharacterOwners(t *testing.T) {
	called := 0
	h := Handler{
		User: func(*http.Request) string { return "actor" },
		Owners: func(_ context.Context, actor, member string) ([]Owner, error) {
			if actor != "actor" || member != "" {
				t.Fatalf("unexpected wallet scope %q %q", actor, member)
			}
			return []Owner{
				{Kind: "character", ID: 11, Divisions: []int{0}, Journal: true},
				{Kind: "corporation", ID: 22, Divisions: []int{1}, Journal: true},
				{Kind: "character", ID: 33, Divisions: []int{0}, Journal: false},
			}, nil
		},
		Summaries: func(_ context.Context, ids []int64, _ time.Time) ([]eve.WalletSummary, error) {
			called++
			if len(ids) != 1 || ids[0] != 11 {
				t.Fatal(ids)
			}
			return []eve.WalletSummary{{OwnerID: 11, Income: "0", Expense: "0"}}, nil
		},
	}
	for _, query := range []string{"", "from=2000-01-01T00%3A00%3A00Z"} {
		w := httptest.NewRecorder()
		h.summary(w, httptest.NewRequest("GET", "/?"+query, nil))
		if w.Code != 400 {
			t.Fatal(query, w.Code)
		}
	}
	from := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	w := httptest.NewRecorder()
	h.summary(w, httptest.NewRequest("GET", "/?member=other&from="+from, nil))
	if w.Code != 200 || called != 1 || strings.Contains(w.Body.String(), `"owner_id":"22"`) {
		t.Fatal(w.Code, called, w.Body.String())
	}
}
func TestWalletDivisionNamesDoNotLeakOtherDivisionsOrHangars(t *testing.T) {
	h := Handler{User: func(*http.Request) string { return "actor" }, Authorize: func(context.Context, string, string, int64) (Owner, error) {
		return Owner{Divisions: []int{2}, Journal: true}, nil
	}, Data: func(_ context.Context, f eve.WalletFilter) ([]map[string]any, error) {
		if f.Division != 0 {
			t.Fatal(f)
		}
		return []map[string]any{{"id": "0", "wallet": json.RawMessage(`[{"division":1,"name":"secret"},{"division":2,"name":"allowed"}]`), "hangar": json.RawMessage(`[{"name":"hangar-secret"}]`)}}, nil
	}}
	w := httptest.NewRecorder()
	h.records(w, httptest.NewRequest("GET", "/?owner_kind=corporation&owner_id=10&division=2&part=divisions", nil))
	if w.Code != 200 || strings.Contains(w.Body.String(), "secret") || !strings.Contains(w.Body.String(), "allowed") {
		t.Fatal(w.Body.String())
	}
}

func TestCorporationSummaryRequiresReadableJournalAndReturnsAggregate(t *testing.T) {
	from := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	called := false
	balance := "123"
	h := Handler{
		User: func(*http.Request) string { return "actor" },
		Authorize: func(_ context.Context, actor, kind string, id int64) (Owner, error) {
			if actor != "actor" || kind != "corporation" || id != 10 {
				t.Fatal(actor, kind, id)
			}
			return Owner{Kind: "corporation", ID: 10, Journal: true, Divisions: []int{1, 3}}, nil
		},
		CorporationSummary: func(_ context.Context, id int64, divisions []int, got time.Time) (eve.WalletSummary, error) {
			called = true
			if id != 10 || len(divisions) != 2 || got.IsZero() {
				t.Fatal(id, divisions, got)
			}
			return eve.WalletSummary{OwnerID: 10, Balance: &balance, Income: "100", Expense: "25"}, nil
		},
	}
	w := httptest.NewRecorder()
	h.corporationSummary(w, httptest.NewRequest("GET", "/?owner_id=10&from="+from, nil))
	if w.Code != 200 || !called || !strings.Contains(w.Body.String(), `"owner_id":"10"`) {
		t.Fatal(w.Code, called, w.Body.String())
	}
}

func TestCorporationFinanceTrendRequiresReadableJournalAndReturnsTax(t *testing.T) {
	from := time.Now().UTC().Add(-30 * 24 * time.Hour).Format(time.RFC3339)
	until := time.Now().UTC().Format(time.RFC3339)
	called := false
	h := Handler{
		User: func(*http.Request) string { return "actor" },
		Authorize: func(_ context.Context, actor, kind string, id int64) (Owner, error) {
			if actor != "actor" || kind != "corporation" || id != 10 {
				t.Fatal(actor, kind, id)
			}
			return Owner{Kind: "corporation", ID: 10, Journal: true, Divisions: []int{1}}, nil
		},
		CorporationFinanceTrend: func(_ context.Context, id int64, divisions []int, gotFrom, gotUntil time.Time) ([]eve.WalletFinanceTrend, error) {
			called = true
			if id != 10 || len(divisions) != 1 || gotFrom.IsZero() || gotUntil.Before(gotFrom) {
				t.Fatal(id, divisions, gotFrom, gotUntil)
			}
			return []eve.WalletFinanceTrend{{Period: "2026-09", Income: "100", Expense: "25", Tax: "5", Net: "75"}}, nil
		},
	}
	w := httptest.NewRecorder()
	h.corporationFinanceTrend(w, httptest.NewRequest("GET", "/?owner_id=10&from="+from+"&until="+until, nil))
	if w.Code != 200 || !called || !strings.Contains(w.Body.String(), `"tax":"5"`) {
		t.Fatalf("%d %t %s", w.Code, called, w.Body.String())
	}
}

func TestPersonalCorporationSummaryUsesMemberReadScope(t *testing.T) {
	from := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	called := false
	balance := "456"
	h := Handler{
		User: func(*http.Request) string { return "admin" },
		CorporationPersonalSummary: func(_ context.Context, actor string, id int64, got time.Time) (eve.WalletSummary, error) {
			called = true
			if actor != "admin" || id != 10 || got.IsZero() {
				t.Fatal(actor, id, got)
			}
			return eve.WalletSummary{OwnerID: id, Balance: &balance, Income: "1000", Expense: "25"}, nil
		},
	}
	w := httptest.NewRecorder()
	h.personalCorporationSummary(w, httptest.NewRequest("GET", "/?corporation_id=10&from="+from, nil))
	if w.Code != 200 || !called || !strings.Contains(w.Body.String(), `"owner_id":"10"`) {
		t.Fatal(w.Code, called, w.Body.String())
	}
}

func TestPersonalCorporationIncomeTrendValidatesRangeAndReturnsItems(t *testing.T) {
	from := time.Now().UTC().Add(-30 * 24 * time.Hour).Format(time.RFC3339)
	until := time.Now().UTC().Format(time.RFC3339)
	called := false
	h := Handler{
		User: func(*http.Request) string { return "admin" },
		CorporationPersonalIncomeTrend: func(_ context.Context, actor string, id int64, gotFrom, gotUntil time.Time) ([]eve.WalletIncomeTrend, error) {
			called = true
			if actor != "admin" || id != 10 || gotFrom.IsZero() || gotUntil.Before(gotFrom) {
				t.Fatalf("unexpected trend scope: %q %d %s %s", actor, id, gotFrom, gotUntil)
			}
			return []eve.WalletIncomeTrend{{Period: "2026-09", Income: "100", Expense: "25", ActiveMembers: 2}}, nil
		},
	}
	w := httptest.NewRecorder()
	h.personalCorporationIncomeTrend(w, httptest.NewRequest("GET", "/?corporation_id=10&from="+from+"&until="+until, nil))
	if w.Code != 200 || !called || !strings.Contains(w.Body.String(), `"period":"2026-09"`) {
		t.Fatal(w.Code, called, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.personalCorporationIncomeTrend(w, httptest.NewRequest("GET", "/?corporation_id=10&from="+from+"&until=2000-01-01T00%3A00%3A00Z", nil))
	if w.Code != 400 {
		t.Fatalf("future/invalid range should be rejected: %d", w.Code)
	}
}

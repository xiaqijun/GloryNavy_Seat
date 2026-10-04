package structures

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/eve"
)

func TestStructuresRejectsInvalidCorporationID(t *testing.T) {
	called := false
	h := Handler{User: func(*http.Request) string { return "user" }, Read: func(context.Context, string, int64) ([]eve.Structure, error) {
		called = true
		return nil, nil
	}}
	w := httptest.NewRecorder()
	h.structures(w, httptest.NewRequest(http.MethodGet, "/structures?corporation_id=01", nil))
	if w.Code != http.StatusBadRequest || called {
		t.Fatalf("status=%d called=%v", w.Code, called)
	}
}

func TestStructuresReturnsAuthorizedRows(t *testing.T) {
	var gotUser string
	var gotCorp int64
	h := Handler{User: func(*http.Request) string { return "user-1" }, Read: func(_ context.Context, user string, corporationID int64) ([]eve.Structure, error) {
		gotUser, gotCorp = user, corporationID
		return []eve.Structure{{CorporationID: corporationID, CorporationName: "Test Corp", Kind: "pos", ID: 9001, State: "online"}}, nil
	}}
	w := httptest.NewRecorder()
	h.structures(w, httptest.NewRequest(http.MethodGet, "/structures?corporation_id=123", nil))
	if w.Code != http.StatusOK || gotUser != "user-1" || gotCorp != 123 {
		t.Fatalf("status=%d user=%q corp=%d", w.Code, gotUser, gotCorp)
	}
	body, _ := io.ReadAll(w.Result().Body)
	if !strings.Contains(string(body), `"items"`) || !strings.Contains(string(body), `"9001"`) {
		t.Fatalf("response=%s", body)
	}
}

func TestStructuresMapsObjectAndUpstreamFailures(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		code int
	}{
		{name: "object denied", err: pgx.ErrNoRows, code: http.StatusNotFound},
		{name: "upstream unavailable", err: errors.New("esi unavailable"), code: http.StatusServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := Handler{User: func(*http.Request) string { return "user" }, Read: func(context.Context, string, int64) ([]eve.Structure, error) { return nil, tc.err }}
			w := httptest.NewRecorder()
			h.structures(w, httptest.NewRequest(http.MethodGet, "/structures", nil))
			if w.Code != tc.code {
				t.Fatalf("status=%d want=%d", w.Code, tc.code)
			}
		})
	}
}

func TestStructuresNormalizesNilItems(t *testing.T) {
	h := Handler{User: func(*http.Request) string { return "user" }, Read: func(context.Context, string, int64) ([]eve.Structure, error) { return nil, nil }}
	w := httptest.NewRecorder()
	h.structures(w, httptest.NewRequest(http.MethodGet, "/structures", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	body, _ := io.ReadAll(w.Result().Body)
	if !strings.Contains(string(body), `"items":[]`) {
		t.Fatalf("response=%s", body)
	}
}

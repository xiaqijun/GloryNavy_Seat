package attendance

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestFleetNoticeHTTPLongContent(t *testing.T) {
	s, _ := attendanceFixture(t)
	e := createTestEvent(t, s)
	h := Handler{Service: s, User: func(*http.Request) string { return manager }}
	input := FleetNoticeInput{SolarSystem: "Jita", Doctrine: "战舰", Details: strings.Repeat("舰", 1500), RequestKey: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"}
	body, _ := json.Marshal(input)
	if len(body) <= 4096 {
		t.Fatal("test must exceed the shared request limit")
	}
	r := httptest.NewRequest(http.MethodPut, "/fleet-notice", bytes.NewReader(body))
	route := chi.NewRouteContext()
	route.URLParams.Add("id", strconv.FormatInt(e.ID, 10))
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, route))
	w := httptest.NewRecorder()
	h.saveFleetNotice(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("save: %d %s", w.Code, w.Body.String())
	}
}

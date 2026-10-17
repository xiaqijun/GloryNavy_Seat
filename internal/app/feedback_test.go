package app

import (
	"context"
	"encoding/json"
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

func TestFeedbackHostAuthorizationAndCSRF(t *testing.T) {
	p := testutil.Database(t)
	ctx := context.Background()
	accounts := identity.New(p)
	token, e := accounts.SignIn(ctx, 101, "Pilot", "owner", "")
	if e != nil {
		t.Fatal(e)
	}
	s, e := accounts.Session(ctx, token)
	if e != nil {
		t.Fatal(e)
	}
	h, e := New(p, slog.New(slog.NewTextHandler(io.Discard, nil)), "test", []string{"system", "identity", "eve", "access", "feedback"}, AuthConfig{Origin: "https://example.com"})
	if e != nil {
		t.Fatal(e)
	}
	body := `{"request_key":"00000000-0000-4000-8000-000000000001","kind":"feature","title":"功能建议","body":"增加筛选"}`
	call := func(method, path, body string, login, csrf bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/api/v1/feedback/"+path, strings.NewReader(body))
		if login {
			r.AddCookie(&http.Cookie{Name: httpapi.SessionCookie, Value: token})
		}
		if csrf {
			r.Header.Set("Origin", "https://example.com")
			r.Header.Set("X-CSRF-Token", s.CSRFToken)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	for _, c := range []struct {
		method, path, body string
		login, csrf        bool
		want               int
	}{{"GET", "tickets", "", false, false, 401}, {"POST", "tickets", body, true, false, 403}, {"GET", "tickets?scope=all", "", true, false, 403}, {"POST", "tickets", body + `{}`, true, true, 400}, {"POST", "tickets", body, true, true, 200}} {
		w := call(c.method, c.path, c.body, c.login, c.csrf)
		if w.Code != c.want {
			t.Fatal(c, w.Code, w.Body.String())
		}
	}
	w := call("GET", "tickets", "", true, false)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var response struct {
		Data struct {
			Tickets []struct {
				ID string `json:"id"`
			} `json:"tickets"`
		} `json:"data"`
	}
	if e = json.Unmarshal(w.Body.Bytes(), &response); e != nil || len(response.Data.Tickets) != 1 {
		t.Fatal(e, w.Body.String())
	}
	if _, e = p.Exec(ctx, "INSERT INTO access_administrators(user_id) VALUES($1)", s.UserID); e != nil {
		t.Fatal(e)
	}
	if w = call("GET", "tickets?scope=all", "", true, false); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	p.Exec(ctx, "DELETE FROM access_administrators WHERE user_id=$1", s.UserID)
	if w = call("POST", "tickets/"+response.Data.Tickets[0].ID+"/messages", `{"request_key":"00000000-0000-4000-8000-000000000002","revision":1,"body":"","status":"resolved"}`, true, true); w.Code != 403 {
		t.Fatal(w.Code, w.Body.String())
	}
	off, e := New(p, slog.New(slog.NewTextHandler(io.Discard, nil)), "test", []string{"system"}, AuthConfig{})
	if e != nil {
		t.Fatal(e)
	}
	w = httptest.NewRecorder()
	off.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/feedback/tickets", nil))
	if w.Code != 404 {
		t.Fatal(w.Code)
	}
}

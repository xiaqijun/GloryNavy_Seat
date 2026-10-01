package identity

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/testutil"
)

func TestLoginRotationExpiryAndOwnership(t *testing.T) {
	pool := testutil.Database(t)
	s := New(pool)
	ctx := context.Background()
	token, err := s.SignIn(ctx, 123, "Pilot", "owner-a", "")
	if err != nil {
		t.Fatal(err)
	}
	session, err := s.Session(ctx, token)
	if err != nil || session == nil || session.Character.ID != "123" {
		t.Fatalf("session: %+v %v", session, err)
	}
	if remaining := time.Until(session.ExpiresAt); remaining > 7*24*time.Hour || remaining < 7*24*time.Hour-time.Minute {
		t.Fatal("session lifetime must be seven days", remaining)
	}
	rotated, err := s.SignIn(ctx, 123, "Renamed Pilot", "owner-a", token)
	if err != nil {
		t.Fatal(err)
	}
	if old, _ := s.Session(ctx, token); old != nil {
		t.Fatal("old session survived rotation")
	}
	current, _ := s.Session(ctx, rotated)
	if current.UserID != session.UserID || current.Character.Name != "Renamed Pilot" || current.CSRFToken == session.CSRFToken {
		t.Fatal("rotation or identity mismatch")
	}
	if _, err = s.SignIn(ctx, 123, "Renamed Pilot", "owner-b", ""); !errors.Is(err, ErrOwnership) {
		t.Fatalf("transfer accepted: %v", err)
	}
	if current, _ := s.Session(ctx, rotated); current != nil {
		t.Fatal("transferred character still has access")
	}
	if _, err = s.SignIn(ctx, 123, "Pilot", "owner-a", ""); !errors.Is(err, ErrOwnership) {
		t.Fatal("quarantined identity restored without review")
	}
	token, err = s.SignIn(ctx, 456, "Second Pilot", "owner-c", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, "UPDATE identity_sessions SET expires_at=now()-interval '1 second'"); err != nil {
		t.Fatal(err)
	}
	if expired, _ := s.Session(ctx, token); expired != nil {
		t.Fatal("expired session accepted")
	}
}

func TestConcurrentFirstLoginCreatesOneUser(t *testing.T) {
	pool := testutil.Database(t)
	s := New(pool)
	ctx := context.Background()
	var wg sync.WaitGroup
	for range 6 {
		wg.Go(func() {
			if _, err := s.SignIn(ctx, 999, "Pilot", "owner", ""); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	var users, sessions int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM identity_users").Scan(&users); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM identity_sessions").Scan(&sessions); err != nil {
		t.Fatal(err)
	}
	if users != 1 || sessions != 5 {
		t.Fatalf("users=%d sessions=%d", users, sessions)
	}
}

func TestAuthorizationAndLogoutCSRF(t *testing.T) {
	pool := testutil.Database(t)
	s := New(pool)
	ctx := context.Background()
	token, err := s.SignIn(ctx, 123, "Pilot", "owner", "")
	if err != nil {
		t.Fatal(err)
	}
	session, _ := s.Session(ctx, token)
	h := Handler{Service: s, Origin: "https://example.com", Secure: true}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if Principal(r.Context()) == nil {
			t.Error("missing principal")
		}
		h.logout(w, r)
	})
	for _, tc := range []struct {
		name, origin, csrf, token, permission string
		want                                  int
	}{
		{"anonymous", "https://example.com", session.CSRFToken, "", "identity.session.logout", 401},
		{"foreign origin", "https://evil.example", session.CSRFToken, token, "identity.session.logout", 403},
		{"missing csrf", "https://example.com", "", token, "identity.session.logout", 403},
		{"unknown permission", "https://example.com", session.CSRFToken, token, "members.admin", 403},
		{"valid", "https://example.com", session.CSRFToken, token, "identity.session.logout", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/", nil)
			req.Header.Set("Origin", tc.origin)
			req.Header.Set("X-CSRF-Token", tc.csrf)
			req.AddCookie(&http.Cookie{Name: httpapi.SessionCookie, Value: tc.token})
			res := httptest.NewRecorder()
			h.Authorize(tc.permission, next).ServeHTTP(res, req)
			if res.Code != tc.want {
				t.Fatalf("status=%d: %s", res.Code, res.Body.String())
			}
			if tc.want == 200 {
				for _, cookie := range res.Result().Cookies() {
					if !cookie.HttpOnly || !cookie.Secure || cookie.MaxAge != -1 {
						t.Fatal("unsafe cookie removal")
					}
				}
			}
		})
	}
	if session, _ := s.Session(ctx, token); session != nil {
		t.Fatal("logout left session usable")
	}
	res := httptest.NewRecorder()
	h.current(res, httptest.NewRequest("GET", "/", nil))
	body, _ := io.ReadAll(res.Result().Body)
	if !strings.Contains(string(body), `"authenticated":false`) {
		t.Fatal("anonymous contract")
	}
}

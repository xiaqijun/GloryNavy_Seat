package identity

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/testutil"
)

func TestBrowserSlidingSessionAndCookie(t *testing.T) {
	ctx := context.Background()
	pool := testutil.Database(t)
	s := New(pool)
	token, err := s.SignIn(ctx, 123, "Pilot", "owner", "")
	if err != nil {
		t.Fatal(err)
	}
	h := Handler{Service: s, Origin: "https://example.com", Secure: true}
	age := func() time.Time {
		t.Helper()
		var v time.Time
		if err := pool.QueryRow(ctx, `UPDATE identity_sessions SET expires_at=now()+interval '1 hour' WHERE token_hash=$1 RETURNING expires_at`, httpapi.Hash(token)).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	request := func(method string, handler http.Handler, origin, site string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/", nil)
		r.Header.Set("Origin", origin)
		r.Header.Set("Sec-Fetch-Site", site)
		r.AddCookie(&http.Cookie{Name: httpapi.SessionCookie, Value: token})
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	read := http.HandlerFunc(h.current)
	age()
	w := request("GET", read, "", "same-origin")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	session, err := s.Session(ctx, token)
	if err != nil || session == nil {
		t.Fatal(err)
	}
	if time.Until(session.ExpiresAt) < SessionLifetime-time.Minute {
		t.Fatal("active old session did not renew")
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Value != token || cookies[0].MaxAge < 604790 || cookies[0].MaxAge > 604800 || !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatal("cookie not renewed safely")
	}
	w = request("GET", read, "", "same-origin")
	second, _ := s.Session(ctx, token)
	if w.Code != 200 || !second.ExpiresAt.Equal(session.ExpiresAt) {
		t.Fatal("frequent poll rewrote session expiry")
	}
	if len(w.Result().Cookies()) != 1 {
		t.Fatal("session endpoint must restore cookie after a lost renewal response")
	}
	for _, tc := range []struct {
		name, method, origin, site string
		handler                    http.Handler
	}{
		{"cross-site read", "GET", "https://other.example", "cross-site", read},
		{"denied capability", "GET", "", "same-origin", h.Authorize("unregistered", http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("denied request reached business handler") }))},
		{"invalid csrf", "POST", h.Origin, "same-origin", h.Authorize("identity.characters.manage", http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("CSRF failure reached handler") }))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := age()
			request(tc.method, tc.handler, tc.origin, tc.site)
			after, _ := s.Session(ctx, token)
			if after == nil || !after.ExpiresAt.Equal(before) {
				t.Fatal("rejected or foreign request renewed session")
			}
		})
	}
	age()
	allowed := h.Authorize("identity.characters.read", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if time.Until(Principal(r.Context()).ExpiresAt) < SessionLifetime-time.Minute {
			t.Error("principal expiry not updated")
		}
		w.WriteHeader(204)
	}))
	if w = request("GET", allowed, "", "same-origin"); w.Code != 204 || len(w.Result().Cookies()) != 1 {
		t.Fatal("authorized API did not renew")
	}
	if err = s.Logout(ctx, token); err != nil {
		t.Fatal(err)
	}
	w = request("GET", read, "", "same-origin")
	for _, c := range w.Result().Cookies() {
		if c.MaxAge != -1 {
			t.Fatal("revoked session resurrected cookie")
		}
	}
}

func TestSlidingRenewalCannotResurrectExpiredBlockedOrRevokedSession(t *testing.T) {
	ctx := context.Background()
	pool := testutil.Database(t)
	s := New(pool)
	for index, reason := range []string{"expired", "blocked", "revoked"} {
		t.Run(reason, func(t *testing.T) {
			id := int64(123 + index)
			token, err := s.SignIn(ctx, id, reason, "owner", "")
			if err != nil {
				t.Fatal(err)
			}
			_, err = pool.Exec(ctx, `UPDATE identity_sessions SET expires_at=now()+interval '1 hour' WHERE token_hash=$1`, httpapi.Hash(token))
			if err != nil {
				t.Fatal(err)
			}
			stale, _ := s.Session(ctx, token)
			switch reason {
			case "expired":
				_, err = pool.Exec(ctx, `UPDATE identity_sessions SET expires_at=now()-interval '1 second' WHERE token_hash=$1`, httpapi.Hash(token))
			case "blocked":
				_, err = pool.Exec(ctx, `UPDATE identity_characters SET status='blocked' WHERE character_id=$1`, id)
			case "revoked":
				err = s.Logout(ctx, token)
			}
			if err != nil {
				t.Fatal(err)
			}
			if renewed, err := s.RenewSession(ctx, token, stale); renewed || !errors.Is(err, pgx.ErrNoRows) {
				t.Fatal("invalid session renewed", reason, err)
			}
			if found, _ := s.Session(ctx, token); found != nil {
				t.Fatal("session resurrected")
			}
		})
	}
}

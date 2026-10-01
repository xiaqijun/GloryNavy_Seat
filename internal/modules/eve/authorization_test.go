package eve

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/testutil"
)

func TestCredentialEncryption(t *testing.T) {
	s, err := NewAuthorization(nil, nil, base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{42}, 32)), nil)
	if err != nil {
		t.Fatal(err)
	}
	a := &authorization{AccessToken: "private-access", RefreshToken: "private-refresh"}
	owner := httpapi.Hash("owner")
	sealed, err := s.seal(123, owner, a)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, []byte(a.RefreshToken)) {
		t.Fatal("plaintext token persisted")
	}
	if got, err := s.open(123, owner, sealed); err != nil || got.RefreshToken != a.RefreshToken {
		t.Fatal("decrypt failed")
	}
	if _, err = s.open(124, owner, sealed); err == nil {
		t.Fatal("credential moved to another character")
	}
	if _, err = s.open(123, httpapi.Hash("other-owner"), sealed); err == nil {
		t.Fatal("credential moved to another owner")
	}
	sealed[len(sealed)-1] ^= 1
	if _, err = s.open(123, owner, sealed); err == nil {
		t.Fatal("tampered ciphertext accepted")
	}
}
func TestRefreshScopeLossRequiresReauthorization(t *testing.T) {
	pool := testutil.Database(t)
	ctx := context.Background()
	client, key, m := fixture(t)
	client.EnableSeatDefaultScopes()
	previous := client.http.Transport
	client.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != m.Token {
			return previous.RoundTrip(r)
		}
		cl := validClaims()
		cl.Scopes = []string{CorporationRolesScope}
		cl.ExpiresAt = jwt.NewNumericDate(time.Now().Add(time.Hour))
		return jsonResponse(tokenResponse{sign(t, key, cl), "rotated-refresh", "Bearer"}), nil
	})
	s, err := NewAuthorization(pool, client, base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := NewSync(pool, s, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	if err != nil {
		t.Fatal(err)
	}
	s.esi.http.Transport = transportFunc(func(*http.Request) (*http.Response, error) {
		t.Error("ESI must not be queried after the grant loses a scope")
		return nil, errESI
	})
	ch := Character{ID: 123, Name: "Pilot", Owner: "owner", authorization: &authorization{
		AccessToken: "expired", RefreshToken: "old-refresh", ExpiresAt: time.Now().Add(-time.Minute),
		Scopes: []string{CorporationRolesScope, "esi-wallet.read_character_wallet.v1"},
	}}
	if err = s.Save(ctx, ch); err != nil {
		t.Fatal(err)
	}
	target := authorizationTarget(t, runtime)
	if err := runtime.work(ctx, syncArgs{target.ID, target.Generation}, target.ActiveJobID.Int64, "authorization"); err != nil {
		t.Fatalf("refresh did not run: %v", err)
	}
	a, err := s.Get(ctx, ch.ID)
	if err != nil || a.State != "reauthorize" || len(a.Scopes) != 0 || !a.SyncedAt.IsZero() {
		t.Fatalf("reduced grant was retained: %v", err)
	}
	var sealed []byte
	if err = pool.QueryRow(ctx, "SELECT sealed FROM eve_credentials WHERE character_id=123").Scan(&sealed); err != nil || len(sealed) != 0 {
		t.Fatal("reduced grant credential not erased")
	}
}

func TestESICacheAndBackoff(t *testing.T) {
	pool := testutil.Database(t)
	c := newESI(pool, nil)
	calls := 0
	c.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("Authorization") != "" {
			t.Error("token sent to public request")
		}
		res := jsonResponse(map[string]string{"name": "Corp"})
		res.Header.Set("Expires", time.Now().Add(time.Hour).UTC().Format(http.TimeFormat))
		return res, nil
	})
	for range 2 {
		var out map[string]string
		if _, err := c.request(context.Background(), "GET", "/corporations/10/", "", nil, &out); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatal("Expires was ignored")
	}
	if _, err := pool.Exec(context.Background(), "UPDATE eve_esi_limits SET next_request_at=now()"); err != nil {
		t.Fatal(err)
	}
	c.http.Transport = transportFunc(func(*http.Request) (*http.Response, error) {
		calls++
		res := jsonResponse(nil)
		res.StatusCode = 429
		res.Header.Set("Retry-After", "3600")
		return res, nil
	})
	var out any
	_, err := c.request(context.Background(), "GET", "/corporations/20/", "", nil, &out)
	var retry retryError
	if !errors.As(err, &retry) || retry.Until.Before(time.Now().Add(59*time.Minute)) {
		t.Fatal("Retry-After ignored")
	}
	_, err = c.request(context.Background(), "GET", "/corporations/20/", "", nil, &out)
	if !errors.As(err, &retry) || calls != 2 {
		t.Fatal("rate limit not shared")
	}
}

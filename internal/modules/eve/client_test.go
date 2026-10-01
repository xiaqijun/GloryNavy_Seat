package eve

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func jsonResponse(value any) *http.Response {
	data, _ := json.Marshal(value)
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(data))), Header: http.Header{}}
}
func fixture(t *testing.T) (*Client, *rsa.PrivateKey, metadata) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	m := metadata{Issuer: issuer, Authorize: issuer + "/v2/oauth/authorize", Token: issuer + "/v2/oauth/token", JWKS: issuer + "/oauth/jwks"}
	c := NewClient("client-id", "secret", "http://127.0.0.1:5173/api/v1/eve/callback")
	c.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/.well-known/oauth-authorization-server":
			return jsonResponse(m), nil
		case "/oauth/jwks":
			return jsonResponse(map[string]any{"keys": []any{map[string]string{"kty": "RSA", "kid": "test-key", "alg": "RS256", "use": "sig", "n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())}}}), nil
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			return nil, ErrSSO
		}
	})
	return c, key, m
}
func validClaims() claims {
	return claims{RegisteredClaims: jwt.RegisteredClaims{Issuer: issuer, Subject: "CHARACTER:EVE:123", Audience: jwt.ClaimStrings{"EVE Online", "client-id"}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)), IssuedAt: jwt.NewNumericDate(time.Now().Add(-time.Minute))}, Name: "Pilot", Owner: "owner", AuthorizedParty: "client-id", Tenant: "tranquility"}
}
func sign(t *testing.T, key *rsa.PrivateKey, cl claims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, cl)
	token.Header["kid"] = "test-key"
	raw, err := token.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestJWTRejectsInvalidIdentity(t *testing.T) {
	c, key, m := fixture(t)
	character, err := c.verify(context.Background(), m, sign(t, key, validClaims()))
	if err != nil || character.ID != 123 {
		t.Fatalf("valid token: %v %v", character, err)
	}
	for _, tc := range []struct {
		name   string
		change func(*claims)
	}{
		{"issuer", func(c *claims) { c.Issuer = "https://evil.example" }},
		{"client audience", func(c *claims) { c.Audience = jwt.ClaimStrings{"EVE Online"} }},
		{"eve audience", func(c *claims) { c.Audience = jwt.ClaimStrings{"client-id"} }},
		{"expired", func(c *claims) { c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Hour)) }},
		{"missing expiry", func(c *claims) { c.ExpiresAt = nil }},
		{"future token", func(c *claims) { c.NotBefore = jwt.NewNumericDate(time.Now().Add(time.Hour)) }},
		{"azp", func(c *claims) { c.AuthorizedParty = "another-client" }},
		{"tenant", func(c *claims) { c.Tenant = "serenity" }},
		{"owner", func(c *claims) { c.Owner = "" }},
		{"subject", func(c *claims) { c.Subject = "CHARACTER:EVE:+123" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cl := validClaims()
			tc.change(&cl)
			if _, err := c.verify(context.Background(), m, sign(t, key, cl)); err == nil {
				t.Fatal("invalid token accepted")
			}
		})
	}
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.verify(context.Background(), m, sign(t, other, validClaims())); err == nil {
		t.Fatal("wrong signature accepted")
	}
	hs := jwt.NewWithClaims(jwt.SigningMethodHS256, validClaims())
	hs.Header["kid"] = "test-key"
	raw, _ := hs.SignedString([]byte("secret"))
	if _, err := c.verify(context.Background(), m, raw); err == nil {
		t.Fatal("algorithm confusion accepted")
	}
}

func TestAuthorizationPKCEAndExchange(t *testing.T) {
	c, key, m := fixture(t)
	ctx := context.Background()
	verifier := "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG"
	raw, err := c.AuthorizationURL(ctx, "test-state", verifier)
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(raw)
	q := parsed.Query()
	sum := sha256.Sum256([]byte(verifier))
	if q.Get("code_challenge") != base64.RawURLEncoding.EncodeToString(sum[:]) || q.Get("scope") != "" || q.Get("state") != "test-state" || strings.Contains(raw, "secret") {
		t.Fatal("unsafe authorization URL")
	}
	previous := c.http.Transport
	c.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != m.Token {
			return previous.RoundTrip(r)
		}
		id, secret, ok := r.BasicAuth()
		if !ok || id != "client-id" || secret != "secret" {
			t.Error("missing confidential client authentication")
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Method != "POST" || r.Form.Get("code") != "code" || r.Form.Get("code_verifier") != verifier || r.Form.Get("redirect_uri") != c.callback {
			t.Error("invalid token request")
		}
		return jsonResponse(map[string]string{"access_token": sign(t, key, validClaims()), "refresh_token": "must-not-leave-client", "token_type": "Bearer"}), nil
	})
	if character, err := c.Exchange(ctx, "code", verifier); err != nil || character.Name != "Pilot" {
		t.Fatalf("exchange: %v %v", character, err)
	}
}

func TestDiscoveryRejectsUntrustedEndpoints(t *testing.T) {
	c, _, m := fixture(t)
	m.Token = "https://evil.example/token"
	c.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) { return jsonResponse(m), nil })
	if _, err := c.AuthorizationURL(context.Background(), "state", "verifier"); err == nil {
		t.Fatal("untrusted token endpoint accepted")
	}
}

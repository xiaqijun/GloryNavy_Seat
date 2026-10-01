package eve

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
)

func TestSeATDefaultScopesAndJWTGrantValidation(t *testing.T) {
	c, key, m := fixture(t)
	c.EnableSeatDefaultScopes()
	requested := c.RequestedScopes()
	if len(requested) != 57 {
		t.Fatalf("unexpected scope count: %d", len(requested))
	}
	seen := map[string]bool{}
	for _, scope := range requested {
		if seen[scope] || !strings.HasPrefix(scope, "esi-") {
			t.Fatal("duplicate or invalid scope")
		}
		seen[scope] = true
	}
	if seen["publicData"] || seen["esi-characters.read_chat_channels.v1"] {
		t.Fatal("legacy scope requested")
	}
	for _, scope := range []string{CorporationRolesScope, "esi-corporations.read_corporation_membership.v1", "esi-wallet.read_character_wallet.v1", "esi-corporations.read_projects.v1", "esi-ui.open_window.v1"} {
		if !seen[scope] {
			t.Fatalf("missing SeAT scope %s", scope)
		}
	}
	raw, err := c.AuthorizationURL(context.Background(), "state", "verifier")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(raw)
	if !slices.Equal(strings.Fields(u.Query().Get("scope")), requested) {
		t.Fatal("authorization request does not match profile")
	}
	granted := requested
	original := c.http.Transport
	c.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != m.Token {
			return original.RoundTrip(r)
		}
		cl := validClaims()
		cl.Scopes = granted
		return jsonResponse(tokenResponse{sign(t, key, cl), "refresh", "Bearer"}), nil
	})
	if _, err = c.Exchange(context.Background(), "code", "verifier"); err != nil {
		t.Fatal("full consent rejected", err)
	}
	granted = []string{CorporationRolesScope}
	if _, err = c.Exchange(context.Background(), "code", "verifier"); !errors.Is(err, ErrReauthorize) {
		t.Fatal("partial consent accepted")
	}
	// A pre-upgrade role-only credential may still refresh its original grant.
	// A refresh is not allowed to silently add scopes from a newer login profile.
	ch, err := c.refresh(context.Background(), "old-refresh")
	if err != nil || !slices.Equal(ch.authorization.Scopes, granted) {
		t.Fatal("legacy refresh broken or scope fabricated")
	}
	requested[0] = "changed"
	if c.RequestedScopes()[0] == "changed" {
		t.Fatal("profile is mutable through returned slice")
	}
}

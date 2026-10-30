package eve

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/module"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/testutil"
)

func TestAllScopesCatalog(t *testing.T) {
	client := NewClient("id", "secret", "https://example.com/callback")
	client.EnableAllESIScopes()
	scopes := client.RequestedScopes()
	if len(scopes) != 72 || !slices.IsSorted(scopes) || !hasScopes(scopes, []string{CorporationRolesScope, FleetWriteScope, MailSendScope, "esi-characters.write_contacts.v1", "esi.activity.char:read", "esi.cosmetic.char:read"}) {
		t.Fatalf("unexpected ESI scope catalog: %v", scopes)
	}
	for i := 1; i < len(scopes); i++ {
		if scopes[i] == scopes[i-1] {
			t.Fatalf("duplicate scope: %s", scopes[i])
		}
	}
	scopes[0] = "modified"
	if client.RequestedScopes()[0] == "modified" || AllESIScopes()[0] == "modified" {
		t.Fatal("scope catalog was mutable")
	}
}

func TestExchangeUsesOriginalFlowScopes(t *testing.T) {
	c, key, m := fixture(t)
	c.EnableAllESIScopes()
	previous := c.http.Transport
	c.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != m.Token {
			return previous.RoundTrip(r)
		}
		cl := validClaims()
		cl.Scopes = []string{CorporationRolesScope}
		return jsonResponse(tokenResponse{sign(t, key, cl), "refresh", "Bearer"}), nil
	})
	if _, err := c.ExchangeForScopes(context.Background(), "code", "verifier", []string{CorporationRolesScope}); err != nil {
		t.Fatalf("old flow rejected: %v", err)
	}
	if _, err := c.Exchange(context.Background(), "code", "verifier"); !errors.Is(err, ErrReauthorize) {
		t.Fatalf("new flow accepted incomplete grant: %v", err)
	}
	if _, err := c.ExchangeForScopes(context.Background(), "code", "verifier", []string{CorporationRolesScope, MailSendScope}); !errors.Is(err, ErrReauthorize) {
		t.Fatalf("old flow accepted missing scope: %v", err)
	}
}

// A normal login contains mail and fleet scopes, but is not a privileged mail setup.
func TestAllScopesOrdinaryLogin(t *testing.T) {
	pool := testutil.Database(t)
	sso := &fakeSSO{required: AllESIScopes(), granted: AllESIScopes()}
	accounts := identity.New(pool)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewHandler(pool, sso, func(ctx context.Context, c Character, previous string) (string, error) {
		return accounts.SignIn(ctx, c.ID, c.Name, c.Owner, previous)
	}, "https://example.com", true, logger)
	auth := identity.Handler{Service: accounts, Origin: "https://example.com", Secure: true}
	reg, err := module.New([]module.Definition{h.Module(), auth.Module()}, []string{"eve", "identity"}, auth.Authorize)
	if err != nil {
		t.Fatal(err)
	}
	router := httpapi.New(logger, reg, http.NotFoundHandler())
	start := httptest.NewRequest("POST", "/api/v1/eve/login", nil)
	start.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, start)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("start: %d", w.Code)
	}
	u, err := url.Parse(w.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	callback := httptest.NewRequest("GET", "/api/v1/eve/callback?code=test&state="+u.Query().Get("state"), nil)
	callback.AddCookie(w.Result().Cookies()[0])
	done := httptest.NewRecorder()
	router.ServeHTTP(done, callback)
	if done.Code != http.StatusSeeOther || done.Header().Get("Location") != "/account" {
		t.Fatalf("ordinary login: %d %s", done.Code, done.Header().Get("Location"))
	}
}

func TestOldLoginFlowPreservesGrantThroughIdentity(t *testing.T) {
	ctx := context.Background()
	pool := testutil.Database(t)
	accounts := identity.New(pool)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc, err := NewAuthorization(pool, nil, base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{4}, 32)), logger)
	if err != nil {
		t.Fatal(err)
	}
	old := Character{ID: 123, Name: "Pilot", Owner: "owner", authorization: &authorization{RefreshToken: "old", Scopes: []string{CorporationRolesScope, MailSendScope, FleetWriteScope}}}
	if err = svc.Save(ctx, old); err != nil {
		t.Fatal(err)
	}
	sso := &fakeSSO{required: []string{CorporationRolesScope}, granted: []string{CorporationRolesScope}}
	h := NewHandler(pool, sso, nil, "https://example.com", true, logger)
	h.Complete = func(ctx context.Context, ch Character, previous string, intent identity.LoginIntent) (string, error) {
		return accounts.Complete(ctx, ch.ID, ch.Name, ch.Owner, previous, intent, func(ctx context.Context, tx pgx.Tx) error {
			return svc.SaveTx(PreserveExistingScopes(ctx), tx, ch)
		})
	}
	auth := identity.Handler{Service: accounts, Origin: "https://example.com", Secure: true}
	reg, err := module.New([]module.Definition{h.Module(), auth.Module()}, []string{"eve", "identity"}, auth.Authorize)
	if err != nil {
		t.Fatal(err)
	}
	router := httpapi.New(logger, reg, http.NotFoundHandler())
	request := httptest.NewRequest("POST", "/api/v1/eve/login", nil)
	request.Header.Set("Origin", "https://example.com")
	start := httptest.NewRecorder()
	router.ServeHTTP(start, request)
	if start.Code != http.StatusSeeOther {
		t.Fatal(start.Code)
	}
	u, err := url.Parse(start.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	sso.required = AllESIScopes() // upgrade while the user is at CCP
	callback := httptest.NewRequest("GET", "/api/v1/eve/callback?code=test&state="+u.Query().Get("state"), nil)
	callback.AddCookie(start.Result().Cookies()[0])
	done := httptest.NewRecorder()
	router.ServeHTTP(done, callback)
	if done.Header().Get("Location") != "/account" {
		t.Fatalf("callback %d %s", done.Code, done.Header().Get("Location"))
	}
	var scopes []string
	if err = pool.QueryRow(ctx, `SELECT scopes FROM eve_credentials WHERE character_id=123`).Scan(&scopes); err != nil {
		t.Fatal(err)
	}
	if !hasScopes(scopes, []string{MailSendScope, FleetWriteScope}) {
		t.Fatalf("login narrowed existing grant: %v", scopes)
	}
}

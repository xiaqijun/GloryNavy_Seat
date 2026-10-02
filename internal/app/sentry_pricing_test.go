package app

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/testutil"
)

func TestSentryAlertPricingAuthorizationCSRFAndPersistence(t *testing.T) {
	pool := testutil.Database(t)
	ctx := context.Background()
	accounts := identity.New(pool)
	adminToken, err := accounts.SignIn(ctx, 92101, "Pricing Admin", "pricing-admin", "")
	if err != nil {
		t.Fatal(err)
	}
	adminSession, err := accounts.Session(ctx, adminToken)
	if err != nil {
		t.Fatal(err)
	}
	memberToken, err := accounts.SignIn(ctx, 92102, "Pricing Member", "pricing-member", "")
	if err != nil {
		t.Fatal(err)
	}
	memberSession, err := accounts.Session(ctx, memberToken)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO access_administrators(user_id) VALUES($1)`, adminSession.UserID); err != nil {
		t.Fatal(err)
	}
	h, err := New(pool, slog.New(slog.NewTextHandler(io.Discard, nil)), "test", []string{"system", "identity", "eve", "access", "sentry"}, AuthConfig{Origin: "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}

	call := func(method, token, csrf, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/api/v1/sentry/alert-pricing", strings.NewReader(body))
		r.Header.Set("Origin", "https://example.com")
		if csrf != "" {
			r.Header.Set("X-CSRF-Token", csrf)
		}
		if token != "" {
			r.AddCookie(&http.Cookie{Name: httpapi.SessionCookie, Value: token})
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}

	if got := call(http.MethodGet, "", "", "").Code; got != http.StatusUnauthorized {
		t.Fatalf("anonymous GET status = %d", got)
	}
	memberRead := call(http.MethodGet, memberToken, "", "")
	if memberRead.Code != http.StatusOK {
		t.Fatalf("member GET status = %d, body=%s", memberRead.Code, memberRead.Body.String())
	}
	var memberEnvelope struct {
		Data struct {
			CanEdit bool `json:"can_edit"`
		} `json:"data"`
	}
	if err := json.Unmarshal(memberRead.Body.Bytes(), &memberEnvelope); err != nil {
		t.Fatal(err)
	}
	if memberEnvelope.Data.CanEdit {
		t.Fatal("member pricing response grants edit access")
	}

	body := `{"price_version":"prod-2026-10","unit_seconds":60,"unit_price_minor":25,"max_grant_seconds":7200,"grant_ttl_seconds":7200,"version":0}`
	if got := call(http.MethodPut, memberToken, memberSession.CSRFToken, body).Code; got != http.StatusForbidden {
		t.Fatalf("member PUT status = %d", got)
	}
	if got := call(http.MethodPut, adminToken, "", body).Code; got != http.StatusForbidden {
		t.Fatalf("admin PUT without CSRF status = %d", got)
	}
	adminWrite := call(http.MethodPut, adminToken, adminSession.CSRFToken, body)
	if adminWrite.Code != http.StatusOK {
		t.Fatalf("admin PUT status = %d, body=%s", adminWrite.Code, adminWrite.Body.String())
	}
	adminRead := call(http.MethodGet, adminToken, "", "")
	if adminRead.Code != http.StatusOK {
		t.Fatalf("admin GET status = %d", adminRead.Code)
	}
	var adminEnvelope struct {
		Data struct {
			CanEdit      bool   `json:"can_edit"`
			Configured   bool   `json:"configured"`
			PriceVersion string `json:"price_version"`
			UnitPrice    int64  `json:"unit_price_minor"`
			Version      int64  `json:"version"`
		} `json:"data"`
	}
	if err := json.Unmarshal(adminRead.Body.Bytes(), &adminEnvelope); err != nil {
		t.Fatal(err)
	}
	if !adminEnvelope.Data.CanEdit || !adminEnvelope.Data.Configured || adminEnvelope.Data.PriceVersion != "prod-2026-10" || adminEnvelope.Data.UnitPrice != 25 || adminEnvelope.Data.Version != 1 {
		t.Fatalf("admin pricing response = %#v", adminEnvelope.Data)
	}
}

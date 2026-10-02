package sentry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPRemoteCreateSendsHashedSecretOnly(t *testing.T) {
	var got ProvisionRequest
	var gotAuth, gotIdempotency string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/integrations/seat/keys" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		gotIdempotency = r.Header.Get("Idempotency-Key")
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		_, _ = w.Write([]byte(`{"key_id":"local-1","version":1,"status":"active"}`))
	}))
	defer server.Close()
	remote, err := NewHTTPRemote(server.URL, strings.Repeat("x", 32))
	if err != nil {
		t.Fatalf("NewHTTPRemote() error = %v", err)
	}
	// The integration URL still has to be HTTPS; use the test server's TLS
	// client explicitly after constructing the validated remote.
	remote = &HTTPRemote{BaseURL: server.URL, Token: strings.Repeat("x", 32), Client: server.Client()}
	in := ProvisionRequest{OperationID: "550e8400-e29b-41d4-a716-446655440000", KeyID: "local-1", AccountID: "account-1", Name: "desktop", Prefix: "eve_abcd", Hash: strings.Repeat("a", 64), Permissions: []string{"monitor"}, Version: 1}
	out, err := remote.Create(context.Background(), in)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if out.ID != "local-1" || gotAuth != "Bearer "+strings.Repeat("x", 32) || gotIdempotency != in.OperationID {
		t.Fatalf("unexpected response/headers: out=%+v auth=%q idempotency=%q", out, gotAuth, gotIdempotency)
	}
	if got.Hash == "" || strings.Contains(got.Hash, "eve_") {
		t.Fatalf("remote request must contain digest, got %q", got.Hash)
	}
}

func TestNewHTTPRemoteRejectsUnsafeOrigin(t *testing.T) {
	for _, raw := range []string{"http://example.com", "https://example.com/path", "https://example.com?x=1"} {
		if _, err := NewHTTPRemote(raw, strings.Repeat("x", 32)); err == nil {
			t.Errorf("NewHTTPRemote(%q) accepted unsafe origin", raw)
		}
	}
}

func TestHTTPRemoteCreateAlertGrantUsesTimeSnapshot(t *testing.T) {
	wantExpiry := time.Date(2099, time.January, 1, 0, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/integrations/seat/alert-grants" || r.Method != http.MethodPost {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Idempotency-Key"); got != "op-1" {
			t.Fatalf("idempotency key = %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		for key, want := range map[string]any{"unit_seconds": 60.0, "unit_price_minor": 7.0, "reserved_seconds": 3600.0, "expires_at": wantExpiry.Format(time.RFC3339)} {
			if body[key] != want {
				t.Fatalf("%s = %#v, want %#v", key, body[key], want)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"grant_id": "grant-1", "operation_id": "op-1", "account_id": "account-1", "price_version": "price-v1", "unit_seconds": 60, "unit_price_minor": 7, "reserved_seconds": 3600, "remaining_seconds": 3600, "expires_at": wantExpiry.Format(time.RFC3339), "status": "active", "protocol_version": 1})
	}))
	defer server.Close()
	r := &HTTPRemote{BaseURL: server.URL, Token: strings.Repeat("x", 32), Client: server.Client()}
	got, err := r.CreateAlertGrant(context.Background(), AlertGrantRequest{OperationID: "op-1", GrantID: "grant-1", AccountID: "account-1", PriceVersion: "price-v1", UnitSeconds: 60, UnitPriceMinor: 7, ReservedSeconds: 3600, ExpiresAt: wantExpiry})
	if err != nil {
		t.Fatal(err)
	}
	if got.GrantID != "grant-1" || got.RemainingSeconds != 3600 {
		t.Fatalf("grant = %#v", got)
	}
}

func TestHTTPRemoteCreateAlertGrantV2OmitsPricing(t *testing.T) {
	wantExpiry := time.Date(2099, time.January, 1, 0, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		for key, want := range map[string]any{
			"operation_id":     "op-2",
			"grant_id":         "grant-2",
			"account_id":       "account-2",
			"key_id":           "key-2",
			"reserved_seconds": 3600.0,
			"expires_at":       wantExpiry.Format(time.RFC3339),
			"protocol_version": 2.0,
		} {
			if body[key] != want {
				t.Fatalf("%s = %#v, want %#v", key, body[key], want)
			}
		}
		for _, field := range []string{"price_version", "unit_seconds", "unit_price_minor"} {
			if _, ok := body[field]; ok {
				t.Fatalf("v2 request leaked pricing field %q: %#v", field, body)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"grant_id": "grant-2", "operation_id": "op-2", "account_id": "account-2", "key_id": "key-2", "reserved_seconds": 3600, "remaining_seconds": 3600, "expires_at": wantExpiry.Format(time.RFC3339), "status": "active", "protocol_version": 2})
	}))
	defer server.Close()
	r := &HTTPRemote{BaseURL: server.URL, Token: strings.Repeat("x", 32), Client: server.Client()}
	got, err := r.CreateAlertGrant(context.Background(), AlertGrantRequest{OperationID: "op-2", GrantID: "grant-2", AccountID: "account-2", KeyID: "key-2", PriceVersion: "seat-price-v2", UnitSeconds: 60, UnitPriceMinor: 7, ReservedSeconds: 3600, ExpiresAt: wantExpiry, ProtocolVersion: 2})
	if err != nil {
		t.Fatal(err)
	}
	if got.ProtocolVersion != 2 || got.RemainingSeconds != 3600 {
		t.Fatalf("grant = %#v", got)
	}
}

func TestHTTPRemoteListAlertDeliveries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/integrations/seat/alert-deliveries" || r.URL.Query().Get("after") != "cursor-1" || r.URL.Query().Get("limit") != "20" {
			t.Fatalf("request = %s", r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"deliveries": []any{map[string]any{"delivery_id": "d-1", "charge_event_id": "e-1", "revision": 1, "grant_id": "g-1", "account_id": "a-1", "key_id": "k-1", "started_at": "2026-10-01T00:00:00Z", "ended_at": "2026-10-01T00:00:30Z", "duration_seconds": 30, "consumption_state": "consumed", "status": "confirmed"}}, "protocol_version": 1})
	}))
	defer server.Close()
	r := &HTTPRemote{BaseURL: server.URL, Token: strings.Repeat("x", 32), Client: server.Client()}
	page, err := r.ListAlertDeliveries(context.Background(), "cursor-1", 20)
	if err != nil || len(page.Deliveries) != 1 || page.Deliveries[0].ConsumptionState != "consumed" {
		t.Fatalf("page=%+v err=%v", page, err)
	}
}

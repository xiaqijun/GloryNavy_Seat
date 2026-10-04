package sentry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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

func TestHTTPRemoteSetAlertConsumptionEnabled(t *testing.T) {
	var gotAuth, gotIdempotency string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/integrations/seat/alert-consumption" || r.Method != http.MethodPut {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		gotIdempotency = r.Header.Get("Idempotency-Key")
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["enabled"] != true {
			t.Fatalf("enabled payload = %#v", body["enabled"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"enabled": true})
	}))
	defer server.Close()
	token := strings.Repeat("x", 32)
	r := &HTTPRemote{BaseURL: server.URL, Token: token, Client: server.Client()}
	if err := r.SetAlertConsumptionEnabled(context.Background(), true, "operation-1"); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer "+token || gotIdempotency != "operation-1" {
		t.Fatalf("headers auth=%q idempotency=%q", gotAuth, gotIdempotency)
	}
}

func TestHTTPRemoteListMonitorContributions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/integrations/seat/monitor-contributions" || r.URL.Query().Get("after") != "cursor-1" || r.URL.Query().Get("limit") != "20" {
			t.Fatalf("request = %s", r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"contributions": []any{map[string]any{
			"contribution_id": "c-1", "account_id": "a-1", "key_id": "k-1", "client_id": "client-1", "system_name": "Jita", "primary_generation": 2,
			"started_at": "2026-10-01T00:00:00Z", "ended_at": "2026-10-01T00:00:30Z", "duration_seconds": 30, "eligibility": "eligible", "evidence": map[string]any{"server_confirmed": true}, "created_at": "2026-10-01T00:00:31Z",
		}}, "next_cursor": "cursor-2", "protocol_version": 1, "rule_version": "primary-presence.v1"})
	}))
	defer server.Close()
	r := &HTTPRemote{BaseURL: server.URL, Token: strings.Repeat("x", 32), Client: server.Client()}
	page, err := r.ListMonitorContributions(context.Background(), "cursor-1", 20)
	if err != nil || len(page.Contributions) != 1 || page.Contributions[0].DurationSeconds != 30 || page.Contributions[0].Eligibility != "eligible" {
		t.Fatalf("page=%+v err=%v", page, err)
	}
}

func TestHTTPRemoteListClientUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/integrations/seat/client-usage" || r.URL.Query().Get("after") != "cursor-1" || r.URL.Query().Get("limit") != "20" {
			t.Fatalf("request = %s", r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"usage": []any{map[string]any{
			"usage_id": "u-1", "account_id": "a-1", "key_id": "k-1", "client_id": "client-1",
			"started_at": "2026-10-01T00:00:00Z", "ended_at": "2026-10-01T00:00:30Z", "duration_seconds": 30, "created_at": "2026-10-01T00:00:31Z",
		}}, "next_cursor": "cursor-2", "protocol_version": 1, "rule_version": "client-heartbeat.v1"})
	}))
	defer server.Close()
	r := &HTTPRemote{BaseURL: server.URL, Token: strings.Repeat("x", 32), Client: server.Client()}
	page, err := r.ListClientUsage(context.Background(), "cursor-1", 20)
	if err != nil || len(page.Usage) != 1 || page.Usage[0].DurationSeconds != 30 || page.Usage[0].AccountID != "a-1" {
		t.Fatalf("page=%+v err=%v", page, err)
	}
}

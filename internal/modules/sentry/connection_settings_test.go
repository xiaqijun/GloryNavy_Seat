package sentry

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCheckServiceConnectionUsesReadOnlyAuthenticatedRequest(t *testing.T) {
	token := strings.Repeat("s", 64)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/integrations/seat/alert-consumption" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`{"protocol_version":1,"enabled":false}`))
	}))
	defer server.Close()

	fallback, err := NewHTTPRemote(server.URL, token)
	if err != nil {
		t.Fatal(err)
	}
	remote, err := newWebConnection(server.URL, token, fallback)
	if err != nil || checkServiceConnection(context.Background(), remote) != nil {
		t.Fatalf("existing administrator-configured origin should connect: %v", err)
	}
	wrong, err := newWebConnection(server.URL, strings.Repeat("n", 64), fallback)
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(checkServiceConnection(context.Background(), wrong), errConnectionCheck) {
		t.Fatal("expected generic error for incorrect token")
	}
}

func TestWebConnectionRequiresPublicHTTPSUnlessOriginIsFromEnvironment(t *testing.T) {
	token := strings.Repeat("s", 64)
	for _, origin := range []string{"http://sentry.example.org", "https://127.0.0.1", "https://100.64.0.2", "https://[::1]", "https://sentry.example.org/other", "https://sentry.example.org?"} {
		if _, err := newWebConnection(origin, token, nil); !errors.Is(err, errConnectionInvalid) {
			t.Errorf("%s accepted: %v", origin, err)
		}
	}
	if r, err := newWebConnection("https://sentry.example.org", token, nil); err != nil || r.Client.Transport == nil {
		t.Fatalf("valid HTTPS origin missing guarded transport: %v", err)
	}
}

func TestServiceConnectionTokenIsEncryptedAndAbsentFromResponse(t *testing.T) {
	service := New(nil, nil)
	if err := service.SetSecretKey(base64.StdEncoding.EncodeToString(make([]byte, 32))); err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("integration-secret", 3)
	scope := "service-connection:https://sentry.example.org"
	sealed, err := service.sealSecret(scope, token)
	if err != nil || strings.Contains(string(sealed), token) {
		t.Fatalf("token encryption failed: %v", err)
	}
	if opened, err := service.openSecret(scope, sealed); err != nil || opened != token {
		t.Fatal("cannot recover the service token")
	}
	if _, err := service.openSecret(scope+"-other", sealed); err == nil {
		t.Fatal("ciphertext accepted for a different origin")
	}
	payload, err := json.Marshal(ConnectionSettings{Origin: "https://sentry.example.org", TokenConfigured: true})
	if err != nil || strings.Contains(string(payload), token) || strings.Contains(string(payload), "ciphertext") {
		t.Fatalf("invalid settings response: %s", payload)
	}
	if _, err := service.ReadConnection(context.Background(), "member"); !errors.Is(err, errConnectionPermission) {
		t.Fatal("expected administrator check")
	}
}

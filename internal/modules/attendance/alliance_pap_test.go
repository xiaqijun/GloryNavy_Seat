package attendance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
)

func TestParseAlliancePAPTokenPrefersReadToken(t *testing.T) {
	token, err := parseAlliancePAPToken([]byte(`{"api_token":"api","read_token":"read"}`))
	if err != nil || token != "read" {
		t.Fatalf("token=%q err=%v, want read token", token, err)
	}
}

func TestParseAlliancePAPTokenAcceptsServiceAPIToken(t *testing.T) {
	token, err := parseAlliancePAPToken([]byte(`{"api_token":"api"}`))
	if err != nil || token != "api" {
		t.Fatalf("token=%q err=%v, want api token fallback", token, err)
	}
}

func TestParseAlliancePAPTokenRejectsMissingToken(t *testing.T) {
	if _, err := parseAlliancePAPToken([]byte(`{"session_secret":"secret"}`)); err == nil {
		t.Fatal("missing token must be rejected")
	}
}

func TestFetchAlliancePAPRetriesTransientUpstream(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			http.Error(w, "temporary", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"source":"winterco","year":2026,"month":9,"complete":true,"records_total":0,"rows":[]}`))
	}))
	defer server.Close()
	auth := t.TempDir() + "/auth.json"
	if err := os.WriteFile(auth, []byte(`{"api_token":"test-token"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Service{AlliancePAP: AlliancePAPConfig{URL: server.URL, AuthFile: auth}}
	got, err := s.fetchAlliancePAP(context.Background())
	if err != nil || !got.OK || calls.Load() != 2 {
		t.Fatalf("response=%+v err=%v calls=%d", got, err, calls.Load())
	}
}

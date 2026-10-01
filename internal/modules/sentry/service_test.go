package sentry

import (
	"strings"
	"testing"
)

func TestOperationIDUsesRequestUUID(t *testing.T) {
	want := "550e8400-e29b-41d4-a716-446655440000"
	got, err := operationID(want)
	if err != nil {
		t.Fatalf("operationID() error = %v", err)
	}
	if got != want {
		t.Fatalf("operationID() = %q, want %q", got, want)
	}
}

func TestOperationIDRejectsNonUUID(t *testing.T) {
	if _, err := operationID("same-request"); err != ErrInvalid {
		t.Fatalf("operationID() error = %v, want ErrInvalid", err)
	}
}

func TestNewSecretHasPrefixAndDigestShape(t *testing.T) {
	secret, digest, prefix, err := newSecret()
	if err != nil {
		t.Fatalf("newSecret() error = %v", err)
	}
	if !strings.HasPrefix(secret, "eve_") || len(digest) != 32 || prefix != secret[:12] {
		t.Fatalf("secret metadata invalid: secret prefix=%q digest=%d prefix=%q", secret[:4], len(digest), prefix)
	}
}

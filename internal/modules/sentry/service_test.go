package sentry

import (
	"strings"
	"testing"
	"time"
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

func TestPolicyValid(t *testing.T) {
	valid := AlertGrantPolicy{
		PriceVersion:    "prod-2026-10",
		UnitSeconds:     60,
		UnitPriceMinor:  25,
		MaxGrantSeconds: 86400,
		GrantTTL:        24 * time.Hour,
	}
	if !policyValid(valid) {
		t.Fatal("policyValid() rejected a valid policy")
	}

	cases := []struct {
		name   string
		change func(*AlertGrantPolicy)
	}{
		{"missing version", func(p *AlertGrantPolicy) { p.PriceVersion = "" }},
		{"newline version", func(p *AlertGrantPolicy) { p.PriceVersion = "prod\n2026" }},
		{"unit too large", func(p *AlertGrantPolicy) { p.UnitSeconds = 86401 }},
		{"zero price", func(p *AlertGrantPolicy) { p.UnitPriceMinor = 0 }},
		{"grant cap too large", func(p *AlertGrantPolicy) { p.MaxGrantSeconds = 2678401 }},
		{"ttl too short", func(p *AlertGrantPolicy) { p.GrantTTL = 59 * time.Second }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			candidate := valid
			tc.change(&candidate)
			if policyValid(candidate) {
				t.Fatal("policyValid() accepted an invalid policy")
			}
		})
	}
}

func TestPolicyFromEditTrimsVersionAndConvertsTTL(t *testing.T) {
	policy := policyFromEdit(AlertPricingEdit{
		PriceVersion:    "  prod-2026-10  ",
		UnitSeconds:     60,
		UnitPriceMinor:  25,
		MaxGrantSeconds: 86400,
		GrantTTLSeconds: 3600,
	})
	if policy.PriceVersion != "prod-2026-10" || policy.GrantTTL != time.Hour {
		t.Fatalf("policyFromEdit() = %#v", policy)
	}
}

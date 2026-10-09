package config

import (
	"testing"
	"time"
)

func TestSlowQueryConfiguration(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://dev@localhost/test")
	t.Setenv("MODULES", "system")
	t.Setenv("EVE_CLIENT_ID", "")
	t.Setenv("EVE_CLIENT_SECRET", "")
	t.Setenv("EVE_TOKEN_KEY", "")
	for _, tc := range []struct {
		value   string
		want    time.Duration
		invalid bool
	}{
		{"", 500 * time.Millisecond, false}, {"0", 0, false}, {"1000", time.Second, false},
		{"-1", 0, true}, {"60001", 0, true}, {"abc", 0, true},
	} {
		t.Run(tc.value, func(t *testing.T) {
			t.Setenv("DB_SLOW_QUERY_MS", tc.value)
			cfg, err := Load()
			if tc.invalid {
				if err == nil {
					t.Fatal("invalid threshold accepted")
				}
				return
			}
			if err != nil || cfg.DBSlowQuery != tc.want {
				t.Fatalf("threshold=%s err=%v", cfg.DBSlowQuery, err)
			}
		})
	}
}

func TestSDEUpdateConfiguration(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://dev@localhost/test")
	t.Setenv("EVE_CLIENT_ID", "")
	t.Setenv("EVE_CLIENT_SECRET", "")
	t.Setenv("SDE_AUTO_UPDATE", "")
	t.Setenv("SDE_CHECK_INTERVAL", "")
	c, err := Load()
	if err != nil || !c.SDEAutoUpdate || c.SDECheckInterval != 6*time.Hour {
		t.Fatal(c.SDEAutoUpdate, c.SDECheckInterval, err)
	}
	t.Setenv("SDE_AUTO_UPDATE", "false")
	c, err = Load()
	if err != nil || c.SDEAutoUpdate {
		t.Fatal(err)
	}
	for _, tc := range []struct{ key, value string }{{"SDE_AUTO_UPDATE", "invalid"}, {"SDE_CHECK_INTERVAL", "0"}, {"SDE_CHECK_INTERVAL", "59m"}, {"SDE_CHECK_INTERVAL", "169h"}} {
		t.Run(tc.key+tc.value, func(t *testing.T) {
			t.Setenv(tc.key, tc.value)
			if _, err := Load(); err == nil {
				t.Fatal("invalid SDE config accepted")
			}
		})
	}
}

func TestEncryptedCredentialConfiguration(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://dev@localhost/test")
	t.Setenv("PUBLIC_ORIGIN", "http://127.0.0.1:5173")
	t.Setenv("MODULES", "system,identity,eve,access")
	t.Setenv("EVE_CLIENT_ID", "client")
	t.Setenv("EVE_CLIENT_SECRET", "secret")
	for _, key := range []string{"", "invalid", "c2hvcnQ="} {
		t.Setenv("EVE_TOKEN_KEY", key)
		if _, err := Load(); err == nil {
			t.Fatal("unencrypted authorization configuration accepted")
		}
	}
	t.Setenv("EVE_TOKEN_KEY", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
}

func TestRejectUnsupportedEnvironmentAndInvalidLimits(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://dev@localhost/test")
	t.Setenv("EVE_ENVIRONMENT", "tranquility")
	t.Setenv("HTTP_ADDR", "127.0.0.1:8080")
	t.Setenv("LOG_LEVEL", "info")
	t.Setenv("DB_MAX_CONNS", "10")
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ key, value string }{{"EVE_ENVIRONMENT", "serenity"}, {"DB_MAX_CONNS", "0"}, {"DB_MAX_CONNS", "101"}, {"DB_MAX_CONNS", "2147483648"}, {"HTTP_ADDR", "invalid"}, {"LOG_LEVEL", "invalid"}, {"DATABASE_URL", ""}} {
		t.Run(tc.key+tc.value, func(t *testing.T) {
			t.Setenv(tc.key, tc.value)
			if _, err := Load(); err == nil {
				t.Error("expected invalid config to fail")
			}
		})
	}
}

func TestLoginConfiguration(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://dev@localhost/test")
	t.Setenv("EVE_CLIENT_ID", "")
	t.Setenv("EVE_CLIENT_SECRET", "")
	for _, origin := range []string{"http://127.0.0.1:5173", "https://seat.example.com"} {
		t.Setenv("PUBLIC_ORIGIN", origin)
		if _, err := Load(); err != nil {
			t.Fatal(err)
		}
	}
	for _, origin := range []string{"http://seat.example.com", "https://example.com/", "https://user:pass@example.com", "https://example.com?redirect=evil", "file:///tmp"} {
		t.Setenv("PUBLIC_ORIGIN", origin)
		if _, err := Load(); err == nil {
			t.Fatalf("unsafe origin accepted: %s", origin)
		}
	}
	t.Setenv("PUBLIC_ORIGIN", "https://seat.example.com")
	t.Setenv("EVE_CLIENT_ID", "configured")
	if _, err := Load(); err == nil {
		t.Fatal("partial credentials accepted")
	}
}

func TestQQBotGroupOpenIDsRejectNumericQQGroupNumbers(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://dev@localhost/test")
	t.Setenv("EVE_CLIENT_ID", "")
	t.Setenv("EVE_CLIENT_SECRET", "")
	t.Setenv("QQ_BOT_GROUP_OPENIDS", "123456789")
	if _, err := Load(); err == nil {
		t.Fatal("numeric QQ group number accepted as Group OpenID")
	}
}

func TestAlertConsumptionConfigurationRequiresRuntimeModules(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://dev@localhost/test")
	t.Setenv("EVE_CLIENT_ID", "")
	t.Setenv("EVE_CLIENT_SECRET", "")
	t.Setenv("SENTRY_INTEGRATION_URL", "https://sentry.example.com")
	t.Setenv("SENTRY_INTEGRATION_TOKEN", "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx")
	t.Setenv("SENTRY_ALERT_CONSUMPTION_ENABLED", "true")
	t.Setenv("SENTRY_ALERT_PRICE_VERSION", "alert-v1")
	t.Setenv("SENTRY_ALERT_UNIT_SECONDS", "60")
	t.Setenv("SENTRY_ALERT_UNIT_PRICE_MINOR", "7")
	t.Setenv("SENTRY_ALERT_MAX_GRANT_SECONDS", "3600")
	t.Setenv("SENTRY_ALERT_GRANT_TTL", "1h")
	t.Setenv("MODULES", "system,identity,sentry")
	if _, err := Load(); err == nil {
		t.Fatal("alert consumption accepted without exchange and eve modules")
	}
	t.Setenv("MODULES", "system,identity,eve,exchange,sentry")
	c, err := Load()
	if err != nil || !c.AlertConsumptionEnabled {
		t.Fatalf("alert consumption config = %#v, err=%v", c, err)
	}
}

func TestAlertConsumptionConfigurationRequiresFrozenPolicy(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://dev@localhost/test")
	t.Setenv("EVE_CLIENT_ID", "")
	t.Setenv("EVE_CLIENT_SECRET", "")
	t.Setenv("SENTRY_INTEGRATION_URL", "https://sentry.example.com")
	t.Setenv("SENTRY_INTEGRATION_TOKEN", "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx")
	t.Setenv("SENTRY_ALERT_CONSUMPTION_ENABLED", "true")
	t.Setenv("MODULES", "system,identity,eve,exchange,sentry")
	for _, key := range []string{
		"SENTRY_ALERT_PRICE_VERSION",
		"SENTRY_ALERT_UNIT_SECONDS",
		"SENTRY_ALERT_UNIT_PRICE_MINOR",
		"SENTRY_ALERT_MAX_GRANT_SECONDS",
		"SENTRY_ALERT_GRANT_TTL",
	} {
		t.Run(key, func(t *testing.T) {
			t.Setenv("SENTRY_ALERT_PRICE_VERSION", "alert-v1")
			t.Setenv("SENTRY_ALERT_UNIT_SECONDS", "60")
			t.Setenv("SENTRY_ALERT_UNIT_PRICE_MINOR", "7")
			t.Setenv("SENTRY_ALERT_MAX_GRANT_SECONDS", "3600")
			t.Setenv("SENTRY_ALERT_GRANT_TTL", "1h")
			t.Setenv(key, "")
			if _, err := Load(); err == nil {
				t.Fatalf("alert consumption accepted without %s", key)
			}
		})
	}
}

func TestApprovalIndexAccountAllowlist(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://dev@localhost/test")
	t.Setenv("EVE_CLIENT_ID", "")
	t.Setenv("EVE_CLIENT_SECRET", "")
	t.Setenv("APPROVAL_INDEX_ACCOUNTS", "11111111-1111-1111-1111-111111111111, 11111111-1111-1111-1111-111111111111")
	c, err := Load()
	if err != nil || len(c.ApprovalIndexAccounts) != 1 {
		t.Fatalf("allowlist=%v err=%v", c.ApprovalIndexAccounts, err)
	}
	for _, value := range []string{"not-a-uuid", "11111111-1111-1111-1111-11111111111z"} {
		t.Setenv("APPROVAL_INDEX_ACCOUNTS", value)
		if _, err := Load(); err == nil {
			t.Fatalf("invalid approval account accepted: %s", value)
		}
	}
}

package config

import (
	"encoding/base64"
	"errors"
	"log/slog"
	"net"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr                string
	DatabaseURL             string
	DBMaxConns              int32
	DBSlowQuery             time.Duration
	Environment             string
	LogLevel                slog.Level
	Modules                 []string
	PublicOrigin            string
	EVEClientID             string
	EVEClientSecret         string
	EVETokenKey             string
	ESIUserAgent            string
	SDEAutoUpdate           bool
	SDEWorkDir              string
	SDECheckInterval        time.Duration
	WinterCoPAPURL          string
	WinterCoPAPAuth         string
	QQBotAppID              string
	QQBotAppSecret          string
	QQBotAPIBase            string
	QQBotGroupOpenIDs       []string
	SentryIntegrationURL    string
	SentryIntegrationToken  string
	AlertConsumptionEnabled bool
	AlertPriceVersion       string
	AlertUnitSeconds        int64
	AlertUnitPriceMinor     int64
	AlertMaxGrantSeconds    int64
	AlertGrantTTL           time.Duration
}

func Load() (Config, error) {
	c := Config{HTTPAddr: value("HTTP_ADDR", "127.0.0.1:8080"), DatabaseURL: os.Getenv("DATABASE_URL"), Environment: value("EVE_ENVIRONMENT", "tranquility")}
	if c.DatabaseURL == "" {
		return c, errors.New("DATABASE_URL is required")
	}
	if _, _, err := net.SplitHostPort(c.HTTPAddr); err != nil {
		return c, errors.New("HTTP_ADDR must contain host:port")
	}
	if c.Environment != "tranquility" {
		return c, errors.New("only EVE_ENVIRONMENT=tranquility is supported")
	}
	if err := c.LogLevel.UnmarshalText([]byte(value("LOG_LEVEL", "info"))); err != nil {
		return c, errors.New("invalid LOG_LEVEL")
	}
	n, err := strconv.ParseInt(value("DB_MAX_CONNS", "10"), 10, 32)
	if err != nil || n < 1 || n > 100 {
		return c, errors.New("DB_MAX_CONNS must be between 1 and 100")
	}
	c.DBMaxConns = int32(n)
	slowMS, err := strconv.ParseInt(value("DB_SLOW_QUERY_MS", "500"), 10, 32)
	if err != nil || slowMS < 0 || slowMS > 60000 {
		return c, errors.New("DB_SLOW_QUERY_MS must be between 0 and 60000")
	}
	c.DBSlowQuery = time.Duration(slowMS) * time.Millisecond
	c.Modules = strings.Split(value("MODULES", "system,identity,eve,access,community,attendance,sentry"), ",")
	for i := range c.Modules {
		c.Modules[i] = strings.TrimSpace(c.Modules[i])
	}
	c.PublicOrigin = value("PUBLIC_ORIGIN", "http://127.0.0.1:5173")
	u, err := url.Parse(c.PublicOrigin)
	if err != nil || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return c, errors.New("PUBLIC_ORIGIN must be an origin without path, credentials or query")
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost" || u.Hostname() == "::1")) {
		return c, errors.New("PUBLIC_ORIGIN requires HTTPS except on loopback")
	}
	c.EVEClientID, c.EVEClientSecret = os.Getenv("EVE_CLIENT_ID"), os.Getenv("EVE_CLIENT_SECRET")
	if (c.EVEClientID == "") != (c.EVEClientSecret == "") {
		return c, errors.New("EVE_CLIENT_ID and EVE_CLIENT_SECRET must both be configured")
	}
	c.EVETokenKey = os.Getenv("EVE_TOKEN_KEY")
	c.ESIUserAgent = value("ESI_USER_AGENT", "GloryNavy/0.1.0")
	c.SDEAutoUpdate, err = strconv.ParseBool(value("SDE_AUTO_UPDATE", "true"))
	if err != nil {
		return c, errors.New("SDE_AUTO_UPDATE must be true or false")
	}
	c.SDEWorkDir = value("SDE_WORK_DIR", ".local/sde")
	c.SDECheckInterval, err = time.ParseDuration(value("SDE_CHECK_INTERVAL", "6h"))
	if err != nil || c.SDECheckInterval < time.Hour || c.SDECheckInterval > 7*24*time.Hour {
		return c, errors.New("SDE_CHECK_INTERVAL must be between 1h and 168h")
	}
	if len(c.ESIUserAgent) > 256 || strings.IndexFunc(c.ESIUserAgent, func(r rune) bool { return r < 32 || r > 126 }) >= 0 {
		return c, errors.New("ESI_USER_AGENT must contain at most 256 printable ASCII characters")
	}
	c.WinterCoPAPURL = strings.TrimRight(os.Getenv("WINTERCO_PAP_URL"), "/")
	c.WinterCoPAPAuth = os.Getenv("WINTERCO_PAP_AUTH_FILE")
	c.QQBotAppID = strings.TrimSpace(os.Getenv("QQ_BOT_APP_ID"))
	c.QQBotAppSecret = strings.TrimSpace(os.Getenv("QQ_BOT_APP_SECRET"))
	c.QQBotAPIBase = strings.TrimRight(value("QQ_BOT_API_BASE", "https://api.bot.qq.com"), "/")
	for _, group := range strings.Split(os.Getenv("QQ_BOT_GROUP_OPENIDS"), ",") {
		if group = strings.TrimSpace(group); group != "" {
			if len(group) > 256 || strings.ContainsAny(group, "\r\n") || isNumericQQGroupID(group) {
				return c, errors.New("QQ_BOT_GROUP_OPENIDS contains an invalid group OpenID")
			}
			c.QQBotGroupOpenIDs = append(c.QQBotGroupOpenIDs, group)
		}
	}
	if (c.QQBotAppID == "") != (c.QQBotAppSecret == "") {
		return c, errors.New("QQ_BOT_APP_ID and QQ_BOT_APP_SECRET must both be configured")
	}
	c.SentryIntegrationURL = strings.TrimRight(strings.TrimSpace(os.Getenv("SENTRY_INTEGRATION_URL")), "/")
	c.SentryIntegrationToken = strings.TrimSpace(os.Getenv("SENTRY_INTEGRATION_TOKEN"))
	if (c.SentryIntegrationURL == "") != (c.SentryIntegrationToken == "") {
		return c, errors.New("SENTRY_INTEGRATION_URL and SENTRY_INTEGRATION_TOKEN must both be configured")
	}
	if c.SentryIntegrationURL != "" {
		su, parseErr := url.Parse(c.SentryIntegrationURL)
		if parseErr != nil || su.Host == "" || su.Path != "" || su.RawQuery != "" || su.Fragment != "" || su.User != nil || (su.Scheme != "https" && !(su.Scheme == "http" && (su.Hostname() == "127.0.0.1" || su.Hostname() == "localhost" || su.Hostname() == "::1"))) {
			return c, errors.New("SENTRY_INTEGRATION_URL must be an HTTPS origin")
		}
		if len(c.SentryIntegrationToken) < 32 || strings.ContainsAny(c.SentryIntegrationToken, "\r\n") {
			return c, errors.New("SENTRY_INTEGRATION_TOKEN must contain at least 32 characters")
		}
	}
	c.AlertConsumptionEnabled, err = strconv.ParseBool(value("SENTRY_ALERT_CONSUMPTION_ENABLED", "false"))
	if err != nil {
		return c, errors.New("SENTRY_ALERT_CONSUMPTION_ENABLED must be true or false")
	}
	if c.AlertConsumptionEnabled && (c.SentryIntegrationURL == "" || !slices.Contains(c.Modules, "sentry") || !slices.Contains(c.Modules, "exchange") || !slices.Contains(c.Modules, "eve")) {
		return c, errors.New("SENTRY_ALERT_CONSUMPTION_ENABLED requires Sentry integration, sentry, exchange and eve modules")
	}
	if c.AlertConsumptionEnabled {
		c.AlertPriceVersion = strings.TrimSpace(os.Getenv("SENTRY_ALERT_PRICE_VERSION"))
		if c.AlertPriceVersion == "" || len(c.AlertPriceVersion) > 80 || strings.ContainsAny(c.AlertPriceVersion, "\r\n") {
			return c, errors.New("SENTRY_ALERT_PRICE_VERSION is required when alert consumption is enabled")
		}
		parsePositive := func(name string, maximum int64) (int64, error) {
			raw := strings.TrimSpace(os.Getenv(name))
			n, parseErr := strconv.ParseInt(raw, 10, 64)
			if parseErr != nil || n <= 0 || (maximum > 0 && n > maximum) {
				return 0, errors.New(name + " must be a positive integer within its allowed range")
			}
			return n, nil
		}
		if c.AlertUnitSeconds, err = parsePositive("SENTRY_ALERT_UNIT_SECONDS", 86400); err != nil {
			return c, err
		}
		if c.AlertUnitPriceMinor, err = parsePositive("SENTRY_ALERT_UNIT_PRICE_MINOR", 1000000000000); err != nil {
			return c, err
		}
		if c.AlertMaxGrantSeconds, err = parsePositive("SENTRY_ALERT_MAX_GRANT_SECONDS", 31*24*60*60); err != nil {
			return c, err
		}
		c.AlertGrantTTL, err = time.ParseDuration(strings.TrimSpace(os.Getenv("SENTRY_ALERT_GRANT_TTL")))
		if err != nil || c.AlertGrantTTL < time.Minute || c.AlertGrantTTL > 31*24*time.Hour {
			return c, errors.New("SENTRY_ALERT_GRANT_TTL must be between 1m and 744h")
		}
	}
	if api, parseErr := url.Parse(c.QQBotAPIBase); parseErr != nil || api.Host == "" || api.Path != "" || api.RawQuery != "" || api.Fragment != "" || api.User != nil || (api.Scheme != "https" && !(api.Scheme == "http" && (api.Hostname() == "127.0.0.1" || api.Hostname() == "localhost" || api.Hostname() == "::1"))) {
		return c, errors.New("QQ_BOT_API_BASE must be an HTTPS origin")
	}
	if c.WinterCoPAPURL != "" {
		u, parseErr := url.Parse(c.WinterCoPAPURL)
		if parseErr != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
			return c, errors.New("WINTERCO_PAP_URL must be an http(s) URL without credentials")
		}
		if c.WinterCoPAPAuth == "" {
			return c, errors.New("WINTERCO_PAP_AUTH_FILE is required when WINTERCO_PAP_URL is configured")
		}
	}
	if c.EVETokenKey != "" || (slices.Contains(c.Modules, "eve") && c.EVEClientID != "") {
		key, err := base64.StdEncoding.DecodeString(c.EVETokenKey)
		if err != nil || len(key) != 32 {
			return c, errors.New("EVE_TOKEN_KEY must be a base64-encoded 32-byte key")
		}
	}
	return c, nil
}

func isNumericQQGroupID(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func value(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

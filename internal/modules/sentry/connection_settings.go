package sentry

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/sentry/internal/store"
)

var (
	errConnectionPermission = errors.New("Sentry connection settings require a site administrator")
	errConnectionInvalid    = errors.New("invalid Sentry connection settings")
	errConnectionConflict   = errors.New("Sentry connection settings revision conflict")
	errConnectionCheck      = errors.New("Sentry service authentication check failed")
)

type ConnectionSettings struct {
	Origin          string     `json:"origin"`
	Revision        int64      `json:"revision"`
	TokenConfigured bool       `json:"token_configured"`
	UpdatedAt       *time.Time `json:"updated_at,omitempty"`
}

type ConnectionEdit struct {
	Origin   string `json:"origin"`
	Token    string `json:"service_token,omitempty"`
	Revision int64  `json:"revision"`
}

// managedConnection resolves the database revision for each operation,
// including those issued by another web instance or background worker.
type managedConnection struct {
	service  *Service
	fallback *HTTPRemote

	mu       sync.Mutex
	revision int64
	remote   *HTTPRemote
}

func (s *Service) ConfigureConnection(fallback *HTTPRemote) {
	connection := &managedConnection{service: s, fallback: fallback}
	s.connection = connection
	s.Remote = connection
	s.MonitorRemote = connection
	s.ClientUsageRemote = connection
}

func (s *Service) requireConnectionAdmin(ctx context.Context, user string) error {
	if s.Administrator == nil || user == "" {
		return errConnectionPermission
	}
	ok, err := s.Administrator(ctx, user)
	if err != nil {
		return err
	}
	if !ok {
		return errConnectionPermission
	}
	if s.connection == nil || s.Pool == nil {
		return ErrRemoteUnavailable
	}
	return nil
}

func (s *Service) ReadConnection(ctx context.Context, user string) (ConnectionSettings, error) {
	if err := s.requireConnectionAdmin(ctx, user); err != nil {
		return ConnectionSettings{}, err
	}
	settings, _, err := s.connection.current(ctx)
	return settings, err
}

func (s *Service) EditConnection(ctx context.Context, user string, input ConnectionEdit) (ConnectionSettings, error) {
	if err := s.requireConnectionAdmin(ctx, user); err != nil {
		return ConnectionSettings{}, err
	}
	current, existing, err := s.connection.current(ctx)
	if err != nil {
		return ConnectionSettings{}, err
	}
	if input.Revision != current.Revision {
		return ConnectionSettings{}, errConnectionConflict
	}
	origin := strings.TrimRight(strings.TrimSpace(input.Origin), "/")
	token := strings.TrimSpace(input.Token)
	if token == "" && existing != nil && origin == current.Origin {
		token = existing.Token
	}
	candidate, err := newWebConnection(origin, token, s.connection.fallback)
	if err != nil {
		return ConnectionSettings{}, err
	}
	if err := checkServiceConnection(ctx, candidate); err != nil {
		return ConnectionSettings{}, err
	}
	encrypted, err := s.sealSecret("service-connection:"+origin, token)
	if err != nil {
		return ConnectionSettings{}, err
	}
	if err := s.requireConnectionAdmin(ctx, user); err != nil {
		return ConnectionSettings{}, err
	}
	row, err := store.WriteConnectionSettings(ctx, s.Pool, origin, encrypted, input.Revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return ConnectionSettings{}, errConnectionConflict
	}
	if err != nil {
		return ConnectionSettings{}, err
	}
	return ConnectionSettings{Origin: row.Origin, Revision: row.Revision, TokenConfigured: true, UpdatedAt: &row.UpdatedAt}, nil
}

func (c *managedConnection) current(ctx context.Context) (ConnectionSettings, *HTTPRemote, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	row, err := store.ReadConnectionSettings(ctx, c.service.Pool)
	if errors.Is(err, pgx.ErrNoRows) {
		if c.fallback == nil {
			return ConnectionSettings{}, nil, nil
		}
		return ConnectionSettings{Origin: c.fallback.BaseURL, TokenConfigured: true}, c.fallback, nil
	}
	if err != nil {
		return ConnectionSettings{}, nil, err
	}
	if c.remote == nil || row.Revision != c.revision {
		token, err := c.service.openSecret("service-connection:"+row.Origin, row.Ciphertext)
		if err != nil {
			return ConnectionSettings{}, nil, err
		}
		next, err := newWebConnection(row.Origin, token, c.fallback)
		if err != nil {
			return ConnectionSettings{}, nil, err
		}
		if c.remote != nil {
			c.remote.Client.CloseIdleConnections()
		}
		c.remote, c.revision = next, row.Revision
	}
	return ConnectionSettings{Origin: row.Origin, Revision: row.Revision, TokenConfigured: true, UpdatedAt: &row.UpdatedAt}, c.remote, nil
}

func (c *managedConnection) active(ctx context.Context) (*HTTPRemote, error) {
	if c.service.Pool == nil {
		return c.fallback, nil
	}
	_, remote, err := c.current(ctx)
	return remote, err
}

func (c *managedConnection) Create(ctx context.Context, request ProvisionRequest) (RemoteKey, error) {
	r, err := c.active(ctx)
	if err != nil {
		return RemoteKey{}, err
	}
	return r.Create(ctx, request)
}

func (c *managedConnection) Revoke(ctx context.Context, remoteID, operationID string) error {
	r, err := c.active(ctx)
	if err != nil {
		return err
	}
	return r.Revoke(ctx, remoteID, operationID)
}

func (c *managedConnection) SetAlertConsumptionEnabled(ctx context.Context, enabled bool, operationID string) error {
	r, err := c.active(ctx)
	if err != nil {
		return err
	}
	return r.SetAlertConsumptionEnabled(ctx, enabled, operationID)
}

func (c *managedConnection) ListMonitorContributions(ctx context.Context, cursor string, limit int) (MonitorContributionPage, error) {
	r, err := c.active(ctx)
	if err != nil || r == nil {
		return MonitorContributionPage{}, err
	}
	return r.ListMonitorContributions(ctx, cursor, limit)
}

func (c *managedConnection) ListClientUsage(ctx context.Context, cursor string, limit int) (ClientUsagePage, error) {
	r, err := c.active(ctx)
	if err != nil || r == nil {
		return ClientUsagePage{}, err
	}
	return r.ListClientUsage(ctx, cursor, limit)
}

func newWebConnection(origin, token string, fallback *HTTPRemote) (*HTTPRemote, error) {
	if len(origin) > 2048 || len(token) < 32 || len(token) > 512 || strings.IndexFunc(token, func(r rune) bool { return r < 33 || r > 126 }) >= 0 {
		return nil, errConnectionInvalid
	}
	remote, err := NewHTTPRemote(origin, token)
	if err != nil {
		return nil, errConnectionInvalid
	}
	// The environment may intentionally designate an internal origin.
	if fallback != nil && remote.BaseURL == fallback.BaseURL {
		return remote, nil
	}
	target, err := url.Parse(remote.BaseURL)
	if err != nil || target.Scheme != "https" || target.ForceQuery || strings.HasSuffix(target.Host, ":") {
		return nil, errConnectionInvalid
	}
	if target.Hostname() == "localhost" || !strings.ContainsAny(target.Hostname(), ".:") {
		return nil, errConnectionInvalid
	}
	if ip, err := netip.ParseAddr(target.Hostname()); err == nil && !publicConnectionAddress(ip) {
		return nil, errConnectionInvalid
	}
	remote.Client.Transport = &http.Transport{
		DialContext:           dialPublicConnection,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 6 * time.Second,
		MaxIdleConnsPerHost:   2,
		IdleConnTimeout:       time.Minute,
	}
	return remote, nil
}

var blockedConnectionRanges = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("168.63.129.16/32"),
	netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
}

var publicIPv6Range = netip.MustParsePrefix("2000::/3")

func publicConnectionAddress(addr netip.Addr) bool {
	addr = addr.Unmap()
	if !addr.IsGlobalUnicast() || addr.IsPrivate() || (addr.Is6() && !publicIPv6Range.Contains(addr)) {
		return false
	}
	for _, blocked := range blockedConnectionRanges {
		if blocked.Contains(addr) {
			return false
		}
	}
	return true
}

func dialPublicConnection(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, errConnectionInvalid
	}
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil || len(ips) == 0 {
		return nil, errConnectionCheck
	}
	for _, ip := range ips {
		if !publicConnectionAddress(ip) {
			return nil, errConnectionInvalid
		}
	}
	dialer := net.Dialer{Timeout: 4 * time.Second}
	for _, ip := range ips {
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
	}
	return nil, errConnectionCheck
}

func checkServiceConnection(ctx context.Context, remote *HTTPRemote) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, remote.BaseURL+"/api/v1/integrations/seat/alert-consumption", nil)
	if err != nil {
		return errConnectionCheck
	}
	req.Header.Set("Authorization", "Bearer "+remote.Token)
	resp, err := remote.Client.Do(req)
	if err != nil {
		return errConnectionCheck
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errConnectionCheck
	}
	var result struct {
		Enabled  *bool `json:"enabled"`
		Protocol int   `json:"protocol_version"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&result); err != nil || result.Enabled == nil || result.Protocol < 1 {
		return errConnectionCheck
	}
	return nil
}

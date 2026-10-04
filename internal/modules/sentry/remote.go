package sentry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var ErrRemoteUnavailable = errors.New("sentry remote integration unavailable")

type RemoteKey struct {
	ID      string `json:"key_id"`
	Version int64  `json:"version"`
	Status  string `json:"status"`
}

type ProvisionRequest struct {
	OperationID string   `json:"operation_id"`
	KeyID       string   `json:"key_id"`
	AccountID   string   `json:"account_id"`
	Name        string   `json:"name"`
	Prefix      string   `json:"key_prefix"`
	Hash        string   `json:"key_hash"`
	Permissions []string `json:"permissions"`
	Version     int      `json:"protocol_version"`
}

type Remote interface {
	Create(context.Context, ProvisionRequest) (RemoteKey, error)
	Revoke(context.Context, string, string) error
}

// AlertConsumptionRemote synchronizes Seat's persisted charging switch with
// the warning service's durable consumption gate.
type AlertConsumptionRemote interface {
	SetAlertConsumptionEnabled(context.Context, bool, string) error
}

// MonitorRemote reads the server-confirmed primary monitor intervals exported
// by EVE Sentry. It never accepts client supplied duration or reward values.
type MonitorContribution struct {
	ContributionID    string         `json:"contribution_id"`
	AccountID         string         `json:"account_id"`
	KeyID             string         `json:"key_id"`
	ClientID          string         `json:"client_id"`
	SystemID          any            `json:"system_id"`
	SystemName        string         `json:"system_name"`
	PrimaryGeneration int64          `json:"primary_generation"`
	StartedAt         string         `json:"started_at"`
	EndedAt           string         `json:"ended_at"`
	DurationSeconds   int64          `json:"duration_seconds"`
	RuleVersion       string         `json:"rule_version"`
	Eligibility       string         `json:"eligibility"`
	Evidence          map[string]any `json:"evidence"`
	CreatedAt         string         `json:"created_at"`
}

type MonitorContributionPage struct {
	Contributions              []MonitorContribution `json:"contributions"`
	NextCursor                 string                `json:"next_cursor"`
	CommittedWatermark         string                `json:"committed_watermark"`
	EarliestAvailableWatermark string                `json:"earliest_available_watermark"`
	HasMore                    bool                  `json:"has_more"`
	ProtocolVersion            int64                 `json:"protocol_version"`
	RuleVersion                string                `json:"rule_version"`
}

type MonitorRemote interface {
	ListMonitorContributions(context.Context, string, int) (MonitorContributionPage, error)
}

// ClientUsage is an authenticated online interval derived from consecutive
// server-accepted client heartbeats. Seat owns pricing and coin settlement.
type ClientUsage struct {
	UsageID         string `json:"usage_id"`
	AccountID       string `json:"account_id"`
	KeyID           string `json:"key_id"`
	ClientID        string `json:"client_id"`
	SystemID        any    `json:"system_id,omitempty"`
	StartedAt       string `json:"started_at"`
	EndedAt         string `json:"ended_at"`
	DurationSeconds int64  `json:"duration_seconds"`
	CreatedAt       string `json:"created_at"`
}

type ClientUsagePage struct {
	Usage                      []ClientUsage `json:"usage"`
	NextCursor                 string        `json:"next_cursor"`
	CommittedWatermark         string        `json:"committed_watermark"`
	EarliestAvailableWatermark string        `json:"earliest_available_watermark"`
	HasMore                    bool          `json:"has_more"`
	ProtocolVersion            int64         `json:"protocol_version"`
	RuleVersion                string        `json:"rule_version"`
}

type ClientUsageRemote interface {
	ListClientUsage(context.Context, string, int) (ClientUsagePage, error)
}

type HTTPRemote struct {
	BaseURL string
	Token   string
	Client  *http.Client
}

func NewHTTPRemote(baseURL, token string) (*HTTPRemote, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	token = strings.TrimSpace(token)
	if baseURL == "" || len(token) < 32 || strings.ContainsAny(token, "\r\n") {
		return nil, ErrRemoteUnavailable
	}
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost" || u.Hostname() == "::1"))) {
		return nil, ErrRemoteUnavailable
	}
	return &HTTPRemote{
		BaseURL: baseURL,
		Token:   token,
		Client: &http.Client{
			Timeout: 8 * time.Second,
			// The integration token must never follow a redirect to an
			// untrusted origin. The caller receives the 3xx as an unavailable
			// remote and can reconcile it without leaking the bearer token.
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func (c *HTTPRemote) Create(ctx context.Context, input ProvisionRequest) (RemoteKey, error) {
	var out RemoteKey
	if c == nil || c.Client == nil {
		return out, ErrRemoteUnavailable
	}
	body, err := json.Marshal(input)
	if err != nil {
		return out, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/v1/integrations/seat/keys", bytes.NewReader(body))
	if err != nil {
		return out, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", input.OperationID)
	resp, err := c.Client.Do(req)
	if err != nil {
		return out, ErrRemoteUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return out, fmt.Errorf("sentry remote create: %w", ErrRemoteUnavailable)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 32<<10)).Decode(&out); err != nil || out.ID == "" || out.ID != input.KeyID || out.Version != int64(input.Version) || out.Status != "active" {
		return out, ErrRemoteUnavailable
	}
	return out, nil
}

func (c *HTTPRemote) Revoke(ctx context.Context, remoteID, operationID string) error {
	if c == nil || c.Client == nil || strings.TrimSpace(remoteID) == "" {
		return ErrRemoteUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.BaseURL+"/api/v1/integrations/seat/keys/"+url.PathEscape(remoteID), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Idempotency-Key", operationID)
	resp, err := c.Client.Do(req)
	if err != nil {
		return ErrRemoteUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("sentry remote revoke: %w", ErrRemoteUnavailable)
	}
	return nil
}

func (c *HTTPRemote) SetAlertConsumptionEnabled(ctx context.Context, enabled bool, operationID string) error {
	if c == nil || c.Client == nil || strings.TrimSpace(operationID) == "" {
		return ErrRemoteUnavailable
	}
	body, err := json.Marshal(map[string]any{"enabled": enabled})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.BaseURL+"/api/v1/integrations/seat/alert-consumption", bytes.NewReader(body))
	if err != nil {
		return err
	}
	setJSONHeaders(req, c.Token, operationID)
	resp, err := c.Client.Do(req)
	if err != nil {
		return ErrRemoteUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("sentry remote alert consumption setting: %w", ErrRemoteUnavailable)
	}
	var out struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 32<<10)).Decode(&out); err != nil || out.Enabled != enabled {
		return ErrRemoteUnavailable
	}
	return nil
}

func (c *HTTPRemote) ListMonitorContributions(ctx context.Context, cursor string, limit int) (MonitorContributionPage, error) {
	var out MonitorContributionPage
	if c == nil || c.Client == nil || limit < 1 || limit > 500 {
		return out, ErrRemoteUnavailable
	}
	u := c.BaseURL + "/api/v1/integrations/seat/monitor-contributions?limit=" + url.QueryEscape(fmt.Sprintf("%d", limit))
	if strings.TrimSpace(cursor) != "" {
		u += "&after=" + url.QueryEscape(cursor)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return out, err
	}
	setJSONHeaders(req, c.Token, "")
	resp, err := c.Client.Do(req)
	if err != nil {
		return out, ErrRemoteUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return out, fmt.Errorf("sentry remote monitor contributions: %w", ErrRemoteUnavailable)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 512<<10)).Decode(&out); err != nil || out.ProtocolVersion != 1 {
		return out, ErrRemoteUnavailable
	}
	if len(out.Contributions) > limit {
		return out, ErrRemoteUnavailable
	}
	return out, nil
}

func (c *HTTPRemote) ListClientUsage(ctx context.Context, cursor string, limit int) (ClientUsagePage, error) {
	var out ClientUsagePage
	if c == nil || c.Client == nil || limit < 1 || limit > 500 {
		return out, ErrRemoteUnavailable
	}
	u := c.BaseURL + "/api/v1/integrations/seat/client-usage?limit=" + url.QueryEscape(fmt.Sprintf("%d", limit))
	if strings.TrimSpace(cursor) != "" {
		u += "&after=" + url.QueryEscape(cursor)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return out, err
	}
	setJSONHeaders(req, c.Token, "")
	resp, err := c.Client.Do(req)
	if err != nil {
		return out, ErrRemoteUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return out, fmt.Errorf("sentry remote client usage: %w", ErrRemoteUnavailable)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 512<<10)).Decode(&out); err != nil || out.ProtocolVersion != 1 {
		return out, ErrRemoteUnavailable
	}
	if len(out.Usage) > limit {
		return out, ErrRemoteUnavailable
	}
	return out, nil
}

func setJSONHeaders(req *http.Request, token, idempotency string) {
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	if idempotency != "" {
		req.Header.Set("Idempotency-Key", idempotency)
	}
}

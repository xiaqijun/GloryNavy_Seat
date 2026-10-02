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

// AlertGrantRequest is the time allowance sent to Sentry. Protocol v2 omits
// pricing entirely: Seat remains the source of truth for policy, coin
// reservation, settlement and refunds. The price fields stay for v1 replay.
type AlertGrantRequest struct {
	OperationID     string    `json:"operation_id"`
	GrantID         string    `json:"grant_id"`
	AccountID       string    `json:"account_id"`
	KeyID           string    `json:"key_id,omitempty"`
	PriceVersion    string    `json:"price_version"`
	UnitSeconds     int64     `json:"unit_seconds"`
	UnitPriceMinor  int64     `json:"unit_price_minor"`
	ReservedSeconds int64     `json:"reserved_seconds"`
	ExpiresAt       time.Time `json:"-"`
	ProtocolVersion int64     `json:"-"`
}

type AlertGrant struct {
	GrantID          string `json:"grant_id"`
	OperationID      string `json:"operation_id"`
	AccountID        string `json:"account_id"`
	KeyID            string `json:"key_id"`
	PriceVersion     string `json:"price_version"`
	UnitSeconds      int64  `json:"unit_seconds"`
	UnitPriceMinor   int64  `json:"unit_price_minor"`
	ReservedSeconds  int64  `json:"reserved_seconds"`
	RemainingSeconds int64  `json:"remaining_seconds"`
	ExpiresAt        string `json:"expires_at"`
	Status           string `json:"status"`
	ProtocolVersion  int64  `json:"protocol_version"`
}

type AlertEvent struct {
	ChargeEventID string `json:"charge_event_id"`
	Revision      int64  `json:"revision"`
	Lifecycle     string `json:"lifecycle"`
}

type AlertEventPage struct {
	Events                     []AlertEvent `json:"events"`
	NextCursor                 string       `json:"next_cursor"`
	CommittedWatermark         string       `json:"committed_watermark"`
	EarliestAvailableWatermark string       `json:"earliest_available_watermark"`
	HasMore                    bool         `json:"has_more"`
	ProtocolVersion            int64        `json:"protocol_version"`
}

type AlertDelivery struct {
	DeliveryID       string `json:"delivery_id"`
	ChargeEventID    string `json:"charge_event_id"`
	Revision         int64  `json:"revision"`
	GrantID          string `json:"grant_id"`
	AccountID        string `json:"account_id"`
	KeyID            string `json:"key_id"`
	StartedAt        string `json:"started_at"`
	EndedAt          string `json:"ended_at"`
	DurationSeconds  int64  `json:"duration_seconds"`
	ConsumptionState string `json:"consumption_state"`
	Status           string `json:"status"`
	AckAt            string `json:"ack_at"`
	AckEvidence      map[string]any `json:"ack_evidence"`
	CreatedAt        string `json:"created_at"`
}

type AlertDeliveryPage struct {
	Deliveries                 []AlertDelivery `json:"deliveries"`
	NextCursor                 string          `json:"next_cursor"`
	CommittedWatermark         string          `json:"committed_watermark"`
	EarliestAvailableWatermark string          `json:"earliest_available_watermark"`
	HasMore                    bool            `json:"has_more"`
	ProtocolVersion            int64           `json:"protocol_version"`
}

// AlertRemote is deliberately separate from Remote so existing key lifecycle
// fakes and implementations do not need to implement the billing protocol.
type AlertRemote interface {
	CreateAlertGrant(context.Context, AlertGrantRequest) (AlertGrant, error)
	RevokeAlertGrant(context.Context, string, string) error
	ListAlertEvents(context.Context, string, int) (AlertEventPage, error)
	ListAlertDeliveries(context.Context, string, int) (AlertDeliveryPage, error)
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

func (c *HTTPRemote) CreateAlertGrant(ctx context.Context, input AlertGrantRequest) (AlertGrant, error) {
	var out AlertGrant
	protocolVersion := input.ProtocolVersion
	if protocolVersion == 0 {
		protocolVersion = 1
	}
	if c == nil || c.Client == nil || strings.TrimSpace(input.OperationID) == "" || strings.TrimSpace(input.GrantID) == "" || strings.TrimSpace(input.AccountID) == "" || input.ReservedSeconds <= 0 || input.ExpiresAt.IsZero() {
		return out, ErrRemoteUnavailable
	}
	if protocolVersion == 1 && (strings.TrimSpace(input.PriceVersion) == "" || input.UnitSeconds <= 0 || input.UnitPriceMinor <= 0) {
		return out, ErrRemoteUnavailable
	}
	payload := struct {
		OperationID     string `json:"operation_id"`
		GrantID         string `json:"grant_id"`
		AccountID       string `json:"account_id"`
		KeyID           string `json:"key_id,omitempty"`
		PriceVersion    string `json:"price_version,omitempty"`
		UnitSeconds     int64  `json:"unit_seconds,omitempty"`
		UnitPriceMinor  int64  `json:"unit_price_minor,omitempty"`
		ReservedSeconds int64  `json:"reserved_seconds"`
		ExpiresAt       string `json:"expires_at"`
		ProtocolVersion int64  `json:"protocol_version"`
	}{OperationID: input.OperationID, GrantID: input.GrantID, AccountID: input.AccountID, KeyID: input.KeyID, PriceVersion: input.PriceVersion, UnitSeconds: input.UnitSeconds, UnitPriceMinor: input.UnitPriceMinor, ReservedSeconds: input.ReservedSeconds, ExpiresAt: input.ExpiresAt.UTC().Format(time.RFC3339), ProtocolVersion: protocolVersion}
	body, err := json.Marshal(payload)
	if err != nil {
		return out, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/v1/integrations/seat/alert-grants", bytes.NewReader(body))
	if err != nil {
		return out, err
	}
	setJSONHeaders(req, c.Token, input.OperationID)
	resp, err := c.Client.Do(req)
	if err != nil {
		return out, ErrRemoteUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return out, fmt.Errorf("sentry remote alert grant: %w", ErrRemoteUnavailable)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 32<<10)).Decode(&out); err != nil || out.GrantID != input.GrantID || out.OperationID != input.OperationID || out.AccountID != input.AccountID || (input.KeyID != "" && out.KeyID != input.KeyID) || out.ReservedSeconds != input.ReservedSeconds || out.RemainingSeconds < 0 || out.RemainingSeconds > out.ReservedSeconds || out.Status != "active" || out.ProtocolVersion != protocolVersion {
		return out, ErrRemoteUnavailable
	}
	if protocolVersion == 1 && (out.PriceVersion != input.PriceVersion || out.UnitSeconds != input.UnitSeconds || out.UnitPriceMinor != input.UnitPriceMinor) {
		return out, ErrRemoteUnavailable
	}
	remoteExpiry, err := time.Parse(time.RFC3339, out.ExpiresAt)
	if err != nil || !remoteExpiry.Equal(input.ExpiresAt.UTC().Truncate(time.Second)) {
		return out, ErrRemoteUnavailable
	}
	return out, nil
}

func (c *HTTPRemote) RevokeAlertGrant(ctx context.Context, grantID, operationID string) error {
	if c == nil || c.Client == nil || strings.TrimSpace(grantID) == "" {
		return ErrRemoteUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.BaseURL+"/api/v1/integrations/seat/alert-grants/"+url.PathEscape(grantID), nil)
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
		return fmt.Errorf("sentry remote alert grant revoke: %w", ErrRemoteUnavailable)
	}
	return nil
}

func (c *HTTPRemote) ListAlertEvents(ctx context.Context, cursor string, limit int) (AlertEventPage, error) {
	var out AlertEventPage
	if c == nil || c.Client == nil || limit < 1 || limit > 500 {
		return out, ErrRemoteUnavailable
	}
	u := c.BaseURL + "/api/v1/integrations/seat/alert-events?limit=" + url.QueryEscape(fmt.Sprintf("%d", limit))
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
		return out, fmt.Errorf("sentry remote alert events: %w", ErrRemoteUnavailable)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 128<<10)).Decode(&out); err != nil || out.ProtocolVersion != 1 {
		return out, ErrRemoteUnavailable
	}
	return out, nil
}

func (c *HTTPRemote) ListAlertDeliveries(ctx context.Context, cursor string, limit int) (AlertDeliveryPage, error) {
	var out AlertDeliveryPage
	if c == nil || c.Client == nil || limit < 1 || limit > 500 {
		return out, ErrRemoteUnavailable
	}
	u := c.BaseURL + "/api/v1/integrations/seat/alert-deliveries?limit=" + url.QueryEscape(fmt.Sprintf("%d", limit))
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
		return out, fmt.Errorf("sentry remote alert deliveries: %w", ErrRemoteUnavailable)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 128<<10)).Decode(&out); err != nil || out.ProtocolVersion != 1 {
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

// Package courier provides HA Logistics route and appraisal data. It does not
// create game contracts, hold cargo, or persist account data.
package courier

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/sync/singleflight"
)

var (
	ErrUnavailable = errors.New("courier unavailable")
	ErrInput       = errors.New("invalid cargo list")
	ErrBusy        = errors.New("courier busy")
)

type Route struct {
	ID                   int64   `json:"id"`
	Start                string  `json:"start"`
	End                  string  `json:"end"`
	CostPerM3            float64 `json:"cost_per_m3"`
	CollateralPercentage float64 `json:"collateral_percentage"`
	TransportType        string  `json:"transport_type"`
	TransportVolume      float64 `json:"transport_volume"`
	Paused               int     `json:"paused"`
	Expiration           int     `json:"expiration"`
	Completion           int     `json:"completion"`
	MinimumCost          float64 `json:"minimum_cost"`
}
type Category struct {
	Name     *string `json:"category_name"`
	Priority int     `json:"category_priority"`
}
type Routes struct {
	Routes     []Route             `json:"routes"`
	Categories map[string]Category `json:"location_categories"`
	FetchedAt  time.Time           `json:"fetched_at"`
}
type Appraisal struct {
	Buy    *float64           `json:"buy_price"`
	Sell   *float64           `json:"sell_price"`
	Volume *float64           `json:"volume"`
	URL    *string            `json:"janice_url"`
	Failed map[string]float64 `json:"failed_items"`
}
type AppraisalResult struct {
	Appraisal Appraisal `json:"appraisal"`
	FromCache bool      `json:"from_cache"`
}
type Service struct {
	client     *http.Client
	base       string
	mu         sync.Mutex
	cached     Routes
	expires    time.Time
	appraisals chan struct{}
	sessionMu  sync.Mutex
	session    upstreamSession
	quoteMu    sync.Mutex
	quotes     map[[32]byte]cachedAppraisal
	quoteGroup singleflight.Group
}

func NewService() *Service {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if configured := strings.TrimSpace(os.Getenv("COURIER_HTTP_PROXY")); configured != "" {
		proxy, err := url.Parse(configured)
		transport.Proxy = func(*http.Request) (*url.URL, error) {
			if err != nil || proxy.Host == "" || (proxy.Scheme != "http" && proxy.Scheme != "https") || (proxy.Path != "" && proxy.Path != "/") || proxy.RawQuery != "" || proxy.Fragment != "" {
				return nil, ErrUnavailable
			}
			return proxy, nil
		}
	}
	return &Service{
		base:       "https://hallogistics.space",
		client:     &http.Client{Transport: transport, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		appraisals: make(chan struct{}, 2),
	}
}

func finite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v <= 9007199254740991
}
func nameOK(s string) bool { return strings.TrimSpace(s) != "" && utf8.RuneCountInString(s) <= 512 }

func (s *Service) request(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	r, err := http.NewRequestWithContext(ctx, method, s.base+path, body)
	if err != nil {
		return nil, ErrUnavailable
	}
	r.Header.Set("Accept", "application/json")
	r.Header.Set("User-Agent", "GloryNavy-SeAT/1.0 (+https://seat.kisectool.com)")
	return s.client.Do(r)
}

func readJSON(r *http.Response, v any) error {
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		return ErrUnavailable
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, 2*1024*1024+1))
	if err != nil || len(data) > 2*1024*1024 || json.Unmarshal(data, v) != nil {
		return ErrUnavailable
	}
	return nil
}

func (s *Service) Routes(ctx context.Context) (Routes, error) {
	// One bounded fetch per cache window; concurrent readers cannot stampede upstream.
	s.mu.Lock()
	defer s.mu.Unlock()
	if time.Now().Before(s.expires) {
		return s.cached, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	r, err := s.request(ctx, "GET", "/api/routes", nil)
	if err != nil {
		return Routes{}, ErrUnavailable
	}
	var raw struct {
		Routes     []json.RawMessage   `json:"routes"`
		Categories map[string]Category `json:"location_categories"`
	}
	if readJSON(r, &raw) != nil || raw.Routes == nil || len(raw.Routes) > 10000 {
		return Routes{}, ErrUnavailable
	}
	out := Routes{Routes: make([]Route, 0, len(raw.Routes)), Categories: raw.Categories, FetchedAt: time.Now().UTC()}
	if out.Categories == nil {
		out.Categories = map[string]Category{}
	}
	seen := map[int64]bool{}
	for _, data := range raw.Routes {
		var fields map[string]json.RawMessage
		var route Route
		if json.Unmarshal(data, &fields) != nil || json.Unmarshal(data, &route) != nil {
			return Routes{}, ErrUnavailable
		}
		for _, k := range []string{"id", "start", "end", "cost_per_m3", "collateral_percentage", "transport_type", "transport_volume", "paused", "expiration", "completion", "minimum_cost"} {
			if len(fields[k]) == 0 || string(fields[k]) == "null" {
				return Routes{}, ErrUnavailable
			}
		}
		if route.ID <= 0 || seen[route.ID] || !nameOK(route.Start) || !nameOK(route.End) || !nameOK(route.TransportType) || (route.Paused != 0 && route.Paused != 1) || route.Expiration < 1 || route.Completion < 1 || route.Expiration > 365 || route.Completion > 365 {
			return Routes{}, ErrUnavailable
		}
		for _, v := range []float64{route.CostPerM3, route.CollateralPercentage, route.TransportVolume, route.MinimumCost} {
			if !finite(v) {
				return Routes{}, ErrUnavailable
			}
		}
		seen[route.ID] = true
		out.Routes = append(out.Routes, route)
	}
	s.cached = out
	s.expires = time.Now().Add(5 * time.Minute)
	return out, nil
}

func (s *Service) Appraise(ctx context.Context, text string) (AppraisalResult, error) {
	text = strings.TrimSpace(text)
	if text == "" || utf8.RuneCountInString(text) > 100000 || strings.Count(text, "\n") >= 5000 {
		return AppraisalResult{}, ErrInput
	}
	return s.cachedAppraise(ctx, text)
}

func (s *Service) appraiseUpstream(ctx context.Context, text string) (AppraisalResult, error) {
	select {
	case s.appraisals <- struct{}{}:
		defer func() { <-s.appraisals }()
	default:
		return AppraisalResult{}, ErrBusy
	}
	ctx, cancel := context.WithTimeout(ctx, 28*time.Second)
	defer cancel()
	session, err := s.appraisalSession(ctx)
	if err != nil {
		return AppraisalResult{}, ErrUnavailable
	}
	body, _ := json.Marshal(map[string]string{"appraisal_text": text})
	r, err := http.NewRequestWithContext(ctx, "POST", s.base+"/api/appraisal", bytes.NewReader(body))
	if err != nil {
		return AppraisalResult{}, ErrUnavailable
	}
	r.Header.Set("Accept", "application/json")
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("User-Agent", "GloryNavy-SeAT/1.0 (+https://seat.kisectool.com)")
	r.Header.Set("Origin", s.base)
	r.Header.Set("Referer", s.base+"/")
	r.Header.Set("X-XSRFToken", session.token)
	r.AddCookie(&session.cookie)
	response, err := s.client.Do(r)
	if err != nil {
		return AppraisalResult{}, ErrUnavailable
	}
	if response.StatusCode == http.StatusForbidden {
		s.sessionMu.Lock()
		if s.session.token == session.token {
			s.session = upstreamSession{}
		}
		s.sessionMu.Unlock()
	}
	var payload struct {
		Success   bool      `json:"success"`
		FromCache bool      `json:"from_cache"`
		Appraisal Appraisal `json:"appraisal"`
	}
	if readJSON(response, &payload) != nil || !payload.Success {
		return AppraisalResult{}, ErrUnavailable
	}
	a := payload.Appraisal
	for _, n := range []*float64{a.Buy, a.Sell, a.Volume} {
		if n == nil || !finite(*n) {
			return AppraisalResult{}, ErrUnavailable
		}
	}
	if a.Failed == nil {
		a.Failed = map[string]float64{}
	}
	for k, v := range a.Failed {
		if !nameOK(k) || !finite(v) || v <= 0 {
			return AppraisalResult{}, ErrUnavailable
		}
	}
	if a.URL != nil {
		u, err := url.Parse(*a.URL)
		if err != nil || u.Scheme != "https" || u.Host != "janice.e-351.com" || u.User != nil {
			a.URL = nil
		}
	}
	return AppraisalResult{Appraisal: a, FromCache: payload.FromCache}, nil
}

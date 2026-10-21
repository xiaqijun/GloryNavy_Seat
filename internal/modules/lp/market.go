package lp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"fmt"
	"glorynavy.local/seat/internal/modules/eve"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"
)

var stations = map[int64]int64{10000002: 60003760, 10000043: 60008494, 10000032: 60011866, 10000030: 60004588, 10000042: 60005686}

type Price struct {
	Buy  *float64 `json:"buy"`
	Sell *float64 `json:"sell"`
}
type Prices struct {
	Items      map[int64]Price `json:"items"`
	ObservedAt time.Time       `json:"observed_at"`
	Source     string          `json:"source"`
	validUntil time.Time
}
type MarketClient struct {
	client           *http.Client
	base             string
	gate             chan struct{}
	group            singleflight.Group
	mu               sync.Mutex
	cache            map[string]Prices
	Request          func(context.Context, eve.ESIRequest, any) (eve.ESIResponse, error)
	unavailableUntil time.Time
}

func NewMarketClient() *MarketClient {
	t := http.DefaultTransport.(*http.Transport).Clone()
	valid := true
	if raw := os.Getenv("LP_HTTP_PROXY"); raw != "" {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			valid = false
		} else {
			t.Proxy = http.ProxyURL(u)
		}
	}
	s := &MarketClient{base: "https://market.fuzzwork.co.uk/aggregates/", gate: make(chan struct{}, 2), cache: map[string]Prices{}}
	if valid {
		s.client = &http.Client{Transport: t, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	return s
}
func (m *MarketClient) Prices(ctx context.Context, region int64, ids []int64) (Prices, error) {
	station, ok := stations[region]
	if !ok || len(ids) == 0 || len(ids) > 100 {
		return Prices{}, ErrInput
	}
	ids = append([]int64(nil), ids...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	parts := make([]string, 0, len(ids))
	var prev int64
	for _, id := range ids {
		if id <= 0 || id > 2147483647 {
			return Prices{}, ErrInput
		}
		if id != prev {
			parts = append(parts, strconv.FormatInt(id, 10))
			prev = id
		}
	}
	key := strconv.FormatInt(station, 10) + ":" + strings.Join(parts, ",")
	ch := m.group.DoChan(key, func() (any, error) {
		m.mu.Lock()
		cached, exists := m.cache[key]
		m.mu.Unlock()
		if exists && time.Now().Before(cached.validUntil) {
			return cached, nil
		}
		select {
		case m.gate <- struct{}{}:
			defer func() { <-m.gate }()
		default:
			return Prices{}, ErrBusy
		}
		if m.client == nil {
			return Prices{}, ErrUnavailable
		}
		// Detached bounded fetch lets concurrent readers share work without one
		// canceled browser aborting every other reader.
		requestCtx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		m.mu.Lock()
		cooling := time.Now().Before(m.unavailableUntil)
		m.mu.Unlock()
		out, err := Prices{}, ErrUnavailable
		if !cooling {
			out, err = m.fetchFuzzwork(requestCtx, station, parts)
		}
		if err != nil {
			if !cooling {
				m.mu.Lock()
				m.unavailableUntil = time.Now().Add(time.Minute)
				m.mu.Unlock()
			}
			if m.Request == nil {
				return Prices{}, err
			}
			out, err = m.fetchESI(requestCtx, region, station, parts)
			if err != nil {
				return Prices{}, err
			}
		}
		m.mu.Lock()
		for k, v := range m.cache {
			if !time.Now().Before(v.validUntil) {
				delete(m.cache, k)
			}
		}
		if len(m.cache) >= 256 {
			for k := range m.cache {
				delete(m.cache, k)
				break
			}
		}
		m.cache[key] = out
		m.mu.Unlock()
		return out, nil
	})
	select {
	case <-ctx.Done():
		return Prices{}, ctx.Err()
	case result := <-ch:
		if result.Err != nil {
			return Prices{}, result.Err
		}
		return result.Val.(Prices), nil
	}
}
func marketSide(price, volume json.Number) *float64 {
	p, e := price.Float64()
	v, ve := volume.Float64()
	if e != nil || ve != nil || !finite(p) || !finite(v) || p <= 0 || v <= 0 {
		return nil
	}
	return &p
}

func (m *MarketClient) fetchFuzzwork(ctx context.Context, station int64, parts []string) (Prices, error) {
	r, err := http.NewRequestWithContext(ctx, "GET", m.base+"?station="+strconv.FormatInt(station, 10)+"&types="+strings.Join(parts, ","), nil)
	if err != nil {
		return Prices{}, ErrUnavailable
	}
	r.Header.Set("Accept", "application/json")
	r.Header.Set("User-Agent", "GloryNavy-SeAT/1.0 (+https://seat.glorynavy.com)")
	response, err := m.client.Do(r)
	if err != nil {
		return Prices{}, ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return Prices{}, ErrUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (2<<20)+1))
	if err != nil || len(body) > 2<<20 {
		return Prices{}, ErrUnavailable
	}
	var data map[string]struct {
		Buy struct {
			Max    json.Number `json:"max"`
			Volume json.Number `json:"volume"`
		} `json:"buy"`
		Sell struct {
			Min    json.Number `json:"min"`
			Volume json.Number `json:"volume"`
		} `json:"sell"`
	}
	if json.Unmarshal(body, &data) != nil || data == nil {
		return Prices{}, ErrUnavailable
	}
	out := Prices{Items: map[int64]Price{}, ObservedAt: time.Now().UTC(), Source: "fuzzwork", validUntil: time.Now().Add(time.Minute)}
	for _, part := range parts {
		v := data[part]
		id, _ := strconv.ParseInt(part, 10, 64)
		out.Items[id] = Price{marketSide(v.Buy.Max, v.Buy.Volume), marketSide(v.Sell.Min, v.Sell.Volume)}
	}
	return out, nil
}

func (m *MarketClient) fetchESI(ctx context.Context, region, station int64, parts []string) (Prices, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	out := Prices{Items: map[int64]Price{}, Source: "esi", validUntil: time.Now().Add(time.Minute)}
	var mu sync.Mutex
	group, ctx := errgroup.WithContext(ctx)
	group.SetLimit(3)
	for _, part := range parts {
		group.Go(func() error {
			id, _ := strconv.ParseInt(part, 10, 64)
			var price Price
			pages := 0
			var observed time.Time
			until := time.Now().Add(time.Minute)
			for page := 1; page <= max(1, pages); page++ {
				var orders []struct {
					TypeID    int64   `json:"type_id"`
					Location  int64   `json:"location_id"`
					Buy       bool    `json:"is_buy_order"`
					Price     float64 `json:"price"`
					Remaining int64   `json:"volume_remain"`
				}
				var meta eve.ESIResponse
				var err error
				for attempt := 0; attempt < 100; attempt++ {
					meta, err = m.Request(ctx, eve.ESIRequest{Method: "GET", Path: fmt.Sprintf("/markets/%d/orders/?order_type=all&type_id=%d&page=%d", region, id, page), ExpectPages: true}, &orders)
					until := eve.ESIRetryTime(err)
					if until.IsZero() || time.Until(until) > 500*time.Millisecond || attempt == 99 {
						break
					}
					timer := time.NewTimer(max(time.Millisecond, time.Until(until)) + 20*time.Millisecond)
					select {
					case <-ctx.Done():
						timer.Stop()
						return ctx.Err()
					case <-timer.C:
					}
				}
				if err != nil {
					if eve.IsESINotFound(err) {
						observed = time.Now().UTC()
						break
					}
					return err
				}
				if orders == nil || meta.Pages < 1 || meta.Pages > 20 || pages != 0 && meta.Pages != pages {
					return ErrUnavailable
				}
				pages = meta.Pages
				stamp := meta.ContentUpdatedAt
				if stamp.IsZero() {
					stamp = meta.ValidatedAt
				}
				if observed.IsZero() || stamp.Before(observed) {
					observed = stamp
				}
				if meta.ExpiresAt.Before(until) {
					until = meta.ExpiresAt
				}
				for _, o := range orders {
					if o.TypeID != id || o.Location != station || o.Remaining <= 0 {
						continue
					}
					if !finite(o.Price) || o.Price <= 0 {
						return ErrUnavailable
					}
					if o.Buy {
						if price.Buy == nil || o.Price > *price.Buy {
							v := o.Price
							price.Buy = &v
						}
					} else if price.Sell == nil || o.Price < *price.Sell {
						v := o.Price
						price.Sell = &v
					}
				}
			}
			mu.Lock()
			out.Items[id] = price
			if out.ObservedAt.IsZero() || observed.Before(out.ObservedAt) {
				out.ObservedAt = observed
			}
			if until.Before(out.validUntil) {
				out.validUntil = until
			}
			mu.Unlock()
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return Prices{}, err
	}
	return out, nil
}

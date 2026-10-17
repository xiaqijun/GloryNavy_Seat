package courier

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const routeFixture = `{"id":1,"start":"Jita","end":"1DH-SX","cost_per_m3":600,"collateral_percentage":1,"transport_type":"JF full max","transport_volume":375000,"paused":0,"expiration":14,"completion":7,"minimum_cost":10000000}`
const appraisalFixture = `{"success":true,"from_cache":true,"appraisal":{"buy_price":100,"sell_price":120,"volume":3,"janice_url":"https://janice.e-351.com/a/test","failed_items":{"Unknown":2}}}`

func TestCourierProxyIsScopedAndInvalidConfigurationFailsClosed(t *testing.T) {
	request := httptest.NewRequest("GET", "https://hallogistics.space/api/routes", nil)
	t.Setenv("COURIER_HTTP_PROXY", "http://127.0.0.1:7890")
	s := NewService()
	proxy, err := s.client.Transport.(*http.Transport).Proxy(request)
	if err != nil || proxy.String() != "http://127.0.0.1:7890" {
		t.Fatal(proxy, err)
	}
	for _, value := range []string{"file:///tmp/proxy", "http://", "http://localhost:7890/path", "http://localhost:7890?secret=hidden"} {
		t.Setenv("COURIER_HTTP_PROXY", value)
		s = NewService()
		if _, err = s.client.Transport.(*http.Transport).Proxy(request); !errors.Is(err, ErrUnavailable) {
			t.Fatal("bad proxy configuration accepted", err)
		}
	}
}

func testService(t *testing.T, h http.HandlerFunc) *Service {
	t.Helper()
	server := httptest.NewServer(h)
	t.Cleanup(server.Close)
	s := NewService()
	s.base = server.URL
	return s
}
func TestRoutesCacheValidationAndFailure(t *testing.T) {
	var calls atomic.Int32
	body := `{"routes":[` + routeFixture + `],"location_categories":{"Jita":{"category_name":"High-sec","category_priority":0}}}`
	s := testService(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); fmt.Fprint(w, body) })
	for i := 0; i < 2; i++ {
		v, err := s.Routes(context.Background())
		if err != nil || len(v.Routes) != 1 || v.Routes[0].MinimumCost != 10000000 || v.FetchedAt.IsZero() {
			t.Fatal(v, err)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("cache missed", calls.Load())
	}
	body = `{"routes":[{"id":1,"start":"Jita","end":"1DH-SX"}]}`
	s.expires = time.Time{}
	if _, err := s.Routes(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatal("missing rates became zero", err)
	}
	body = `{"routes":[` + strings.Replace(routeFixture, `"cost_per_m3":600`, `"cost_per_m3":-1`, 1) + `]}`
	if _, err := s.Routes(context.Background()); err == nil {
		t.Fatal("negative rate accepted")
	}
	body = `{"routes":[` + routeFixture + `,` + routeFixture + `]}`
	if _, err := s.Routes(context.Background()); err == nil {
		t.Fatal("duplicate route accepted")
	}
}
func TestAppraisalSessionAndResponse(t *testing.T) {
	var calls atomic.Int32
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path == "/" {
			http.SetCookie(w, &http.Cookie{Name: "_xsrf", Value: "test-token"})
			return
		}
		cookie, err := r.Cookie("_xsrf")
		var body map[string]string
		if err != nil || cookie.Value != "test-token" || r.Header.Get("X-XSRFToken") != "test-token" || r.Header.Get("Origin") == "" || json.NewDecoder(r.Body).Decode(&body) != nil || body["appraisal_text"] != "三钛合金\t3" {
			t.Error("bad session or input")
		}
		fmt.Fprint(w, appraisalFixture)
	})
	v, err := s.Appraise(context.Background(), " 三钛合金\t3 ")
	if err != nil || !v.FromCache || *v.Appraisal.Sell != 120 || v.Appraisal.Failed["Unknown"] != 2 || calls.Load() != 2 {
		t.Fatal(v, err)
	}
	for _, input := range []string{"", strings.Repeat("字", 100001), strings.Repeat("x\n", 5001)} {
		if _, err = s.Appraise(context.Background(), input); !errors.Is(err, ErrInput) {
			t.Fatal("bad input accepted", err)
		}
	}
	if calls.Load() != 2 {
		t.Fatal("invalid input reached upstream")
	}
	s.appraisals <- struct{}{}
	s.appraisals <- struct{}{}
	if _, err = s.Appraise(context.Background(), "Tritanium 1"); !errors.Is(err, ErrBusy) {
		t.Fatal(err)
	}
}
func TestAppraisalRejectsInvalidAndUnsafeUpstream(t *testing.T) {
	for _, body := range []string{`not json`, `{"success":false}`, `{"success":true,"appraisal":{"volume":3}}`, strings.Replace(appraisalFixture, `"volume":3`, `"volume":-1`, 1)} {
		t.Run(body[:min(len(body), 30)], func(t *testing.T) {
			s := testService(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/" {
					http.SetCookie(w, &http.Cookie{Name: "_xsrf", Value: "token"})
					return
				}
				fmt.Fprint(w, body)
			})
			if _, err := s.Appraise(context.Background(), "Tritanium 1"); !errors.Is(err, ErrUnavailable) {
				t.Fatal(err)
			}
		})
	}
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.SetCookie(w, &http.Cookie{Name: "_xsrf", Value: "token"})
			return
		}
		fmt.Fprint(w, strings.Replace(appraisalFixture, "https://janice.e-351.com/a/test", "https://example.com/phish", 1))
	})
	v, err := s.Appraise(context.Background(), "Tritanium 1")
	if err != nil || v.Appraisal.URL != nil {
		t.Fatal("unsafe link", v, err)
	}
	redirects := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirects++ }))
	defer target.Close()
	s = testService(t, func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) })
	if _, err = s.Appraise(context.Background(), "Tritanium 1"); err == nil || redirects != 0 {
		t.Fatal("redirect followed", err, redirects)
	}
	if _, err = s.Routes(context.Background()); err == nil || redirects != 0 {
		t.Fatal("routes redirect followed", err, redirects)
	}
}
func TestCourierHTTPInputAndErrorIsolation(t *testing.T) {
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		fmt.Fprint(w, "secret upstream debug")
	})
	h := Handler{Service: s}
	for _, body := range []string{`{`, `{"appraisal_text":"x","unexpected":1}`, `{"appraisal_text":"x"} {}`, `{"appraisal_text":""}`} {
		w := httptest.NewRecorder()
		h.appraisal(w, httptest.NewRequest("POST", "/", strings.NewReader(body)))
		if w.Code != 400 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	h.appraisal(w, httptest.NewRequest("POST", "/", strings.NewReader(`{"appraisal_text":"Tritanium 1"}`)))
	if w.Code != 503 || strings.Contains(w.Body.String(), "secret") || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Code, w.Body.String())
	}
}

package esiclient

import (
	"net/http"
	"testing"
	"time"
)

func TestCacheFreshnessIsNotQuotaWindow(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	for _, tc := range []struct {
		name, route string
		h           http.Header
		ttl         int
		persist     bool
	}{
		{"catalog", "GET /characters/{id}/contracts/{id}/items/", http.Header{}, 3600, true},
		{"max age and age", "GET /status/", http.Header{"Cache-Control": {"max-age=30"}, "Age": {"12"}}, 18, true},
		{"exhausted age", "GET /status/", http.Header{"Cache-Control": {"max-age=10"}, "Age": {"12"}}, 0, true},
		{"expired", "GET /status/", http.Header{"Expires": {now.Add(-time.Hour).Format(http.TimeFormat)}}, 0, true},
		{"revalidate", "GET /status/", http.Header{"Cache-Control": {"no-cache, max-age=300"}}, 0, true},
		{"no store", "GET /status/", http.Header{"Cache-Control": {"no-store, max-age=300"}}, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			expires, persist := cacheExpiry(tc.h, tc.route, now)
			if expires.Sub(now) != time.Duration(tc.ttl)*time.Second || persist != tc.persist {
				t.Fatal(expires.Sub(now), persist)
			}
		})
	}
}

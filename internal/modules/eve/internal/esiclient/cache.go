package esiclient

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// HTTP freshness and the quota window are independent clocks. A known zero TTL
// is not the same as a missing policy; no-store also removes an older cache entry.
func cacheExpiry(h http.Header, route string, now time.Time) (time.Time, bool) {
	ttl := int64(300)
	if p, known := catalog[route]; known {
		ttl = p.CacheSeconds
	}
	expires := now.Add(time.Duration(ttl) * time.Second)
	if date, err := http.ParseTime(h.Get("Expires")); err == nil {
		expires = date
	}
	noStore, revalidate := false, false
	for _, directive := range strings.Split(h.Get("Cache-Control"), ",") {
		key, value, _ := strings.Cut(strings.TrimSpace(directive), "=")
		switch strings.ToLower(key) {
		case "no-store":
			noStore = true
		case "no-cache":
			revalidate = true
		case "max-age":
			seconds, err := strconv.ParseInt(strings.Trim(value, "\""), 10, 32)
			if err == nil && seconds >= 0 {
				age, _ := strconv.ParseInt(h.Get("Age"), 10, 32)
				expires = now.Add(time.Duration(max(int64(0), seconds-max(int64(0), age))) * time.Second)
			}
		}
	}
	if noStore || revalidate || expires.Before(now) {
		expires = now
	}
	if expires.After(now.Add(24 * time.Hour)) {
		expires = now.Add(24 * time.Hour)
	}
	return expires, !noStore
}

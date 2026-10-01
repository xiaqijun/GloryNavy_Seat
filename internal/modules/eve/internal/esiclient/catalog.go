package esiclient

import (
	_ "embed"
	"encoding/json"
	"strings"
)

//go:embed catalog.json
var catalogJSON []byte

type routePolicy struct {
	Group         string `json:"group"`
	Capacity      int64  `json:"capacity"`
	WindowSeconds int64  `json:"window_seconds"`
	CacheSeconds  int64  `json:"cache_seconds"`
}

var catalog = func() map[string]routePolicy {
	var c struct {
		CompatibilityDate string                 `json:"compatibility_date"`
		Routes            map[string]routePolicy `json:"routes"`
	}
	if err := json.Unmarshal(catalogJSON, &c); err != nil || c.CompatibilityDate != CompatibilityDate || len(c.Routes) == 0 {
		panic("ESI catalog does not match compatibility date")
	}
	return c.Routes
}()

func routeKey(method, path string) string {
	return method + " " + strings.TrimRight(pathNumber.ReplaceAllString(strings.Split(path, "?")[0], "/{id}"), "/") + "/"
}

// BucketPolicy exposes only verified policy metadata to management responses.
// It does not claim an upstream balance or that an HTTP request was observed.
func BucketPolicy(group string) (capacity, windowSeconds int64) {
	for _, p := range catalog {
		if p.Group == group && group != "" {
			return p.Capacity, p.WindowSeconds
		}
	}
	return 0, 0
}

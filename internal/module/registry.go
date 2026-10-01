// Package module defines the host contract for trusted, compiled-in modules.
package module

import (
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
)

const HostAPIVersion = 1

type Dependency struct {
	ID         string
	APIVersion int
}
type Manifest struct {
	ID         string       `json:"id"`
	Version    string       `json:"version"`
	APIVersion int          `json:"api_version"`
	Requires   []Dependency `json:"-"`
}
type Route struct {
	Method     string
	Path       string // Relative to /api/v1/<module ID>; literals and {parameters} only.
	Handler    http.Handler
	Public     bool // Must explicitly opt in to anonymous access.
	Permission string
}
type Definition struct {
	Manifest    Manifest
	Permissions []string
	Routes      []Route
}
type Endpoint struct {
	Method, Pattern string
	Handler         http.Handler
}

// Authorizer is supplied by the host, never by an untrusted module or the UI.
type Authorizer func(permission string, next http.Handler) http.Handler
type Registry struct {
	manifests []Manifest
	endpoints []Endpoint
}

var idPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
var versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
var routePattern = regexp.MustCompile(`^/([a-z0-9-]+|\{[a-z][a-z0-9_]*\})(/([a-z0-9-]+|\{[a-z][a-z0-9_]*\}))*$`)
var parameterPattern = regexp.MustCompile(`\{[^}]+\}`)

// New validates dependencies and routes before exposing any endpoint.
// Disabled dependencies are errors; activation never silently enables modules.
func New(definitions []Definition, enabled []string, authorize Authorizer) (*Registry, error) {
	available := make(map[string]Definition)
	for _, d := range definitions {
		m := d.Manifest
		if !idPattern.MatchString(m.ID) || !versionPattern.MatchString(m.Version) || m.APIVersion != HostAPIVersion {
			return nil, fmt.Errorf("invalid or incompatible module %q", m.ID)
		}
		if _, exists := available[m.ID]; exists {
			return nil, fmt.Errorf("duplicate module %q", m.ID)
		}
		available[m.ID] = d
	}
	active := make(map[string]bool)
	for _, id := range enabled {
		if _, ok := available[id]; !ok {
			return nil, fmt.Errorf("unknown enabled module %q", id)
		}
		if active[id] {
			return nil, fmt.Errorf("duplicate enabled module %q", id)
		}
		active[id] = true
	}
	order := []string{}
	state := make(map[string]int)
	var visit func(string) error
	visit = func(id string) error {
		if state[id] == 1 {
			return fmt.Errorf("module dependency cycle at %q", id)
		}
		if state[id] == 2 {
			return nil
		}
		state[id] = 1
		for _, dep := range available[id].Manifest.Requires {
			if !active[dep.ID] {
				return fmt.Errorf("module %q requires enabled module %q", id, dep.ID)
			}
			if available[dep.ID].Manifest.APIVersion != dep.APIVersion {
				return fmt.Errorf("incompatible dependency %q for %q", dep.ID, id)
			}
			if err := visit(dep.ID); err != nil {
				return err
			}
		}
		state[id] = 2
		order = append(order, id)
		return nil
	}
	ids := slices.Clone(enabled)
	slices.Sort(ids)
	for _, id := range ids {
		if err := visit(id); err != nil {
			return nil, err
		}
	}
	reg := &Registry{manifests: []Manifest{}, endpoints: []Endpoint{}}
	seen := make(map[string]bool)
	for _, id := range order {
		d := available[id]
		permissions := make(map[string]bool)
		for _, p := range d.Permissions {
			if !strings.HasPrefix(p, id+".") || len(p) <= len(id)+1 || permissions[p] {
				return nil, fmt.Errorf("invalid permission for %q", id)
			}
			permissions[p] = true
		}
		for _, route := range d.Routes {
			if !slices.Contains([]string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}, route.Method) || !routePattern.MatchString(route.Path) || route.Handler == nil {
				return nil, fmt.Errorf("invalid route for %q", id)
			}
			pattern := "/api/v1/" + id + route.Path
			key := route.Method + " " + parameterPattern.ReplaceAllString(pattern, "{}")
			if seen[key] {
				return nil, fmt.Errorf("duplicate route %q", key)
			}
			seen[key] = true
			handler := route.Handler
			if route.Public {
				if route.Permission != "" {
					return nil, fmt.Errorf("public route has permission in %q", id)
				}
			} else {
				if !permissions[route.Permission] || authorize == nil {
					return nil, fmt.Errorf("protected route has no permission enforcement in %q", id)
				}
				handler = authorize(route.Permission, handler)
				if handler == nil {
					return nil, fmt.Errorf("missing authorization handler in %q", id)
				}
			}
			reg.endpoints = append(reg.endpoints, Endpoint{route.Method, pattern, handler})
		}
		m := d.Manifest
		m.Requires = slices.Clone(m.Requires)
		reg.manifests = append(reg.manifests, m)
	}
	return reg, nil
}

func (r *Registry) Endpoints() []Endpoint { return slices.Clone(r.endpoints) }
func (r *Registry) Manifests() []Manifest {
	out := slices.Clone(r.manifests)
	for i := range out {
		out[i].Requires = slices.Clone(out[i].Requires)
	}
	return out
}

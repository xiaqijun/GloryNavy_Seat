package module

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func example(id string, deps ...Dependency) Definition {
	return Definition{Manifest: Manifest{ID: id, Version: "0.1.0", APIVersion: HostAPIVersion, Requires: deps}, Routes: []Route{{Method: "GET", Path: "/status", Public: true, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })}}}
}

func TestDependencyOrderAndDisabledRoutes(t *testing.T) {
	a, b, c := example("alpha", Dependency{"beta", 1}), example("beta"), example("disabled")
	registry, err := New([]Definition{c, a, b}, []string{"alpha", "beta"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := registry.Manifests(); len(got) != 2 || got[0].ID != "beta" || got[1].ID != "alpha" {
		t.Fatalf("dependency order: %v", got)
	}
	paths := []string{}
	for _, e := range registry.Endpoints() {
		paths = append(paths, e.Pattern)
	}
	if !reflect.DeepEqual(paths, []string{"/api/v1/beta/status", "/api/v1/alpha/status"}) {
		t.Fatalf("unexpected routes: %v", paths)
	}
	// The catalog is a snapshot, not a mutable reference to registry configuration.
	snapshot := registry.Manifests()
	snapshot[1].Requires[0].ID = "changed"
	if registry.Manifests()[1].Requires[0].ID != "beta" {
		t.Fatal("catalog mutation escaped")
	}
}

func TestInvalidModulesFailBeforeServing(t *testing.T) {
	cases := []struct {
		name        string
		definitions []Definition
		enabled     []string
	}{
		{"unknown", []Definition{example("one")}, []string{"missing"}},
		{"duplicate module", []Definition{example("one"), example("one")}, []string{"one"}},
		{"duplicate activation", []Definition{example("one")}, []string{"one", "one"}},
		{"disabled dependency", []Definition{example("one", Dependency{"two", 1}), example("two")}, []string{"one"}},
		{"missing dependency", []Definition{example("one", Dependency{"missing", 1})}, []string{"one"}},
		{"dependency version", []Definition{example("one", Dependency{"two", 2}), example("two")}, []string{"one", "two"}},
		{"cycle", []Definition{example("one", Dependency{"two", 1}), example("two", Dependency{"one", 1})}, []string{"one", "two"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if r, err := New(tc.definitions, tc.enabled, nil); err == nil || r != nil {
				t.Fatal("invalid module set accepted")
			}
		})
	}
	for _, tc := range []struct {
		name   string
		mutate func(*Definition)
	}{
		{"host version", func(d *Definition) { d.Manifest.APIVersion = 2 }},
		{"path escape", func(d *Definition) { d.Routes[0].Path = "/../auth" }},
		{"wildcard", func(d *Definition) { d.Routes[0].Path = "/*" }},
		{"method", func(d *Definition) { d.Routes[0].Method = "TRACE" }},
		{"nil handler", func(d *Definition) { d.Routes[0].Handler = nil }},
		{"public permission", func(d *Definition) { d.Routes[0].Permission = "one.read" }},
		{"foreign permission", func(d *Definition) { d.Permissions = []string{"two.read"} }},
		{"parameter collision", func(d *Definition) {
			r := d.Routes[0]
			r.Path = "/{id}"
			d.Routes = []Route{r}
			r.Path = "/{name}"
			d.Routes = append(d.Routes, r)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := example("one")
			tc.mutate(&d)
			if r, err := New([]Definition{d}, []string{"one"}, nil); err == nil || r != nil {
				t.Fatal("invalid declaration accepted")
			}
		})
	}
}

func TestProtectedRoutesRequireHostAuthorization(t *testing.T) {
	d := example("members")
	d.Routes[0].Public = false
	d.Routes[0].Permission = "members.read"
	d.Permissions = []string{"members.read"}
	if _, err := New([]Definition{d}, []string{"members"}, nil); err == nil {
		t.Fatal("missing authorizer accepted")
	}
	called := false
	d.Routes[0].Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })
	deny := func(permission string, next http.Handler) http.Handler {
		if permission != "members.read" {
			t.Errorf("wrong permission: %s", permission)
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) })
	}
	reg, err := New([]Definition{d}, []string{"members"}, deny)
	if err != nil {
		t.Fatal(err)
	}
	res := httptest.NewRecorder()
	reg.Endpoints()[0].Handler.ServeHTTP(res, httptest.NewRequest("GET", "/", nil))
	if res.Code != 403 || called {
		t.Fatal("protected handler bypassed authorizer")
	}
	d.Permissions = nil
	if _, err := New([]Definition{d}, []string{"members"}, deny); err == nil {
		t.Fatal("undeclared permission accepted")
	}
}

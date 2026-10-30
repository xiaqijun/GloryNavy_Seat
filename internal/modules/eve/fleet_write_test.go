package eve

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"glorynavy.local/seat/internal/httpapi"
)

func TestFleetAuthorizationAddsOnlyExplicitScope(t *testing.T) {
	got, err := fleetAuthorizationURL("https://login.eveonline.com/oauth/authorize?scope=esi-fleets.read_fleet.v1")
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	scopes := strings.Fields(u.Query().Get("scope"))
	if !hasScopes(scopes, []string{FleetReadScope, FleetWriteScope}) || len(scopes) != 2 {
		t.Fatal(scopes)
	}
	got, err = fleetAuthorizationURL(got)
	if err != nil {
		t.Fatal(err)
	}
	u, _ = url.Parse(got)
	if len(strings.Fields(u.Query().Get("scope"))) != 2 {
		t.Fatal(got)
	}
}

func TestFleetMOTDWriteRejectsChangedSnapshotAndUnknown(t *testing.T) {
	s, ch := syncFixture(t)
	ctx := context.Background()
	ch.authorization.Scopes = append(ch.authorization.Scopes, FleetReadScope, FleetWriteScope)
	if err := s.auth.Save(ctx, ch); err != nil {
		t.Fatal(err)
	}
	gen, ok, err := s.auth.FleetWriteGrant(ctx, ch.ID, httpapi.Hash(ch.Owner))
	if err != nil || !ok {
		t.Fatal(gen, ok, err)
	}
	motd := "before"
	puts := 0
	broken := false
	s.auth.esi.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		if _, e := s.pool.Exec(r.Context(), "UPDATE eve_esi_limits SET next_request_at='epoch'"); e != nil {
			t.Error(e)
		}
		switch r.URL.Path {
		case "/characters/123/fleet/":
			return esiResponse(map[string]any{"fleet_id": 999}), nil
		case "/fleets/999/members/":
			return esiResponse([]map[string]any{{"character_id": 123}}), nil
		case "/characters/affiliation/":
			return esiResponse([]map[string]any{{"character_id": 123, "corporation_id": 10}}), nil
		case "/universe/names/":
			return esiResponse([]map[string]any{{"id": 123, "name": "Pilot", "category": "character"}}), nil
		case "/fleets/999/":
			if r.Method == "GET" {
				return esiResponse(map[string]any{"motd": motd}), nil
			}
			puts++
			if broken {
				return nil, errors.New("response lost")
			}
			res := esiResponse(nil)
			res.StatusCode = 204
			return res, nil
		default:
			t.Errorf("unexpected %s", r.URL.Path)
			return &http.Response{StatusCode: 500, Header: http.Header{}, Body: http.NoBody}, nil
		}
	})
	clear := func() { _, _ = s.pool.Exec(ctx, `UPDATE eve_esi_limits SET next_request_at=now()-interval '1 second'`) }
	clear()
	if _, _, err := s.auth.ReadFleetMOTD(ctx, ch.ID, httpapi.Hash(ch.Owner)); err != nil {
		t.Fatalf("initial read: %v", err)
	}
	clear()
	result := s.auth.UpdateFleetMOTD(ctx, ch.ID, httpapi.Hash(ch.Owner), gen, 999, "wrong", "new")
	if result.Reason != "fleet_changed" || puts != 0 {
		t.Fatal(result, puts)
	}
	clear()
	result = s.auth.UpdateFleetMOTD(ctx, ch.ID, httpapi.Hash(ch.Owner), gen, 999, "before", "new")
	if result.State != "sent" || puts != 1 {
		t.Fatal(result, puts)
	}
	broken = true
	clear()
	result = s.auth.UpdateFleetMOTD(ctx, ch.ID, httpapi.Hash(ch.Owner), gen, 999, "before", "new")
	if result.State != "unknown" || puts != 2 {
		t.Fatal(result, puts)
	}
}

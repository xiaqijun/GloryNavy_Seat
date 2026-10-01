package eve

import (
	"context"
	"errors"
	"glorynavy.local/seat/internal/httpapi"
	"net/http"
	"testing"
)

func TestFittingWriteScopeNoCacheAndUncertainResult(t *testing.T) {
	s, ch := syncFixture(t)
	ctx := context.Background()
	ch.authorization.Scopes = []string{CorporationRolesScope, FittingsReadScope, FittingsWriteScope}
	if e := s.auth.Save(ctx, ch); e != nil {
		t.Fatal(e)
	}
	calls := 0
	broken := false
	s.auth.esi.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "POST" || r.Header.Get("If-None-Match") != "" {
			t.Fatal("write routed as cached read")
		}
		if broken {
			return nil, errors.New("response lost")
		}
		v := jsonResponse(map[string]int64{"fitting_id": 999})
		v.StatusCode = 201
		return v, nil
	})
	fit := GameFitting{Name: "Test", ShipTypeID: 587, Items: []SavedFittingItem{{TypeID: 2048, Flag: "LoSlot0", Quantity: 1}}}
	for i := 0; i < 2; i++ {
		if _, e := s.pool.Exec(ctx, "UPDATE eve_esi_limits SET next_request_at=now()-interval '1 second'"); e != nil {
			t.Fatal(e)
		}
		r := s.auth.SaveGameFitting(ctx, ch.ID, httpapi.Hash(ch.Owner), fit)
		if r.State != "saved" || r.ID != 999 {
			t.Fatal(r)
		}
	}
	if calls != 2 {
		t.Fatal("POST response reused from cache", calls)
	}
	broken = true
	s.pool.Exec(ctx, "UPDATE eve_esi_limits SET next_request_at=now()-interval '1 second'")
	r := s.auth.SaveGameFitting(ctx, ch.ID, httpapi.Hash(ch.Owner), fit)
	if r.State != "unknown" || calls != 3 {
		t.Fatal("uncertain request retried", r, calls)
	}
	ch.authorization.Scopes = []string{CorporationRolesScope, FittingsReadScope}
	if e := s.auth.Save(ctx, ch); e != nil {
		t.Fatal(e)
	}
	r = s.auth.SaveGameFitting(ctx, ch.ID, httpapi.Hash(ch.Owner), fit)
	if r.Reason != "missing_scope" || calls != 3 {
		t.Fatal(r, calls)
	}
}

func TestFittingWriteScopeIsExplicitAddition(t *testing.T) {
	c := &Client{}
	c.EnableSeatDefaultScopes()
	if len(c.RequestedScopes()) != 57 {
		t.Fatal("baseline changed")
	}
	c.EnableFittingsWriteScope()
	c.EnableFittingsWriteScope()
	scopes := c.RequestedScopes()
	if len(scopes) != 58 || !hasScopes(scopes, []string{FittingsWriteScope}) {
		t.Fatal("missing or duplicated write scope")
	}
}

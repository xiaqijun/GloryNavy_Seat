package eve

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"glorynavy.local/seat/internal/httpapi"
)

func TestContractStructureNamesScopeCacheAndRevocation(t *testing.T) {
	s, ch := syncFixture(t)
	ctx := context.Background()
	ch.authorization.Scopes = append(ch.authorization.Scopes, structureReadScope)
	if err := s.auth.Save(ctx, ch); err != nil {
		t.Fatal(err)
	}
	h := NewContractHTTP(s.pool, s.auth, nil)
	bound := true
	h.BindingOwner = func(context.Context, int64) ([]byte, error) {
		if !bound {
			return nil, fmt.Errorf("unbound")
		}
		return httpapi.Hash(ch.Owner), nil
	}
	var calls atomic.Int32
	h.esi.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer test-access" || r.URL.Path != "/universe/structures/1030000000001/" {
			t.Errorf("incorrect scope or endpoint: %s", r.URL.Path)
		}
		return esiResponse(map[string]any{"name": "GNV 测试建筑"}), nil
	})
	lookup := func(owner ContractOwner) []contractView {
		rows := []contractView{{Start: entity(1030000000001, "station"), End: entity(1030000000001, "station")}}
		h.decorate(ctx, owner, rows)
		return rows
	}
	owner := ContractOwner{Kind: "character", ID: ch.ID}
	for range 2 {
		rows := lookup(owner)
		if rows[0].Start.Name != "GNV 测试建筑" || rows[0].End.Name != rows[0].Start.Name || rows[0].Start.Category != "structure" {
			t.Fatalf("name/category missing: %+v", rows)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("dedup/cache missed: %d", calls.Load())
	}
	var shared int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM eve_entity_names WHERE entity_id=1030000000001`).Scan(&shared); err != nil || shared != 0 {
		t.Fatal("private name leaked into public table", err)
	}
	bound = false
	if rows := lookup(owner); rows[0].Start.Name != "" {
		t.Fatal("unbound character received cached private name")
	}
	bound = true
	if rows := lookup(ContractOwner{Kind: "character", ID: 999}); rows[0].Start.Name != "" {
		t.Fatal("other character received private name")
	}
	if rows := lookup(ContractOwner{Kind: "corporation", ID: 789}); rows[0].Start.Name != "" {
		t.Fatal("corporation without source received name")
	}
	ch.authorization.Scopes = []string{CorporationRolesScope}
	if err := s.auth.Save(ctx, ch); err != nil {
		t.Fatal(err)
	}
	if rows := lookup(owner); rows[0].Start.Name != "" {
		t.Fatal("removed scope received old cached name")
	}
	ch.authorization.Scopes = append(ch.authorization.Scopes, structureReadScope)
	if err := s.auth.Save(ctx, ch); err != nil {
		t.Fatal(err)
	}
	if rows := lookup(owner); rows[0].Start.Name == "" {
		t.Fatal("reauthorized lookup failed")
	}
	if calls.Load() != 2 {
		t.Fatal("new grant reused old private cache")
	}
}

func TestContractLocationFailureIsolationAndCooldown(t *testing.T) {
	s, ch := syncFixture(t)
	ctx := context.Background()
	ch.authorization.Scopes = append(ch.authorization.Scopes, structureReadScope)
	if err := s.auth.Save(ctx, ch); err != nil {
		t.Fatal(err)
	}
	h := NewContractHTTP(s.pool, s.auth, nil)
	h.BindingOwner = func(context.Context, int64) ([]byte, error) { return httpapi.Hash(ch.Owner), nil }
	var privateCalls atomic.Int32
	h.esi.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "/structures/") {
			privateCalls.Add(1)
			res := esiResponse(map[string]string{"error": "Forbidden"})
			res.StatusCode = 403
			return res, nil
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("public location used bearer token")
		}
		if strings.Contains(r.URL.Path, "/stations/") {
			return esiResponse(map[string]string{"name": "Jita IV - Moon 4"}), nil
		}
		var ids []int64
		_ = json.NewDecoder(r.Body).Decode(&ids)
		// One invalid party or public location must not hide another station.
		res := esiResponse(map[string]string{"error": "not found"})
		res.StatusCode = 404
		return res, nil
	})
	for range 2 {
		rows := []contractView{{Issuer: entity(999999, "character"), Start: entity(60003760, "station"), End: entity(1030000000002, "station")}}
		h.decorate(ctx, ContractOwner{Kind: "character", ID: ch.ID}, rows)
		if rows[0].Start.Name != "Jita IV - Moon 4" || rows[0].End.Name != "" || rows[0].End.ID != "1030000000002" {
			t.Fatalf("failure corrupted names/IDs: %+v", rows)
		}
	}
	if privateCalls.Load() != 1 {
		t.Fatalf("denied structure repeatedly requested: %d", privateCalls.Load())
	}
}

func TestContractLocationRevokedDuringRequest(t *testing.T) {
	s, ch := syncFixture(t)
	ctx := context.Background()
	ch.authorization.Scopes = append(ch.authorization.Scopes, structureReadScope)
	if err := s.auth.Save(ctx, ch); err != nil {
		t.Fatal(err)
	}
	h := NewContractHTTP(s.pool, s.auth, nil)
	var bound atomic.Bool
	bound.Store(true)
	h.BindingOwner = func(context.Context, int64) ([]byte, error) {
		if !bound.Load() {
			return nil, fmt.Errorf("unbound")
		}
		return httpapi.Hash(ch.Owner), nil
	}
	h.esi.http.Transport = transportFunc(func(*http.Request) (*http.Response, error) {
		bound.Store(false)
		return esiResponse(map[string]string{"name": "Do not expose"}), nil
	})
	refs := []*contractEntity{new(contractEntity)}
	*refs[0] = entity(1030000000001, "station")
	h.contractLocations(ctx, ContractOwner{Kind: "character", ID: ch.ID}, refs)
	if refs[0].Name != "" {
		t.Fatal("binding changed during request")
	}
}

func TestContractCorporationLocationSourceExpiry(t *testing.T) {
	s, ch := contractFixture(t)
	ctx := context.Background()
	ch.authorization.Scopes = append(ch.authorization.Scopes, structureReadScope)
	if err := s.auth.Save(ctx, ch); err != nil {
		t.Fatal(err)
	}
	_, err := s.pool.Exec(ctx, `INSERT INTO eve_role_snapshots(character_id,owner_hash,corporation_id,corporation_name,ceo_id,roles,roles_at_hq,roles_at_base,roles_at_other,synced_at,valid_until)
 SELECT character_id,owner_hash,10,'Corp',999,'{}','{}','{}','{}',now(),now()+interval '1 hour' FROM eve_credentials WHERE character_id=$1`, ch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.pool.Exec(ctx, `UPDATE eve_credentials SET state='ready',roles_not_before=NULL WHERE character_id=$1`, ch.ID); err != nil {
		t.Fatal(err)
	}
	h := NewContractHTTP(s.pool, s.auth, nil)
	h.BindingOwner = func(context.Context, int64) ([]byte, error) { return httpapi.Hash(ch.Owner), nil }
	var calls atomic.Int32
	h.esi.http.Transport = transportFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return esiResponse(map[string]string{"name": "Corporation source structure"}), nil
	})
	lookup := func() string {
		rows := []contractView{{Start: entity(1030000000001, "station")}}
		h.decorate(ctx, ContractOwner{Kind: "corporation", ID: 10}, rows)
		return rows[0].Start.Name
	}
	if lookup() == "" {
		_, sourceErr := h.locationSource(ctx, ContractOwner{Kind: "corporation", ID: 10})
		var targetState string
		_ = s.pool.QueryRow(ctx, `SELECT state||':'||reason FROM eve_sync_targets WHERE character_id=$1 AND resource='corporation_contracts'`, ch.ID).Scan(&targetState)
		t.Fatalf("current corporation source was not used: source=%v target=%s calls=%d", sourceErr, targetState, calls.Load())
	}
	if _, err = s.pool.Exec(ctx, `UPDATE eve_role_snapshots SET valid_until=now()-interval '1 minute' WHERE character_id=$1`, ch.ID); err != nil {
		t.Fatal(err)
	}
	if lookup() != "" || calls.Load() != 1 {
		t.Fatal("expired corporation source reused private name")
	}
}

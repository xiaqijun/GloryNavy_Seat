package eve

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"glorynavy.local/seat/internal/modules/eve/internal/store"
)

func contractTarget(t *testing.T, s *SyncService, id int64, resource string) store.EveSyncTarget {
	t.Helper()
	rows, e := store.New(s.pool).ListCharacterSync(context.Background(), id)
	if e != nil {
		t.Fatal(e)
	}
	for _, r := range rows {
		if r.Resource == resource {
			return r
		}
	}
	t.Fatal("contract target missing")
	return store.EveSyncTarget{}
}
func contractJSON(id int, typ string) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{"contract_id":%d,"issuer_id":123,"issuer_corporation_id":10,"assignee_id":0,"acceptor_id":0,"type":%q,"status":"outstanding","availability":"public","for_corporation":false,"date_issued":"2026-09-14T00:00:00Z","date_expired":"2026-09-20T00:00:00Z","price":123456789012345.67}`, id, typ))
}
func contractFixture(t *testing.T) (*SyncService, Character) {
	s, ch := syncFixture(t)
	ch.authorization.Scopes = append(ch.authorization.Scopes, characterContractsScope, corporationContractsScope)
	if e := s.auth.Save(context.Background(), ch); e != nil {
		t.Fatal(e)
	}
	return s, ch
}
func contractTransport(t *testing.T, s *SyncService, fn func(*http.Request) *http.Response) {
	t.Helper()
	s.auth.esi.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		// Rate-limit mechanics are tested separately. Keep pagination tests deterministic.
		if _, e := s.pool.Exec(r.Context(), "UPDATE eve_esi_limits SET next_request_at='epoch'"); e != nil {
			t.Error(e)
		}
		return fn(r), nil
	})
}
func contractListResponse(rows []json.RawMessage, pages string) *http.Response {
	r := esiResponse(rows)
	r.Header.Set("X-Pages", pages)
	return r
}

func TestContractsPaginationDetailsPrecisionAndReplay(t *testing.T) {
	s, ch := contractFixture(t)
	ctx := context.Background()
	pages := []string{}
	detailCalls := 0
	contractTransport(t, s, func(r *http.Request) *http.Response {
		if strings.HasSuffix(r.URL.Path, "/items/") {
			detailCalls++
			return jsonResponse(json.RawMessage(`[{"record_id":100,"type_id":34,"quantity":1,"is_included":true,"is_singleton":true,"raw_quantity":-2}]`))
		}
		if strings.HasSuffix(r.URL.Path, "/bids/") {
			detailCalls++
			return jsonResponse(json.RawMessage(`[{"bid_id":1,"bidder_id":555,"date_bid":"2026-09-14T00:00:00Z","amount":123456789012345.67}]`))
		}
		pages = append(pages, r.URL.Query().Get("page"))
		if r.URL.Query().Get("page") == "1" {
			return contractListResponse([]json.RawMessage{contractJSON(1, "auction")}, "2")
		}
		return contractListResponse([]json.RawMessage{contractJSON(2, "courier")}, "2")
	})
	target := contractTarget(t, s, ch.ID, "character_contracts")
	args := syncArgs{target.ID, target.Generation}
	if e := s.work(ctx, args, target.ActiveJobID.Int64, target.Resource); e == nil {
		t.Fatal("first page should continue")
	}
	cursor, e := store.New(s.pool).GetContractCursor(ctx, target.ID)
	if e != nil || cursor.Page != 2 {
		t.Fatal("cursor not committed", e, cursor)
	}
	first := contractTarget(t, s, ch.ID, target.Resource)
	if first.LastSuccessAt.Valid {
		t.Fatal("partial list reported complete")
	}
	var priority int
	if e = s.pool.QueryRow(ctx, "SELECT min(priority) FROM river_job WHERE kind='eve.contract-detail.v1'").Scan(&priority); e != nil || priority != 3 {
		t.Fatal("details could block list pagination", priority, e)
	}
	if _, e = s.pool.Exec(ctx, "UPDATE eve_sync_targets SET next_due_at=now() WHERE id=$1", target.ID); e != nil {
		t.Fatal(e)
	}
	restarted, e := NewSync(s.pool, s.auth, s.logger, nil)
	if e != nil {
		t.Fatal(e)
	}
	if e = restarted.work(ctx, args, target.ActiveJobID.Int64, target.Resource); e != nil {
		t.Fatal(e)
	}
	done := contractTarget(t, s, ch.ID, target.Resource)
	if !done.LastSuccessAt.Valid || len(pages) != 2 || pages[1] != "2" {
		t.Fatal("did not resume second page", pages)
	}
	for _, part := range []string{"items", "bids"} {
		a := contractDetailArgs{"character", 123, 1, part, 123, target.Generation}
		if e = s.workContractDetail(ctx, a, 1); e != nil {
			t.Fatal(e)
		}
		if e = s.workContractDetail(ctx, a, 1); e != nil {
			t.Fatal(e)
		}
	}
	if detailCalls != 2 {
		t.Fatal("replayed detail called ESI", detailCalls)
	}
	var amount, price string
	var rawQuantity int64
	if e = s.pool.QueryRow(ctx, "SELECT amount::text FROM eve_contract_bids").Scan(&amount); e != nil || amount != "123456789012345.67" {
		t.Fatal("ISK precision lost", amount, e)
	}
	if e = s.pool.QueryRow(ctx, "SELECT payload->>'price' FROM eve_contracts WHERE contract_id=1").Scan(&price); e != nil || price != "123456789012345.67" {
		t.Fatal("contract price lost", price, e)
	}
	if e = s.pool.QueryRow(ctx, "SELECT raw_quantity FROM eve_contract_items").Scan(&rawQuantity); e != nil || rawQuantity != -2 {
		t.Fatal("blueprint semantics lost", rawQuantity, e)
	}
	var courierDetails int
	if e = s.pool.QueryRow(ctx, "SELECT count(*) FROM eve_contract_details WHERE contract_id=2").Scan(&courierDetails); e != nil || courierDetails != 0 {
		t.Fatal("courier items scheduled", e)
	}
}

func TestContractsMissingPagesDoesNotPublishAndHistorySurvives(t *testing.T) {
	s, ch := contractFixture(t)
	ctx := context.Background()
	phase := 0
	contractTransport(t, s, func(r *http.Request) *http.Response {
		switch phase {
		case 0:
			return contractListResponse([]json.RawMessage{contractJSON(1, "courier")}, "")
		case 1:
			return contractListResponse([]json.RawMessage{contractJSON(1, "courier")}, "1")
		default:
			return contractListResponse([]json.RawMessage{}, "1")
		}
	})
	target := contractTarget(t, s, ch.ID, "character_contracts")
	args := syncArgs{target.ID, target.Generation}
	_ = s.work(ctx, args, target.ActiveJobID.Int64, target.Resource)
	var count int
	_ = s.pool.QueryRow(ctx, "SELECT count(*) FROM eve_contracts").Scan(&count)
	if count != 0 {
		t.Fatal("published missing-page response")
	}
	phase = 1
	_, _ = s.pool.Exec(ctx, "UPDATE eve_sync_targets SET next_due_at=now() WHERE id=$1", target.ID)
	// Invalid metadata is never cached.
	if e := s.work(ctx, args, target.ActiveJobID.Int64, target.Resource); e != nil {
		t.Fatal(e)
	}
	phase = 2
	_, _ = s.pool.Exec(ctx, "UPDATE eve_esi_cache SET expires_at=now()-interval '1 second'; UPDATE eve_sync_targets SET next_due_at=now() WHERE resource='character_contracts'")
	if e := s.dispatch(ctx); e != nil {
		t.Fatal(e)
	}
	target = contractTarget(t, s, ch.ID, target.Resource)
	if e := s.work(ctx, syncArgs{target.ID, target.Generation}, target.ActiveJobID.Int64, target.Resource); e != nil {
		t.Fatal(e)
	}
	_ = s.pool.QueryRow(ctx, "SELECT count(*) FROM eve_contracts").Scan(&count)
	if count != 1 {
		t.Fatal("upstream window removed local history", count)
	}
}

func setContractCorporation(t *testing.T, s *SyncService, ch Character, corp int64) {
	t.Helper()
	ctx := context.Background()
	_, e := s.pool.Exec(ctx, `UPDATE eve_credentials SET state='ready' WHERE character_id=$1`, ch.ID)
	if e != nil {
		t.Fatal(e)
	}
	c, e := store.New(s.pool).GetCredential(ctx, ch.ID)
	if e != nil {
		t.Fatal(e)
	}
	e = store.New(s.pool).SaveSnapshot(ctx, store.SaveSnapshotParams{CharacterID: ch.ID, OwnerHash: c.OwnerHash, CorporationID: corp, CorporationName: "Corp", CeoID: 999, Roles: []string{}, RolesAtHq: []string{}, RolesAtBase: []string{}, RolesAtOther: []string{}, SyncedAt: timestamp(time.Now()), ValidUntil: timestamp(time.Now().Add(time.Hour))})
	if e != nil {
		t.Fatal(e)
	}
}
func TestCorporationContractsSelectSourceAndRejectChangedAffiliation(t *testing.T) {
	s, ch := contractFixture(t)
	ctx := context.Background()
	setContractCorporation(t, s, ch, 10)
	second := ch
	second.ID = 456
	second.Name = "Second"
	if e := s.auth.Save(ctx, second); e != nil {
		t.Fatal(e)
	}
	setContractCorporation(t, s, second, 10)
	requests := 0
	contractTransport(t, s, func(r *http.Request) *http.Response {
		requests++
		if !strings.HasPrefix(r.URL.Path, "/corporations/10/") {
			t.Error("wrong corporation requested")
		}
		return contractListResponse([]json.RawMessage{json.RawMessage(strings.Replace(string(contractJSON(1, "courier")), `"for_corporation":false`, `"for_corporation":true`, 1))}, "1")
	})
	t2 := contractTarget(t, s, 456, "corporation_contracts")
	_ = s.work(ctx, syncArgs{t2.ID, t2.Generation}, t2.ActiveJobID.Int64, t2.Resource)
	if requests != 0 {
		t.Fatal("duplicate corporation source fetched")
	}
	t1 := contractTarget(t, s, 123, "corporation_contracts")
	if e := s.work(ctx, syncArgs{t1.ID, t1.Generation}, t1.ActiveJobID.Int64, t1.Resource); e != nil {
		t.Fatal(e)
	}
	var owner int64
	if e := s.pool.QueryRow(ctx, "SELECT owner_id FROM eve_contracts WHERE owner_kind='corporation'").Scan(&owner); e != nil || owner != 10 {
		t.Fatal("corporation ownership missing", e)
	}
	_, _ = s.pool.Exec(ctx, "UPDATE eve_sync_targets SET state='blocked' WHERE character_id=123 AND resource='corporation_contracts'; UPDATE eve_sync_targets SET next_due_at=now() WHERE character_id=456 AND resource='corporation_contracts'")
	contractTransport(t, s, func(r *http.Request) *http.Response {
		setContractCorporation(t, s, second, 20)
		return contractListResponse([]json.RawMessage{contractJSON(2, "courier")}, "1")
	})
	_ = s.work(ctx, syncArgs{t2.ID, t2.Generation}, t2.ActiveJobID.Int64, t2.Resource)
	var count int
	_ = s.pool.QueryRow(ctx, "SELECT count(*) FROM eve_contracts WHERE contract_id=2").Scan(&count)
	if count != 0 {
		t.Fatal("departed source published old corporation data")
	}
}

func TestContractDetailReauthorizationFencesOldResponse(t *testing.T) {
	s, ch := contractFixture(t)
	ctx := context.Background()
	target := contractTarget(t, s, ch.ID, "character_contracts")
	contractTransport(t, s, func(r *http.Request) *http.Response {
		if strings.HasSuffix(r.URL.Path, "/items/") {
			if e := s.auth.Save(ctx, ch); e != nil {
				t.Fatal(e)
			}
			return jsonResponse(json.RawMessage(`[{"record_id":100,"type_id":34,"quantity":1,"is_included":true,"is_singleton":false}]`))
		}
		return contractListResponse([]json.RawMessage{contractJSON(1, "item_exchange")}, "1")
	})
	if e := s.work(ctx, syncArgs{target.ID, target.Generation}, target.ActiveJobID.Int64, target.Resource); e != nil {
		t.Fatal(e)
	}
	_ = s.workContractDetail(ctx, contractDetailArgs{"character", 123, 1, "items", 123, target.Generation}, 1)
	var count int
	_ = s.pool.QueryRow(ctx, "SELECT count(*) FROM eve_contract_items").Scan(&count)
	if count != 0 {
		t.Fatal("old generation published items")
	}
	_, e := s.pool.Exec(ctx, "UPDATE eve_contract_details SET page=3,pages=4")
	if e != nil {
		t.Fatal(e)
	}
	nextTarget := contractTarget(t, s, ch.ID, "character_contracts")
	c, e := store.New(s.pool).GetCredential(ctx, ch.ID)
	if e != nil {
		t.Fatal(e)
	}
	result, e := s.collectContracts(ctx, nextTarget, c)
	if e != nil {
		t.Fatal(e)
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	if e = s.saveContractPage(ctx, tx, nextTarget, c, result.contracts); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	d, e := store.New(s.pool).GetContractDetail(ctx, store.GetContractDetailParams{OwnerKind: "character", OwnerID: 123, ContractID: 1, Part: "items"})
	if e != nil || d.Page != 1 || d.Pages != 0 {
		t.Fatal("new grant resumed old detail pages", d, e)
	}
}

func TestContractPageCountSurvivesCacheAnd304(t *testing.T) {
	s, ch := contractFixture(t)
	ctx := context.Background()
	c, e := store.New(s.pool).GetCredential(ctx, ch.ID)
	if e != nil {
		t.Fatal(e)
	}
	calls := 0
	contractTransport(t, s, func(*http.Request) *http.Response {
		calls++
		if calls == 1 {
			r := contractListResponse([]json.RawMessage{}, "3")
			r.Header.Set("ETag", "contracts-v1")
			return r
		}
		r := esiResponse(nil)
		r.StatusCode = 304
		return r
	})
	request := func() {
		var rows []json.RawMessage
		response, e := s.auth.esi.Request(ctx, ESIRequest{Method: "GET", Path: "/characters/123/contracts/?page=1", CharacterID: c.CharacterID, Generation: c.GrantGeneration, Scopes: []string{characterContractsScope}, ExpectPages: true}, &rows)
		if e != nil {
			t.Fatal(e)
		}
		if response.Pages != 3 {
			t.Fatal("lost cached page count", response.Pages)
		}
	}
	request()
	request()
	if calls != 1 {
		t.Fatal("cache bypassed")
	}
	if _, e = s.pool.Exec(ctx, "UPDATE eve_esi_cache SET expires_at=now()-interval '1 second'"); e != nil {
		t.Fatal(e)
	}
	request()
	if calls != 2 {
		t.Fatal("304 not requested")
	}
}

func TestContractDetails403PreservesCredentialsAndSnapshot(t *testing.T) {
	s, ch := contractFixture(t)
	ctx := context.Background()
	setContractCorporation(t, s, ch, 10)
	target := contractTarget(t, s, ch.ID, "character_contracts")
	contractTransport(t, s, func(r *http.Request) *http.Response {
		if strings.HasSuffix(r.URL.Path, "/items/") {
			response := jsonResponse(map[string]string{"error": "denied"})
			response.StatusCode = 403
			return response
		}
		return contractListResponse([]json.RawMessage{contractJSON(1, "item_exchange")}, "1")
	})
	if e := s.work(ctx, syncArgs{target.ID, target.Generation}, target.ActiveJobID.Int64, target.Resource); e != nil {
		t.Fatal(e)
	}
	if e := s.workContractDetail(ctx, contractDetailArgs{"character", ch.ID, 1, "items", ch.ID, target.Generation}, 1); e != nil {
		t.Fatal(e)
	}
	c, e := store.New(s.pool).GetCredential(ctx, ch.ID)
	if e != nil || len(c.Sealed) == 0 || c.State == "reauthorize" {
		t.Fatal("403 destroyed credential", e)
	}
	fact, e := s.auth.Get(ctx, ch.ID)
	if e != nil || fact.CorporationID != 10 {
		t.Fatal("403 invalidated unrelated authorization", e)
	}
	stats, e := store.New(s.pool).ContractDetailStats(ctx, target.ID)
	if e != nil || stats.Failed != 1 {
		t.Fatal("detail error not visible", stats, e)
	}
}

package eve

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
	"glorynavy.local/seat/internal/testutil"
)

func TestContractReadIsolationPaginationAndPrecision(t *testing.T) {
	pool := testutil.Database(t)
	ctx := context.Background()
	q := store.New(pool)
	for _, owner := range []string{"character", "corporation"} {
		for i := 1; i <= 30; i++ {
			p := strings.Replace(string(contractJSON(i, "item_exchange")), `"contract_id":`, `"title":"运输物资", "contract_id":`, 1)
			if owner == "corporation" {
				p = strings.Replace(p, `"assignee_id":0`, `"assignee_id":123`, 1)
			}
			if _, e := q.UpsertContract(ctx, store.UpsertContractParams{OwnerKind: owner, OwnerID: 123, ContractID: int64(i), SourceCharacterID: 123, SourceGeneration: 1, ContractType: "item_exchange", Status: "outstanding", Payload: []byte(p)}); e != nil {
				t.Fatal(e)
			}
		}
	}
	if e := q.InsertContractItems(ctx, store.InsertContractItemsParams{OwnerKind: "character", OwnerID: 123, ContractID: 30, Items: []byte(`[{"record_id":1,"type_id":34,"quantity":9007199254740993,"is_included":false,"is_singleton":true,"raw_quantity":-2}]`)}); e != nil {
		t.Fatal(e)
	}
	if e := q.InsertContractBids(ctx, store.InsertContractBidsParams{OwnerKind: "character", OwnerID: 123, ContractID: 30, Bids: []byte(`[{"bid_id":1,"bidder_id":234,"amount":123456789012345.67,"date_bid":"2026-09-14T00:00:00Z"}]`)}); e != nil {
		t.Fatal(e)
	}
	_ = q.SaveEntityName(ctx, store.SaveEntityNameParams{EntityID: 34, Name: "三钛合金", Category: "inventory_type", Language: "zh"})
	allowed := true
	h := NewContractHTTP(pool, nil, func(*http.Request) string { return "member" })
	h.Owners = func(context.Context, string) ([]ContractOwner, error) {
		if !allowed {
			return []ContractOwner{}, nil
		}
		return []ContractOwner{{Kind: "character", ID: 123, Name: "Pilot"}}, nil
	}
	mux := chi.NewRouter()
	for _, route := range h.Routes() {
		mux.Method(route.Method, route.Path, route.Handler)
	}
	get := func(path string, code int) string {
		t.Helper()
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != code {
			t.Fatalf("%s got %d: %s", path, w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	first := get("/contracts/character/123", 200)
	var list struct {
		Data struct {
			Items []contractView `json:"items"`
			Next  string         `json:"next_cursor"`
		}
	}
	if e := json.Unmarshal([]byte(first), &list); e != nil {
		t.Fatal(e)
	}
	if len(list.Data.Items) != 25 || list.Data.Next != "6" || *list.Data.Items[0].Price != "123456789012345.67" {
		t.Fatal("pagination or precision mismatch")
	}
	second := get("/contracts/character/123?before=6", 200)
	if strings.Contains(second, `"id":"30"`) {
		t.Fatal("cursor replayed page")
	}
	for _, path := range []string{"/contracts/character/999", "/contracts/corporation/123", "/contracts/corporation/123/30", "/contracts/corporation/123/30/items", "/contracts/character/999/30/bids"} {
		get(path, 404)
	}
	get("/contracts/character/123?before=-1", 400)
	get("/contracts/character/123?status=garbage", 400)
	if !strings.Contains(get("/contracts/character/123/30/items", 200), `"quantity":"9007199254740993"`) {
		t.Fatal("quantity lost precision")
	}
	if !strings.Contains(get("/contracts/character/123/30/items", 200), "三钛合金") {
		t.Fatal("cached name missing")
	}
	if !strings.Contains(get("/contracts/character/123/30/bids", 200), `"amount":"123456789012345.67"`) {
		t.Fatal("bid lost precision")
	}
	get("/contracts/character/123/30", 200)
	allowed = false
	for _, suffix := range []string{"", "/30", "/30/items", "/30/bids"} {
		get("/contracts/character/123"+suffix, 404)
	}
}

func TestContractItemReadPaginationAndNameFailure(t *testing.T) {
	pool := testutil.Database(t)
	ctx := context.Background()
	q := store.New(pool)
	if _, e := q.UpsertContract(ctx, store.UpsertContractParams{OwnerKind: "character", OwnerID: 123, ContractID: 1, ContractType: "item_exchange", Status: "outstanding", Payload: contractJSON(1, "item_exchange")}); e != nil {
		t.Fatal(e)
	}
	records := []string{}
	for i := 1; i <= 101; i++ {
		records = append(records, fmt.Sprintf(`{"record_id":%d,"type_id":34,"quantity":1,"is_included":true,"is_singleton":false}`, i))
	}
	if e := q.InsertContractItems(ctx, store.InsertContractItemsParams{OwnerKind: "character", OwnerID: 123, ContractID: 1, Items: []byte("[" + strings.Join(records, ",") + "]")}); e != nil {
		t.Fatal(e)
	}
	h := NewContractHTTP(pool, nil, func(*http.Request) string { return "member" })
	h.esi = newESI(testutil.Database(t), nil)
	calls := 0
	h.esi.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("Authorization") != "" {
			t.Fatal("public names used token")
		}
		return nil, fmt.Errorf("offline")
	})
	h.Owners = func(context.Context, string) ([]ContractOwner, error) {
		return []ContractOwner{{Kind: "character", ID: 123}}, nil
	}
	mux := chi.NewRouter()
	for _, r := range h.Routes() {
		mux.Method(r.Method, r.Path, r.Handler)
	}
	for _, tc := range []struct {
		cursor string
		count  int
		next   string
	}{{"", 100, "100"}, {"100", 1, ""}} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", "/contracts/character/123/1/items?after="+tc.cursor, nil))
		var body struct {
			Data struct {
				Items []json.RawMessage `json:"items"`
				Next  string            `json:"next_cursor"`
			}
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &body) != nil || len(body.Data.Items) != tc.count || body.Data.Next != tc.next {
			t.Fatal("item pagination failed", w.Body.String())
		}
	}
	if calls != 0 {
		t.Fatal("item rendering must not fetch ESI names")
	}
}

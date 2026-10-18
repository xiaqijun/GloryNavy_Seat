package eve

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
)

func TestContractSearchItemsPartiesKeywordsAndScope(t *testing.T) {
	s, ch := syncFixture(t)
	ctx := context.Background()
	if err := s.auth.Save(ctx, ch); err != nil {
		t.Fatal(err)
	}
	target := authorizationTarget(t, s)
	q := store.New(s.pool)
	if _, err := ImportSDENames(ctx, s.pool, sdeFixture(t, 100, sampleTypes), 100); err != nil {
		t.Fatal(err)
	}
	for _, n := range []store.SaveEntityNameParams{{EntityID: 456, Name: "Borrower Pilot", Category: "character", Language: "en"}, {EntityID: 600001, Name: "Jita Trade Hub", Category: "station", Language: "en"}, {EntityID: 1000000000001, Name: "Private Hangar", Category: "structure", Language: "en"}} {
		if err := q.SaveEntityName(ctx, n); err != nil {
			t.Fatal(err)
		}
	}
	// Search may use a previously resolved public name; it never resolves private locations.
	if _, err := s.pool.Exec(ctx, `UPDATE eve_entity_names SET expires_at=now()-interval '1 day'`); err != nil {
		t.Fatal(err)
	}
	seed := func(owner string, ownerID, id int64, title, state, kind string, types ...int) {
		t.Helper()
		raw := map[string]any{}
		if err := json.Unmarshal(contractJSON(int(id), kind), &raw); err != nil {
			t.Fatal(err)
		}
		raw["title"] = title
		raw["issuer_id"] = 111
		raw["issuer_corporation_id"] = 222
		raw["assignee_id"] = 0
		raw["acceptor_id"] = 0
		raw["start_location_id"] = 0
		raw["end_location_id"] = 0
		if id == 901 {
			raw["assignee_id"] = 456
			raw["start_location_id"] = 600001
			raw["end_location_id"] = 1000000000001
		}
		if owner == "corporation" && id != 990 {
			raw["assignee_id"] = ownerID
		}
		if id == 902 {
			delete(raw, "assignee_id")
			delete(raw, "acceptor_id")
		}
		payload, _ := json.Marshal(raw)
		if _, err := q.UpsertContract(ctx, store.UpsertContractParams{OwnerKind: owner, OwnerID: ownerID, ContractID: id, SourceCharacterID: ch.ID, SourceGeneration: 1, ContractType: kind, Status: "outstanding", Payload: payload}); err != nil {
			t.Fatal(err)
		}
		records := []string{}
		for i, typ := range types {
			records = append(records, fmt.Sprintf(`{"record_id":%d,"type_id":%d,"quantity":1,"is_included":true,"is_singleton":false}`, i+1, typ))
		}
		if err := q.InsertContractItems(ctx, store.InsertContractItemsParams{OwnerKind: owner, OwnerID: ownerID, ContractID: id, Items: []byte("[" + strings.Join(records, ",") + "]")}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.pool.Exec(ctx, `INSERT INTO eve_contract_details(owner_kind,owner_id,contract_id,part,target_id,source_character_id,generation,state) VALUES($1,$2,$3,'items',$4,$5,1,$6)`, owner, ownerID, id, target.ID, ch.ID, state); err != nil {
			t.Fatal(err)
		}
	}
	seed("character", 123, 901, "BATCH-20261013-000002", "ready", "item_exchange", 34, 34)
	seed("character", 123, 902, "", "ready", "item_exchange", 587)
	seed("character", 123, 903, "100% literal", "running", "item_exchange", 34)
	seed("character", 123, 904, "literal_under", "ready", "item_exchange", 999)
	seed("character", 123, 905, "组合礼包", "ready", "item_exchange", 34, 587)
	seed("character", 123, 906, "密封包裹", "ready", "courier", 34)
	seed("character", 456, 901, "别人的合同", "ready", "item_exchange", 587)
	seed("corporation", 123, 901, "军团合同", "ready", "item_exchange", 587)
	seed("corporation", 123, 990, "范围外合同", "ready", "item_exchange", 34)
	h := NewContractHTTP(s.pool, nil, func(*http.Request) string { return "member" })
	allowed := true
	h.Owners = func(context.Context, string) ([]ContractOwner, error) {
		if !allowed {
			return nil, nil
		}
		return []ContractOwner{{Kind: "character", ID: 123}, {Kind: "corporation", ID: 123}}, nil
	}
	router := chi.NewRouter()
	for _, r := range h.Routes() {
		router.Method(r.Method, r.Path, r.Handler)
	}
	read := func(path string, status int) ([]string, string) {
		t.Helper()
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != status {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		var b struct {
			Data struct {
				Items []contractView
				Next  string `json:"next_cursor"`
			}
		}
		_ = json.Unmarshal(w.Body.Bytes(), &b)
		ids := []string{}
		for _, c := range b.Data.Items {
			ids = append(ids, c.ID)
		}
		return ids, b.Data.Next
	}
	for _, tc := range []struct {
		q    string
		want []string
	}{
		{"三钛合金", []string{"905", "901"}}, {"tRiTaNiUm", []string{"905", "901"}}, {"Rifter", []string{"905", "902"}},
		{"Tritanium Rifter", []string{"905"}}, {"\u3000Borrower\t  三钛合金  ", []string{"901"}},
		{"#901", []string{"901"}}, {"01", []string{"901"}}, {"000002", []string{"901"}}, {"#34", []string{"905", "901"}},
		{"Jita", []string{"901"}}, {"456", []string{"901"}}, {"Private Hangar", []string{}}, {"%", []string{"903"}}, {"_", []string{"904"}},
		{"missing", []string{}}, {"组合 missing", []string{}},
	} {
		got, _ := read("/contracts/character/123?q="+url.QueryEscape(tc.q), 200)
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%q: got %v want %v", tc.q, got, tc.want)
		}
	}
	if ids, _ := read("/contracts/character/123?q=三钛合金&status=finished", 200); len(ids) != 0 {
		t.Fatal(ids)
	}
	if ids, _ := read("/contracts/corporation/123?q=三钛合金", 200); len(ids) != 0 {
		t.Fatal("out of scope corporation item leaked", ids)
	}
	if ids, _ := read("/contracts/corporation/123?q=Rifter", 200); !reflect.DeepEqual(ids, []string{"901"}) {
		t.Fatal(ids)
	}
	for i := int64(1000); i < 1030; i++ {
		seed("character", 123, i, "分页物资", "ready", "item_exchange", 34)
	}
	ids, next := read("/contracts/character/123?q=分页物资%20三钛合金", 200)
	if len(ids) != 25 || next != "1005" {
		t.Fatal(len(ids), next)
	}
	ids, next = read("/contracts/character/123?q=分页物资%20三钛合金&before="+next, 200)
	if len(ids) != 5 || next != "" {
		t.Fatal(ids, next)
	}
	for _, bad := range []string{strings.Repeat("中", 101), strings.Repeat("a ", 11), "a\x00b"} {
		read("/contracts/character/123?q="+url.QueryEscape(bad), 400)
	}
	read("/contracts/character/456?q=Rifter", 404)
	allowed = false
	read("/contracts/character/123?q=三钛合金", 404)
}

func TestWalletSearchItemsPartiesAndPagination(t *testing.T) {
	s, ch := walletFixture(t)
	ctx := context.Background()
	q := store.New(s.pool)
	c, err := q.GetCredential(ctx, ch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ImportSDENames(ctx, s.pool, sdeFixture(t, 100, sampleTypes), 100); err != nil {
		t.Fatal(err)
	}
	for _, n := range []store.SaveEntityNameParams{{EntityID: 456, Name: "Buyer Pilot", Category: "character", Language: "en"}, {EntityID: 10, Name: "Tax Corporation", Category: "corporation", Language: "en"}} {
		if err = q.SaveEntityName(ctx, n); err != nil {
			t.Fatal(err)
		}
	}
	save := func(kind, part string, owner, id int64, division int, raw string) {
		t.Helper()
		if err := store.SaveWalletObservation(ctx, s.pool, kind, owner, division, part, id, ch.ID, c.OwnerHash, time.Now(), time.Now(), []byte(raw)); err != nil {
			t.Fatal(err)
		}
	}
	for id := int64(1000); id < 1053; id++ {
		save("character", "transactions", ch.ID, id, 0, `{"type_id":34,"client_id":456,"is_buy":true,"quantity":3,"unit_price":4}`)
	}
	save("character", "transactions", ch.ID, 901, 0, `{"type_id":587,"client_id":789,"is_buy":false}`)
	save("character", "transactions", ch.ID, 900, 0, `{"client_id":789,"is_buy":false}`)
	save("character", "journal", ch.ID, 801, 0, `{"description":"activity 100%","reason":"奖励","amount":85,"tax":15,"tax_receiver_id":10,"first_party_id":456,"second_party_id":123}`)
	save("character", "journal", ch.ID, 802, 0, `{"description":"literal_under","amount":-2}`)
	save("character", "transactions", 999, 9999, 0, `{"type_id":34,"client_id":456,"is_buy":true}`)
	read := func(part, search string, before int64, direction string) []map[string]any {
		t.Helper()
		rows, e := s.auth.WalletData(ctx, WalletFilter{Kind: "character", Owner: ch.ID, Part: part, Search: search, Before: before, Direction: direction})
		if e != nil {
			t.Fatal(e)
		}
		return rows
	}
	for _, term := range []string{"三钛合金", "tritanium", "BUYER Pilot", "三钛合金 Buyer", "#34"} {
		first := read("transactions", term, 0, "buy")
		if len(first) != 51 {
			t.Fatal(term, len(first))
		}
		before, _ := strconv.ParseInt(first[49]["id"].(string), 10, 64)
		last := read("transactions", term, before, "buy")
		if len(last) != 3 {
			t.Fatal(term, len(last))
		}
	}
	for _, tc := range []struct{ part, term, dir, want string }{{"transactions", "Rifter", "sell", "901"}, {"transactions", "#901", "", "901"}, {"journal", "Tax Corporation", "tax", "801"}, {"journal", "Buyer 奖励", "", "801"}, {"journal", "%", "", "801"}, {"journal", "_", "", "802"}} {
		rows := read(tc.part, tc.term, 0, tc.dir)
		if len(rows) != 1 || rows[0]["id"] != tc.want {
			t.Fatal(tc, rows)
		}
	}
	if rows := read("transactions", "三钛合金", 0, "sell"); len(rows) != 0 {
		t.Fatal(rows)
	}
	if rows := read("journal", "missing", 0, ""); len(rows) != 0 {
		t.Fatal(rows)
	}
	if _, err = s.pool.Exec(ctx, `UPDATE eve_credentials SET scopes=ARRAY[]::text[] WHERE character_id=$1`, ch.ID); err != nil {
		t.Fatal(err)
	}
	if rows := read("transactions", "Buyer", 0, ""); len(rows) != 0 {
		t.Fatal("revoked search leaked", rows)
	}
}

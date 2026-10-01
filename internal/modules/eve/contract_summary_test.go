package eve

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
	"glorynavy.local/seat/internal/platform/locale"
)

func TestContractSummaryScopesCompletenessAndDescriptions(t *testing.T) {
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
	single := `[{"record_id":1,"type_id":34,"quantity":2,"is_singleton":false,"is_included":true}]`
	fixtures := []struct {
		id                              int64
		kind, description, state, items string
	}{
		{1, "character", "", "ready", `[{"record_id":1,"type_id":34,"quantity":2,"is_singleton":false,"is_included":true},{"record_id":2,"type_id":34,"quantity":3,"is_singleton":false,"is_included":true},{"record_id":3,"type_id":587,"quantity":1,"is_singleton":false,"is_included":true}]`},
		{1, "corporation", "", "ready", `[{"record_id":1,"type_id":587,"quantity":1,"is_singleton":false,"is_included":true}]`},
		{2, "character", " \n ", "ready", `[{"record_id":1,"type_id":34,"quantity":9007199254740993,"is_singleton":false,"is_included":false}]`},
		{3, "character", "", "ready", `[{"record_id":1,"type_id":34,"quantity":2,"is_singleton":false,"is_included":true},{"record_id":2,"type_id":587,"quantity":1,"is_singleton":false,"is_included":false}]`},
		{4, "character", "原始描述", "ready", single},
		{5, "character", "", "running", single},
		{6, "character", "", "ready", `[{"record_id":1,"type_id":34,"quantity":2,"is_singleton":false,"is_included":true},{"record_id":2,"type_id":34,"quantity":3,"is_singleton":false,"is_included":true}]`},
	}
	for _, f := range fixtures {
		raw := map[string]any{}
		if err := json.Unmarshal(contractJSON(int(f.id), "item_exchange"), &raw); err != nil {
			t.Fatal(err)
		}
		raw["title"] = f.description
		if f.kind == "corporation" {
			raw["assignee_id"] = 123
		}
		payload, _ := json.Marshal(raw)
		if _, err := q.UpsertContract(ctx, store.UpsertContractParams{OwnerKind: f.kind, OwnerID: 123, ContractID: f.id, SourceCharacterID: 123, SourceGeneration: 1, ContractType: "item_exchange", Status: "outstanding", Payload: payload}); err != nil {
			t.Fatal(err)
		}
		if err := q.InsertContractItems(ctx, store.InsertContractItemsParams{OwnerKind: f.kind, OwnerID: 123, ContractID: f.id, Items: []byte(f.items)}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.pool.Exec(ctx, `INSERT INTO eve_contract_details(owner_kind,owner_id,contract_id,part,target_id,source_character_id,generation,state) VALUES($1,123,$2,'items',$3,123,1,$4)`, f.kind, f.id, target.ID, f.state); err != nil {
			t.Fatal(err)
		}
	}
	h := NewContractHTTP(s.pool, nil, func(*http.Request) string { return "member" })
	h.Owners = func(context.Context, string) ([]ContractOwner, error) {
		return []ContractOwner{{Kind: "character", ID: 123}}, nil
	}
	router := chi.NewRouter()
	for _, route := range h.Routes() {
		router.Method(route.Method, route.Path, route.Handler)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/contracts/character/123", nil))
	var body struct {
		Data struct {
			Items []contractView `json:"items"`
		} `json:"data"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &body) != nil {
		t.Fatal(w.Code, w.Body.String())
	}
	want := map[string]string{"1": "三钛合金等 2 种物品", "2": "三钛合金 × 9007199254740993", "3": "提供 三钛合金 × 2 · 换取 Rifter × 1", "4": "", "5": "", "6": "三钛合金 × 5"}
	if len(body.Data.Items) != 6 {
		t.Fatal("wrong list size")
	}
	directions := map[string]string{"1": "sell", "2": "buy", "3": "exchange", "4": "sell", "5": "unknown", "6": "sell"}
	for _, c := range body.Data.Items {
		if c.TradeDirection != directions[c.ID] {
			t.Fatalf("direction %s: %s", c.ID, c.TradeDirection)
		}
		if c.Summary != want[c.ID] {
			t.Fatalf("contract %s: %q", c.ID, c.Summary)
		}
		if c.ID == "4" && c.Title != "原始描述" {
			t.Fatal("description overwritten")
		}
		w = httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", "/contracts/character/123/"+c.ID, nil))
		var detail struct {
			Data struct {
				Contract contractView `json:"contract"`
			} `json:"data"`
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &detail) != nil || detail.Data.Contract.Summary != c.Summary || detail.Data.Contract.TradeDirection != c.TradeDirection {
			t.Fatal("list/detail summary mismatch")
		}
	}
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/contracts/corporation/123/1", nil))
	if w.Code != 404 || strings.Contains(w.Body.String(), "Rifter") {
		t.Fatal("unauthorized summary leaked")
	}
	en := []contractView{{ID: "1", Type: "item_exchange"}, {ID: "2", Type: "item_exchange"}, {ID: "3", Type: "item_exchange"}, {ID: "4", Type: "item_exchange", Title: "原始描述"}}
	h.summaries(locale.With(ctx, "en"), "character", 123, en)
	for i, want := range []string{"Tritanium and others · 2 item types", "Tritanium × 9007199254740993", "Offer Tritanium × 2 · Receive Rifter × 1", ""} {
		if en[i].Summary != want {
			t.Fatalf("English summary %d: %q", i, en[i].Summary)
		}
	}
	if en[3].Title != "原始描述" {
		t.Fatal("user title changed")
	}
	h.StaticData = nil
	withoutNames := []contractView{{ID: "2", Type: "item_exchange", Title: "Description"}, {ID: "1", Type: "loan"}, {ID: "100", Type: "item_exchange"}}
	h.summaries(ctx, "character", 123, withoutNames)
	if withoutNames[0].TradeDirection != "buy" || withoutNames[0].Summary != "" || withoutNames[1].TradeDirection != "unknown" || withoutNames[2].TradeDirection != "unknown" {
		t.Fatal("direction must survive unavailable names and reject unsupported or missing items", withoutNames)
	}
	courier := []contractView{{ID: "9", Type: "courier", Start: entity(600, "station"), End: entity(601, "station")}}
	h.summaries(ctx, "character", 123, courier)
	if courier[0].Summary != "#600 → #601" || courier[0].TradeDirection != "transport" {
		t.Fatal(courier[0].Summary)
	}
}

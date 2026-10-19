package welfare

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
)

func TestCapitalCheckPreviewAndClaims(t *testing.T) {
	s, purchase := capitalFixture(t)
	ctx := context.Background()
	check := func(kind string, id int64, want string) CapitalCheck {
		t.Helper()
		got, err := s.CapitalCheck(ctx, userID, 10, kind, id)
		if err != nil || got.State != want {
			t.Fatalf("%s/%d: %+v %v; want %s", kind, id, got, err, want)
		}
		return got
	}
	check("supercarrier", 0, "closed")
	rate := int64(1250)
	if _, err := s.Execute(ctx, adminID, Command{Action: "configure", RequestKey: key(2900), CorporationID: 10, Kind: "supercarrier", Config: Config{Enabled: true, SubsidyRateBPS: &rate}}); err != nil {
		t.Fatal(err)
	}
	check("supercarrier", 0, "contract_required")
	check("supercarrier", 999, "contract_invalid")
	got := check("supercarrier", 901, "ready")
	if got.Purchase == nil || got.Purchase.CharacterID != 123 || got.Purchase.PriceMinor != 1000000000000 || got.Purchase.AwardMinor != 125000000000 || got.RateBPS != rate {
		t.Fatal(got)
	}
	var cases, claims int
	if err := s.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM welfare_cases), (SELECT count(*) FROM welfare_claims)`).Scan(&cases, &claims); err != nil || cases != 0 || claims != 0 {
		t.Fatal("preview mutated state", cases, claims, err)
	}
	purchase.Status = "outstanding"
	check("supercarrier", 901, "contract_invalid")
	purchase.Status = "finished"
	data, err := s.Execute(ctx, userID, Command{Action: "apply", RequestKey: key(2901), CorporationID: 10, Kind: "supercarrier", Detail: Detail{ContractID: 901}})
	if err != nil {
		t.Fatal(err)
	}
	var item Case
	if err = json.Unmarshal(data, &item); err != nil {
		t.Fatal(err)
	}
	if got = check("supercarrier", 0, "pending"); got.CaseID != item.ID || got.Purchase != nil {
		t.Fatal(got)
	}
	if _, err = s.Execute(ctx, userID, Command{Action: "cancel", RequestKey: key(2902), ID: item.ID, Version: item.Version}); err != nil {
		t.Fatal(err)
	}
	check("supercarrier", 901, "ready")
	if err = store.SaveProfile(ctx, s.Pool, Member{AccountID: userID, History: map[string]string{"supercarrier": "used"}, Months: []string{}}); err != nil {
		t.Fatal(err)
	}
	check("supercarrier", 901, "claimed")
}

func TestCapitalCheckIsolationAndHTTP(t *testing.T) {
	s, _ := capitalFixture(t)
	ctx := context.Background()
	if _, err := s.Execute(ctx, adminID, Command{Action: "configure", RequestKey: key(2910), CorporationID: 10, Kind: "supercarrier", Config: Config{Enabled: true}}); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"capital", "solo", "unknown"} {
		if _, err := s.CapitalCheck(ctx, userID, 10, kind, 901); !errors.Is(err, ErrInvalid) {
			t.Fatal(kind, err)
		}
	}
	if _, err := s.CapitalCheck(ctx, userID, 11, "supercarrier", 901); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("foreign corporation", err)
	}
	s.Characters = func(context.Context, string) ([]Character, error) {
		return []Character{{ID: 123, AccountID: otherID, CorporationID: 10}, {ID: 456, AccountID: userID, CorporationID: 11}}, nil
	}
	s.PurchaseContract = func(context.Context, string, int64, int64) (eve.DeliveryContract, error) {
		t.Fatal("queried unowned purchase")
		return eve.DeliveryContract{}, nil
	}
	if got, err := s.CapitalCheck(ctx, userID, 10, "supercarrier", 901); err != nil || got.State != "unbound" {
		t.Fatal(got, err)
	}
	h := Handler{Service: s, User: func(*http.Request) string { return userID }}
	registered := false
	for _, route := range h.Module().Routes {
		if route.Path == "/capital/check" {
			registered = route.Method == "GET" && route.Permission == "welfare.self" && !route.Public
		}
	}
	if !registered {
		t.Fatal("preview route not protected")
	}
	for _, id := range []string{"-1", "0", "abc", "9223372036854775808"} {
		rec := httptest.NewRecorder()
		h.capitalCheck(rec, httptest.NewRequest("GET", "/capital/check?corporation_id=10&kind=supercarrier&contract_id="+id, nil))
		if rec.Code != http.StatusBadRequest {
			t.Fatal(id, rec.Code)
		}
	}
}

func TestCapitalCheckContractOccupiedAndPrecision(t *testing.T) {
	s, purchase := capitalFixture(t)
	ctx := context.Background()
	rate := int64(3333)
	if _, err := s.Execute(ctx, adminID, Command{Action: "configure", RequestKey: key(2920), CorporationID: 10, Kind: "titan", Config: Config{Enabled: true, SubsidyRateBPS: &rate}}); err != nil {
		t.Fatal(err)
	}
	purchase.Items = []eve.DeliveryItem{{TypeID: 671, Quantity: 1, Included: true, Name: "Erebus"}}
	purchase.Price = "999999999999.99"
	got, err := s.CapitalCheck(ctx, userID, 10, "titan", 901)
	if err != nil || got.State != "ready" || got.Purchase.AwardMinor != 33329999999999 || got.Purchase.ShipName != "Erebus" {
		t.Fatal(got, err)
	}
	// A different account's existing use is not exposed as a record or identity.
	_, err = store.Save(ctx, s.Pool, Case{AccountID: otherID, CorporationID: 10, Kind: "titan", State: "submitted", Detail: raw(Detail{ContractID: 901}), Keys: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	got, err = s.CapitalCheck(ctx, userID, 10, "titan", 901)
	if err != nil || got.State != "contract_used" || got.CaseID != 0 || got.Purchase != nil {
		t.Fatal(got, err)
	}
}

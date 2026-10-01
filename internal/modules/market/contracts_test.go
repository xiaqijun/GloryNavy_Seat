package market

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/testutil"
)

func appraisalContract() eve.DeliveryContract {
	return eve.DeliveryContract{ID: 99, Type: "item_exchange", Status: "outstanding", Expired: time.Now().Add(time.Hour), ItemsReady: true, ContentToken: "terms",
		Items: []eve.DeliveryItem{{TypeID: 34, Quantity: 1000, Included: true, Name: "Tritanium"}, {TypeID: 35, Quantity: 2, Included: false, Name: "Pyerite"}}}
}

func TestContractAppraisalEligibilityAndSides(t *testing.T) {
	c := appraisalContract()
	for _, side := range []string{"included", "requested"} {
		items, err := contractItems(c, side, time.Now())
		if err != nil || len(items) != 1 || (side == "included" && items[0].TypeID != 34) || (side == "requested" && items[0].TypeID != 35) {
			t.Fatal(items, err)
		}
	}
	for _, mutate := range []func(*eve.DeliveryContract){
		func(c *eve.DeliveryContract) { c.Status = "finished" },
		func(c *eve.DeliveryContract) { c.Expired = time.Now().Add(-time.Second) },
		func(c *eve.DeliveryContract) { c.AcceptorID = 5 },
		func(c *eve.DeliveryContract) { c.Accepted = "2026-09-20T00:00:00Z" },
		func(c *eve.DeliveryContract) { c.Type = "courier" },
		func(c *eve.DeliveryContract) { c.ItemsReady = false },
		func(c *eve.DeliveryContract) { c.Items = nil },
		func(c *eve.DeliveryContract) { q := int64(-2); c.Items[0].RawQuantity = &q },
		func(c *eve.DeliveryContract) { c.Items[0].Quantity = 1000000001 },
	} {
		c := appraisalContract()
		mutate(&c)
		if _, err := contractItems(c, "included", time.Now()); err == nil {
			t.Fatalf("accepted invalid contract: %+v", c)
		}
	}
}

func TestContractAppraisalHTTPGuardsAndSnapshots(t *testing.T) {
	pool := testutil.Database(t)
	if _, err := pool.Exec(context.Background(), `UPDATE market_settings SET ratio_bps=8000`); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []struct {
		name   string
		status int
	}{{"ready", 200}, {"requested", 200}, {"denied", 404}, {"revoked", 404}, {"changed", 409}, {"accepted", 409}, {"partial", 409}, {"invalid-side", 400}, {"forged-items", 400}} {
		t.Run(scenario.name, func(t *testing.T) {
			reads, prices := 0, 0
			s := &Service{Pool: pool, Contract: func(_ context.Context, actor, kind string, owner, id int64) (eve.DeliveryContract, error) {
				reads++
				if actor != "member" || kind != "corporation" || owner != 10 || id != 99 {
					t.Fatal("scope not forwarded")
				}
				if scenario.name == "denied" || (scenario.name == "revoked" && reads > 1) {
					return eve.DeliveryContract{}, pgx.ErrNoRows
				}
				c := appraisalContract()
				if scenario.name == "changed" && reads > 1 {
					c.ContentToken = "different"
				}
				if scenario.name == "accepted" && reads > 1 {
					c.Status = "finished"
				}
				if scenario.name == "partial" {
					c.ItemsReady = false
				}
				return c, nil
			}, Prices: func(_ context.Context, id int64) (eve.MarketPrices, error) {
				prices++
				want := int64(34)
				if scenario.name == "requested" {
					want = 35
				}
				if id != want {
					t.Fatal("wrong side priced", id)
				}
				return eve.MarketPrices{Buy: str("4.00"), Sell: str("6.00")}, nil
			}}
			h := Handler{Service: s, User: func(*http.Request) string { return "member" }}
			side := "included"
			if scenario.name == "requested" {
				side = "requested"
			}
			if scenario.name == "invalid-side" {
				side = "all"
			}
			body := `{"owner_kind":"corporation","owner_id":"10","contract_id":"99","side":"` + side + `"}`
			if scenario.name == "forged-items" {
				body = strings.TrimSuffix(body, "}") + `,"items":[]}`
			}
			w := httptest.NewRecorder()
			h.estimateContract(w, httptest.NewRequest("POST", "/estimate-contract", strings.NewReader(body)))
			if w.Code != scenario.status {
				t.Fatal(w.Code, w.Body.String())
			}
			if scenario.status == 200 {
				var out struct {
					Data Appraisal `json:"data"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
					t.Fatal(err)
				}
				want := "4000.00"
				if scenario.name == "requested" {
					want = "8.00"
				}
				if out.Data.Adjusted.Mid != want || len(out.Data.Lines) != 1 || reads != 2 {
					t.Fatal(out, reads)
				}
			}
			if (scenario.name == "denied" || scenario.name == "partial" || scenario.status == 400) && prices != 0 {
				t.Fatal("priced rejected contract")
			}
		})
	}
}

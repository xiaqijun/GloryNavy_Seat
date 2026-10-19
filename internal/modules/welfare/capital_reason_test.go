package welfare

import (
	"context"
	"errors"
	"testing"

	"glorynavy.local/seat/internal/modules/eve"
)

func TestCapitalPurchaseReasonsPreserveValidation(t *testing.T) {
	s, purchase := capitalFixture(t)
	ctx := context.Background()
	if _, err := s.Execute(ctx, adminID, Command{Action: "configure", RequestKey: key(2940), CorporationID: 10, Kind: "supercarrier", Config: Config{Enabled: true}}); err != nil {
		t.Fatal(err)
	}
	base := *purchase
	for _, tc := range []struct {
		name, reason string
		change       func(*eve.DeliveryContract)
	}{
		{"modules-only", "ship_missing", func(c *eve.DeliveryContract) {
			c.Items = []eve.DeliveryItem{{TypeID: 13922, Quantity: 4, Included: true}}
		}},
		{"wrong-class", "ship_type", func(c *eve.DeliveryContract) {
			c.Items = []eve.DeliveryItem{{TypeID: 671, Quantity: 1, Included: true}}
		}},
		{"items-not-ready", "items_pending", func(c *eve.DeliveryContract) { c.ItemsReady = false }},
		{"not-purchaser", "", func(c *eve.DeliveryContract) { c.AcceptorID = 456 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			*purchase = base
			tc.change(purchase)
			if _, _, err := s.capitalPurchase("supercarrier", 123, *purchase); !errors.Is(err, ErrCapitalPurchase) {
				t.Fatal("guard error identity changed", err)
			}
			got, err := s.CapitalCheck(ctx, userID, 10, "supercarrier", 901)
			if err != nil || got.State != "contract_invalid" || got.Reason != tc.reason || got.Purchase != nil {
				t.Fatal(got, err)
			}
			if _, err = s.Execute(ctx, userID, Command{Action: "apply", RequestKey: key(2941), CorporationID: 10, Kind: "supercarrier", Detail: Detail{ContractID: 901}}); !errors.Is(err, ErrCapitalPurchase) {
				t.Fatal("invalid purchase accepted", err)
			}
		})
	}
	*purchase = base
	purchase.ForCorporation = true
	got, err := s.CapitalCheck(ctx, userID, 10, "supercarrier", 901)
	if err != nil || got.State != "ready" || got.Reason != "" {
		t.Fatal("corporation seller is not a corporation buyer", got, err)
	}
	var count int
	if err = s.Pool.QueryRow(ctx, `SELECT count(*) FROM welfare_cases`).Scan(&count); err != nil || count != 0 {
		t.Fatal("preview or invalid apply changed cases", count, err)
	}
}

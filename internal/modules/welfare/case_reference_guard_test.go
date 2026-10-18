package welfare

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/exchange"
	"glorynavy.local/seat/migrations"
	"testing"
	"time"
)

func TestRepairedCaseGrowthSnapshotStillExact(t *testing.T) {
	old := "WF-20261001-11111111-1111-4111-8111-111111111111"
	v := Case{Kind: "growth_fitting_1", Reference: "WF-20261001-000001", CorporationID: 10, CreatedAt: time.Now().Add(-time.Hour)}
	d := Detail{CharacterID: 123, Rewards: &GrowthRewards{Items: []GrowthItem{{ID: 34, Quantity: 5}}}}
	c := eve.DeliveryContract{ID: 1, ContentToken: "terms", Type: "item_exchange", Title: old, IssuerID: 456, IssuerCorporationID: 10, AssigneeID: 123, Issued: time.Now().Add(-time.Minute), Status: "outstanding", ItemsReady: true, Price: "0", Reward: "0", Items: []eve.DeliveryItem{{TypeID: 34, Quantity: 5, Included: true}}}
	match := func(snapshot json.RawMessage, c eve.DeliveryContract) (bool, error) {
		return exchange.MatchRewardDelivery(snapshot, c)
	}
	if status := fulfillmentState(caseForContractReference(v, []string{v.Reference, old}, c.Title), d, c, match); status != "awaiting_acceptance" {
		t.Fatal(status)
	}
	c.Items[0].Quantity = 4
	if status := fulfillmentState(caseForContractReference(v, []string{v.Reference, old}, c.Title), d, c, match); status != "mismatch" {
		t.Fatal("alias bypassed quantity", status)
	}
	c.Items[0].Quantity = 5
	c.AssigneeID = 999
	if status := fulfillmentState(caseForContractReference(v, []string{v.Reference, old}, c.Title), d, c, match); status != "mismatch" {
		t.Fatal("alias bypassed recipient", status)
	}
}
func TestRepairedCaseDownRetainsHistory(t *testing.T) {
	s, _, _ := repairedCase(t)
	db := stdlib.OpenDB(*s.Pool.Config().ConnConfig)
	defer db.Close()
	p, err := goose.NewProvider(goose.DialectPostgres, db, migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.DownTo(context.Background(), 96); err == nil {
		t.Fatal("repair audit lost")
	}
}

package welfare

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/market"
	"testing"
)

func TestQuoteLossChecksObjectBeforeMarketAndKeepsItemOrder(t *testing.T) {
	called := false
	s := &Service{
		Losses: func(_ context.Context, actor string, corp, character, before, id int64) ([]Loss, error) {
			if actor != "owner" || corp != 10 || character != 123 || before != 0 || id != 987 {
				return nil, pgx.ErrNoRows
			}
			return []Loss{{ID: 987, CharacterID: 123, CorporationID: 10, Items: []eve.BattleItem{
				{TypeID: 34, Name: "Cargo", Quantity: 5, Dropped: 3, Destroyed: 2},
				{TypeID: 35, Name: "Empty", Quantity: 0},
				{TypeID: 36, Name: "Module", Quantity: 1, Destroyed: 1},
			}}}, nil
		},
		EstimateLoss: func(_ context.Context, items []market.Item) (market.Appraisal, int64, error) {
			called = true
			if len(items) != 2 || items[0].TypeID != 34 || items[0].Quantity != 5 || items[1].TypeID != 36 {
				t.Fatalf("wrong market input: %+v", items)
			}
			cargo, module := "120.00", "400.00"
			return market.Appraisal{Lines: []market.Line{
				{TypeID: 34, Quantity: 5, Mid: &cargo},
				{TypeID: 36, Quantity: 1, Mid: &module},
			}}, 1, nil
		},
	}
	if _, err := s.quoteLoss(context.Background(), "other", 10, 123, 987); !errors.Is(err, pgx.ErrNoRows) || called {
		t.Fatalf("unauthorized quote reached market: %v %v", err, called)
	}
	prices, err := s.quoteLoss(context.Background(), "owner", 10, 123, 987)
	if err != nil || !called || len(prices) != 3 || prices[0].Mid == nil || *prices[0].Mid != "120.00" || prices[1].Mid != nil || prices[2].Mid == nil || *prices[2].Mid != "400.00" {
		t.Fatalf("incorrect price mapping: %+v %v", prices, err)
	}
}

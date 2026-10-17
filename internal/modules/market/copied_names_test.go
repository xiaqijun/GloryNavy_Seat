package market

import (
	"context"
	"testing"

	"glorynavy.local/seat/internal/modules/eve"
)

func TestAppraisalCopiedNameMarkerKeepsQuantitiesAndExactNames(t *testing.T) {
	const chinese = "23级监察官的个人财产"
	const english = "23rd Tier Overseer's Personal Effects"
	for _, name := range []string{chinese, english} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			s := Service{Resolve: func(_ context.Context, names []string) (map[string]eve.StaticTypeName, error) {
				// Simulate the same exact SDE lookup keys used by the real resolver.
				found := map[string]eve.StaticTypeName{}
				for _, n := range names {
					if n == chinese {
						found[n] = eve.StaticTypeName{ID: 19421, Name: chinese}
					}
					if n == english {
						found["23rd tier overseer's personal effects"] = eve.StaticTypeName{ID: 19421, Name: chinese}
					}
				}
				return found, nil
			}, Prices: func(_ context.Context, id int64) (eve.MarketPrices, error) {
				if id != 19421 {
					t.Fatal("wrong type", id)
				}
				calls++
				return eve.MarketPrices{Buy: str("10.00"), Sell: str("12.00")}, nil
			}}
			text := name + "*\t3\r\n" + name + "*\t2"
			got, err := s.Estimate(context.Background(), text, 10000)
			if err != nil || !got.Complete || len(got.Lines) != 2 || calls != 1 || got.Totals.Mid != "55.00" {
				t.Fatal(got, calls, err)
			}
			for i, quantity := range []int64{3, 2} {
				line := got.Lines[i]
				if line.TypeID != 19421 || line.Quantity != quantity || line.Name != chinese || line.Status != "ready" {
					t.Fatal(line)
				}
			}
			if got.Lines[0].Input != name+"*\t3" || got.Lines[1].Input != name+"*\t2" {
				t.Fatal("original input changed", got.Lines)
			}
		})
	}
}

func TestAppraisalCopiedNameMarkerDoesNotGuess(t *testing.T) {
	s := Service{Resolve: func(context.Context, []string) (map[string]eve.StaticTypeName, error) {
		return map[string]eve.StaticTypeName{
			"exact*": {ID: 1, Name: "Exact*"}, "exact": {ID: 2, Name: "Exact"},
			"known": {ID: 3, Name: "Known"},
		}, nil
	}, Prices: func(_ context.Context, id int64) (eve.MarketPrices, error) {
		if id == 2 {
			t.Fatal("fallback replaced exact name")
		}
		return eve.MarketPrices{Buy: str("1"), Sell: str("1")}, nil
	}}
	got, err := s.Estimate(context.Background(), "Exact*\t2\nKnown *\t3\nUnknown*\t1\nKno*wn\t1\nKnown**\t1\n*\t1\nKnown*\t1,2", 10000)
	if err != nil || got.Complete || got.Lines[0].TypeID != 1 || got.Lines[1].TypeID != 3 || got.Totals.Mid != "5.00" {
		t.Fatal(got, err)
	}
	for _, line := range got.Lines[2:6] {
		if line.Status != "unknown_type" || line.Mid != nil {
			t.Fatal("guessed unknown name", line)
		}
	}
	if got.Lines[6].Status != "invalid_quantity" {
		t.Fatal("invalid quantity accepted", got.Lines[6])
	}
}

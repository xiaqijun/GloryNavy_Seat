package loan

import (
	"context"
	"encoding/json"
	"testing"
)

func TestValueCollateralItemsIncludesAssembledChildrenAndDeduplicatesRecords(t *testing.T) {
	raw := json.RawMessage(`[{"record_id":"10","type_id":"100","quantity":"1","included":true,"items":[{"record_id":"11","type_id":"200","quantity":"2","included":true}]},{"record_id":"10","type_id":"100","quantity":"1","included":true}]`)
	value, err := ValueCollateralItems(context.Background(), raw, func(_ context.Context, ids []int64) (map[int64]string, error) {
		if len(ids) != 2 {
			t.Fatalf("price ids=%v, want two types", ids)
		}
		return map[int64]string{100: "10.00", 200: "2.50"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if value != 1500 {
		t.Fatalf("value=%d, want 1500 minor ISK", value)
	}
}

func TestValueCollateralItemsRejectsMissingIncludedPrice(t *testing.T) {
	raw := json.RawMessage(`[{"record_id":"10","type_id":"100","quantity":"1","included":true}]`)
	if _, err := ValueCollateralItems(context.Background(), raw, func(context.Context, []int64) (map[int64]string, error) {
		return map[int64]string{}, nil
	}); err == nil {
		t.Fatal("expected missing price error")
	}
}

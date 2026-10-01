package eve

import (
	"context"
	"encoding/json"
	"glorynavy.local/seat/internal/testutil"
	"net/http"
	"testing"
)

func TestJitaPricesFullPaginationStationAndMissingSide(t *testing.T) {
	p := testutil.Database(t)
	s := newESI(p, nil)
	phase := 0
	s.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/markets/10000002/orders/" || r.URL.Query().Get("type_id") != "34" || r.URL.Query().Get("order_type") != "all" {
			t.Fatal(r.URL)
		}
		if _, err := p.Exec(r.Context(), `UPDATE eve_esi_limits SET next_request_at='epoch'`); err != nil {
			t.Fatal(err)
		}
		if phase == 1 {
			return contractListResponse([]json.RawMessage{}, "3"), nil
		}
		if r.URL.Query().Get("page") == "1" {
			return contractListResponse([]json.RawMessage{json.RawMessage(`{"type_id":34,"location_id":60003760,"is_buy_order":true,"price":4,"volume_remain":5}`), json.RawMessage(`{"type_id":34,"location_id":60000001,"is_buy_order":false,"price":1,"volume_remain":5}`)}, "2"), nil
		}
		return contractListResponse([]json.RawMessage{json.RawMessage(`{"type_id":34,"location_id":60003760,"is_buy_order":true,"price":5.01,"volume_remain":5}`), json.RawMessage(`{"type_id":34,"location_id":60003760,"is_buy_order":false,"price":8.01,"volume_remain":5}`)}, "2"), nil
	})
	v, err := s.JitaPrices(context.Background(), 34)
	if err != nil || v.Buy == nil || *v.Buy != "5.01" || v.Sell == nil || *v.Sell != "8.01" {
		t.Fatal(v, err)
	}
	phase = 1
	if _, err = p.Exec(context.Background(), `UPDATE eve_esi_cache SET expires_at='epoch' WHERE page_count=2 AND convert_from(body,'UTF8') LIKE '%60000001%'`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.JitaPrices(context.Background(), 34); err == nil {
		t.Fatal("mixed page counts accepted")
	}
}

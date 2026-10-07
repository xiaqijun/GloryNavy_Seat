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

func TestJitaPricesPLEXUsesGlobalMarket(t *testing.T) {
	p := testutil.Database(t)
	s := newESI(p, nil)
	s.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		if _, err := p.Exec(r.Context(), `UPDATE eve_esi_limits SET next_request_at='epoch'`); err != nil {
			t.Fatal(err)
		}
		if r.URL.Path != "/markets/19000001/orders/" || r.URL.Query().Get("type_id") != "44992" {
			t.Fatal(r.URL)
		}
		if r.URL.Query().Get("page") == "1" {
			return contractListResponse([]json.RawMessage{
				json.RawMessage(`{"type_id":44992,"location_id":60004462,"is_buy_order":false,"price":5090000,"volume_remain":5}`),
				json.RawMessage(`{"type_id":44992,"location_id":60003760,"is_buy_order":true,"price":4715000,"volume_remain":5}`),
			}, "2"), nil
		}
		return contractListResponse([]json.RawMessage{json.RawMessage(`{"type_id":44992,"location_id":60003760,"is_buy_order":true,"price":4717000,"volume_remain":5}`)}, "2"), nil
	})
	v, err := s.JitaPrices(context.Background(), 44992)
	if err != nil || v.Buy == nil || *v.Buy != "4717000.00" || v.Sell == nil || *v.Sell != "5090000.00" || v.Mid != nil {
		t.Fatal(v, err)
	}
}

func TestJitaPricesPLEXUsesESIAverageWhenGlobalOrderBookIsEmpty(t *testing.T) {
	p := testutil.Database(t)
	s := newESI(p, nil)
	s.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		if _, err := p.Exec(r.Context(), `UPDATE eve_esi_limits SET next_request_at='epoch'`); err != nil {
			t.Fatal(err)
		}
		switch r.URL.Path {
		case "/markets/19000001/orders/":
			if r.URL.Query().Get("type_id") != "44992" || r.URL.Query().Get("page") != "1" {
				t.Fatal(r.URL)
			}
			return contractListResponse([]json.RawMessage{}, "1"), nil
		case "/markets/prices/":
			return esiResponse([]map[string]any{{"type_id": 44992, "average_price": 4712023.41}}), nil
		default:
			t.Fatal(r.URL)
			return nil, nil
		}
	})
	v, err := s.JitaPrices(context.Background(), 44992)
	if err != nil || v.Buy != nil || v.Sell != nil || v.Mid == nil || *v.Mid != "4712023.41" {
		t.Fatal(v, err)
	}
}

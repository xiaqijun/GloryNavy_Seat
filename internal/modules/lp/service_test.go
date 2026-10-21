package lp

import (
	"context"
	"encoding/json"
	"errors"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/platform/locale"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestOffersValidateCatalogAndLocalizeMaterials(t *testing.T) {
	var called bool
	s := Service{
		Reference: func(context.Context) (*eve.LPReference, error) {
			return &eve.LPReference{Build: 123, Corporations: []eve.LPCorporation{{LPName: eve.LPName{ID: 1000130}}}, Blueprints: map[int64]eve.LPBlueprint{10: {Product: eve.LPMaterial{TypeID: 30, Quantity: 1}, Materials: []eve.LPMaterial{{TypeID: 40, Quantity: 5}}}}}, nil
		},
		Request: func(_ context.Context, r eve.ESIRequest, out any) (eve.ESIResponse, error) {
			called = true
			if r.Path != "/loyalty/stores/1000130/offers/" || r.CharacterID != 0 || r.Generation != 0 || len(r.Scopes) != 0 {
				t.Fatal(r)
			}
			return eve.ESIResponse{ContentUpdatedAt: time.Unix(100, 0)}, json.Unmarshal([]byte(`[{"offer_id":1,"type_id":10,"quantity":2,"lp_cost":100,"isk_cost":50,"required_items":[{"type_id":20,"quantity":3}]}]`), out)
		},
		Names: func(ctx context.Context, ids []int64) (map[int64]eve.StaticTypeName, error) {
			if len(ids) != 4 {
				t.Fatal(ids)
			}
			return map[int64]eve.StaticTypeName{10: {Name: locale.Choose(ctx, "蓝图", "Blueprint")}}, nil
		},
	}
	if _, err := s.Offers(context.Background(), 999); !errors.Is(err, ErrInput) || called {
		t.Fatal(err, called)
	}
	v, err := s.Offers(context.Background(), 1000130)
	if err != nil || len(v.Offers) != 1 || v.Offers[0].Blueprint.Product.TypeID != 30 || v.Names[10].Chinese != "蓝图" || v.Names[10].English != "Blueprint" || v.Build != 123 {
		t.Fatal(v, err)
	}
}
func TestMarketCachePriceEdgesAndBounds(t *testing.T) {
	var count atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		if r.URL.Query().Get("station") != "60003760" || r.URL.Query().Get("types") != "10,20" {
			t.Error(r.URL)
		}
		w.Write([]byte(`{"10":{"buy":{"max":"15","volume":"3"},"sell":{"min":"20","volume":"7"}},"20":{"buy":{"max":"900","volume":"0"},"sell":{"min":"10","volume":"0"}}}`))
	}))
	defer server.Close()
	m := NewMarketClient()
	m.base = server.URL
	m.client = server.Client()
	for i := 0; i < 2; i++ {
		v, e := m.Prices(context.Background(), 10000002, []int64{20, 10, 10})
		if e != nil || *v.Items[10].Buy != 15 || *v.Items[10].Sell != 20 || v.Items[20].Buy != nil || v.Items[20].Sell != nil {
			t.Fatal(v, e)
		}
	}
	if count.Load() != 1 {
		t.Fatal(count.Load())
	}
	if _, e := m.Prices(context.Background(), 42, []int64{10}); !errors.Is(e, ErrInput) {
		t.Fatal(e)
	}
	if _, e := m.Prices(context.Background(), 10000002, make([]int64, 101)); !errors.Is(e, ErrInput) {
		t.Fatal(e)
	}
}
func TestMarketRejectsRedirectAndMalformedData(t *testing.T) {
	for _, body := range []string{"null", "[]", "{broken"} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
		m := NewMarketClient()
		m.base = server.URL
		if _, e := m.Prices(context.Background(), 10000002, []int64{10}); !errors.Is(e, ErrUnavailable) {
			t.Fatal(body, e)
		}
		server.Close()
	}
	var followed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/target" {
			followed.Store(true)
		}
		http.Redirect(w, r, "/target", 302)
	}))
	defer server.Close()
	m := NewMarketClient()
	m.base = server.URL
	if _, e := m.Prices(context.Background(), 10000002, []int64{10}); !errors.Is(e, ErrUnavailable) || followed.Load() {
		t.Fatal(e)
	}
	t.Setenv("LP_HTTP_PROXY", ":invalid")
	if NewMarketClient().client != nil {
		t.Fatal("invalid proxy did not fail closed")
	}
}

func TestESIMarketFallbackStationPaginationAndCache(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer server.Close()
	m := NewMarketClient()
	m.base = server.URL
	m.Request = func(_ context.Context, r eve.ESIRequest, out any) (eve.ESIResponse, error) {
		calls.Add(1)
		if r.CharacterID != 0 || r.Generation != 0 || !r.ExpectPages {
			t.Fatal(r)
		}
		body := `[{"type_id":34,"location_id":60003760,"is_buy_order":true,"price":3,"volume_remain":8},{"type_id":34,"location_id":60003760,"is_buy_order":false,"price":5,"volume_remain":10},{"type_id":34,"location_id":60000000,"is_buy_order":false,"price":1,"volume_remain":50}]`
		if r.Path == "/markets/10000002/orders/?order_type=all&type_id=34&page=2" {
			body = `[{"type_id":34,"location_id":60003760,"is_buy_order":false,"price":4,"volume_remain":10}]`
		}
		return eve.ESIResponse{Pages: 2, ContentUpdatedAt: time.Now().Add(-time.Hour), ExpiresAt: time.Now().Add(time.Minute)}, json.Unmarshal([]byte(body), out)
	}
	for i := 0; i < 2; i++ {
		v, e := m.Prices(context.Background(), 10000002, []int64{34})
		if e != nil || v.Source != "esi" || *v.Items[34].Buy != 3 || *v.Items[34].Sell != 4 {
			t.Fatal(v, e)
		}
	}
	if calls.Load() != 2 {
		t.Fatal("cache used observation time instead of validity", calls.Load())
	}
}

func TestESIMarketFallbackDoesNotHideGatewayFailure(t *testing.T) {
	m := NewMarketClient()
	m.Request = func(context.Context, eve.ESIRequest, any) (eve.ESIResponse, error) {
		return eve.ESIResponse{}, errors.New("budget deferred")
	}
	if _, e := m.fetchESI(context.Background(), 10000002, 60003760, []string{"34"}); e == nil {
		t.Fatal("gateway failure became missing price")
	}
}

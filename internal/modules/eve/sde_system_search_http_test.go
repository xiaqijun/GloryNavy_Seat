package eve

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"glorynavy.local/seat/internal/platform/locale"
	"glorynavy.local/seat/internal/testutil"
)

func TestSolarSystemSearchHTTPUsesDatabaseAndLimitsResults(t *testing.T) {
	pool := testutil.Database(t)
	ctx := context.Background()
	var systems strings.Builder
	for i := range 35 {
		fmt.Fprintf(&systems, "{\"_key\":%d,\"name\":{\"en\":\"TEST-%02d\",\"zh\":\"测试星系%02d\"}}\n", 30020000+i, i, i)
	}
	if _, err := ImportSDENames(ctx, pool, sdeSystemFixture(t, 200, sampleTypes, systems.String()), 200); err != nil {
		t.Fatal(err)
	}
	h := &Handler{pool: pool}
	request := httptest.NewRequest(http.MethodGet, "/solar-systems/search?q=test", nil)
	request = request.WithContext(locale.With(request.Context(), "en"))
	response := httptest.NewRecorder()
	h.searchSolarSystems(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}
	var payload struct {
		Data []struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			NameZh string `json:"name_zh"`
			NameEn string `json:"name_en"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data) != 30 {
		t.Fatalf("expected a maximum of 30 results, got %d", len(payload.Data))
	}
	if first, last := payload.Data[0], payload.Data[29]; first.ID != "30020000" || first.Name != "TEST-00" || first.NameZh != "测试星系00" || first.NameEn != "TEST-00" || last.Name != "TEST-29" {
		t.Fatalf("unexpected order or localized payload: first=%+v last=%+v", first, last)
	}
}

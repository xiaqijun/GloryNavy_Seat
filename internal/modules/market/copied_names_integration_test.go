package market

import (
	"archive/zip"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/testutil"
)

func TestAppraisalCopiedInventoryThroughHTTPAndSDE(t *testing.T) {
	pool := testutil.Database(t)
	path := filepath.Join(t.TempDir(), "names.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	for name, data := range map[string]string{
		"_sde.jsonl": `{"_key":"sde","buildNumber":100}`,
		"types.jsonl": `{"_key":19421,"name":{"zh":"23级监察官的个人财产","en":"23rd Tier Overseer's Personal Effects"}}
{"_key":900001,"name":{"zh":"重名物品","en":"Test A"}}
{"_key":900002,"name":{"zh":"重名物品","en":"Test B"}}`,
		"mapSolarSystems.jsonl": `{"_key":30000142,"name":{"en":"Jita","zh":"吉他"}}`,
	} {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write([]byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = eve.ImportSDENames(context.Background(), pool, path, 100); err != nil {
		t.Fatal(err)
	}
	names := eve.NewStaticData(pool, "", 0)
	calls := 0
	h := Handler{Service: &Service{Pool: pool, Resolve: names.ResolveTypeNames, Prices: func(_ context.Context, id int64) (eve.MarketPrices, error) {
		if id != 19421 {
			t.Fatal("unexpected quote", id)
		}
		calls++
		return eve.MarketPrices{Buy: str("10"), Sell: str("12")}, nil
	}}}
	body, _ := json.Marshal(map[string]string{"text": "23级监察官的个人财产*\t3\n23级监察官的个人财产*\t2\n重名物品*\t1"})
	response := httptest.NewRecorder()
	h.estimate(response, httptest.NewRequest("POST", "/estimate", strings.NewReader(string(body))))
	var result struct {
		Data Appraisal `json:"data"`
	}
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &result) != nil {
		t.Fatal(response.Code, response.Body.String())
	}
	if calls != 1 || len(result.Data.Lines) != 3 || result.Data.Complete || result.Data.Totals.Mid != "55.00" {
		t.Fatal(result, calls)
	}
	for i, quantity := range []int64{3, 2} {
		line := result.Data.Lines[i]
		if line.Status != "ready" || line.TypeID != 19421 || line.Quantity != quantity || line.Name != "23级监察官的个人财产" {
			t.Fatal(line)
		}
	}
	if result.Data.Lines[2].Status != "unknown_type" || result.Data.Lines[2].Mid != nil {
		t.Fatal("ambiguous name guessed", result.Data.Lines[2])
	}
}

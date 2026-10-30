package eve

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"glorynavy.local/seat/internal/platform/locale"
	"glorynavy.local/seat/internal/testutil"
)

func TestSearchSolarSystemsUsesActiveReleaseAndBothLanguages(t *testing.T) {
	pool := testutil.Database(t)
	ctx := context.Background()
	if _, err := ImportSDENames(ctx, pool, sdeFixture(t, 100, sampleTypes), 100); err != nil {
		t.Fatal(err)
	}
	s := &StaticDataService{pool: pool}
	all, err := s.SearchSolarSystems(ctx, "")
	if err != nil || len(all) != 2 {
		t.Fatalf("empty query must list available systems: %#v, %v", all, err)
	}
	for _, term := range []string{"jIt", "吉他", "30000142"} {
		got, err := s.SearchSolarSystems(ctx, term)
		if err != nil || len(got) != 1 || got[0].ID != 30000142 || got[0].Name != "吉他" {
			t.Fatalf("%q: %#v, %v", term, got, err)
		}
	}
	got, err := s.SearchSolarSystems(locale.With(ctx, "en"), "吉他")
	if err != nil || len(got) != 1 || got[0].Name != "Jita" {
		t.Fatalf("English fallback: %#v, %v", got, err)
	}
	if got, err = s.SearchSolarSystems(ctx, "%_"); err != nil || len(got) != 0 {
		t.Fatalf("wildcards must be literal: %#v, %v", got, err)
	}
	updated := strings.Replace(sampleSystems, "吉他", "新名称", 1)
	if _, err = ImportSDENames(ctx, pool, sdeSystemFixture(t, 101, sampleTypes, updated), 101); err != nil {
		t.Fatal(err)
	}
	if got, err = s.SearchSolarSystems(ctx, "吉他"); err != nil || len(got) != 0 {
		t.Fatalf("inactive release must not be searched: %#v, %v", got, err)
	}
}

func TestSearchSolarSystemsRejectsOverlongQuery(t *testing.T) {
	req := httptest.NewRequest("GET", "/solar-systems/search?q="+strings.Repeat("x", 81), nil)
	response := httptest.NewRecorder()
	(&Handler{}).searchSolarSystems(response, req)
	if response.Code != 400 {
		t.Fatalf("expected 400, got %d", response.Code)
	}
}

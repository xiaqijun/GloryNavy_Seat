package eve

import (
	"archive/zip"
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"glorynavy.local/seat/internal/modules/eve/internal/store"
	"glorynavy.local/seat/internal/platform/locale"
	"glorynavy.local/seat/internal/testutil"
)

func sdeFixture(t *testing.T, build int64, types string, extra ...string) string {
	return sdeSystemFixture(t, build, types, sampleSystems, extra...)
}
func sdeSystemFixture(t *testing.T, build int64, types, systems string, extra ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sde.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	for _, v := range [][2]string{{"_sde.jsonl", fmt.Sprintf(`{"_key":"sde","buildNumber":%d}`, build)}, {"types.jsonl", types}, {"mapSolarSystems.jsonl", systems}} {
		w, e := z.Create(v[0])
		if e != nil {
			t.Fatal(e)
		}
		if _, e = w.Write([]byte(v[1])); e != nil {
			t.Fatal(e)
		}
	}
	for _, name := range extra {
		w, e := z.Create(name)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = w.Write([]byte("{}")); e != nil {
			t.Fatal(e)
		}
	}
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

const sampleSystems = `{"_key":30000142,"name":{"en":"Jita","zh":"吉他"}}
{"_key":30000143,"name":{"en":"Test System"}}`

const sampleTypes = `{"_key":0,"name":{"en":"#System","zh":"#星系"},"published":false}
{"_key":34,"name":{"en":"Tritanium","zh":"三钛合金"},"future_field":true}
{"_key":587,"name":{"en":"Rifter"}}`

func TestSDENamesRequestLocale(t *testing.T) {
	pool := testutil.Database(t)
	ctx := context.Background()
	if _, err := ImportSDENames(ctx, pool, sdeFixture(t, 100, sampleTypes), 100); err != nil {
		t.Fatal(err)
	}
	s := &StaticDataService{pool: pool}
	for _, language := range []string{"en", "zh-CN", "en"} {
		request := locale.With(ctx, language)
		names, err := s.TypeNames(request, []int64{34, 587})
		if err != nil {
			t.Fatal(err)
		}
		if names[34].Name != locale.Choose(request, "三钛合金", "Tritanium") || names[587].Name != "Rifter" || names[587].Language != "en" {
			t.Fatal(names)
		}
		places, err := s.SolarSystemNames(request, []int64{30000142})
		if err != nil {
			t.Fatal(err)
		}
		if places[30000142].Name != locale.Choose(request, "吉他", "Jita") {
			t.Fatal(places)
		}
		found, err := s.SearchTypes(request, "三钛合金")
		if err != nil || len(found) != 1 || found[0].Name != names[34].Name {
			t.Fatal(found, err)
		}
	}
}

func TestSDENamesPublicationAndFallback(t *testing.T) {
	pool := testutil.Database(t)
	ctx := context.Background()
	path := sdeFixture(t, 100, sampleTypes)
	first, err := ImportSDENames(ctx, pool, path, 100)
	if err != nil {
		t.Fatal(err)
	}
	if first.Count != 3 || first.Reused || len(first.SHA256) != 64 {
		t.Fatal(first)
	}
	again, err := ImportSDENames(ctx, pool, path, 100)
	if err != nil || !again.Reused || again.ID != first.ID {
		t.Fatal(again, err)
	}
	q := store.New(pool)
	for _, v := range []store.SaveEntityNameParams{
		{EntityID: 34, Name: "Tritanium", Category: "inventory_type", Language: "en"},
		{EntityID: 587, Name: "旧缓存中文", Category: "inventory_type", Language: "zh"},
		{EntityID: 999, Name: "ESI cached name", Category: "inventory_type", Language: "en"},
	} {
		if err = q.SaveEntityName(ctx, v); err != nil {
			t.Fatal(err)
		}
	}
	h := NewContractHTTP(pool, nil, nil)
	h.esi = newESI(testutil.Database(t), nil)
	calls := 0
	h.esi.http.Transport = transportFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, fmt.Errorf("offline") })
	refs := []*contractEntity{}
	for _, id := range []int64{34, 587, 999, 998} {
		v := entity(id, "inventory_type")
		refs = append(refs, &v)
	}
	h.names(ctx, refs...)
	for i, want := range []string{"三钛合金", "Rifter", "ESI cached name", ""} {
		if refs[i].Name != want {
			t.Fatalf("%d: got %+v", i, refs[i])
		}
	}
	if calls != 0 || refs[0].NameLanguage != "zh" || refs[1].NameLanguage != "en" {
		t.Fatal("SDE did not take precedence without ESI", calls)
	}
	names, err := store.ReadSDETypeNames(ctx, pool, []int64{0})
	if err != nil || len(names) != 1 {
		t.Fatal("unpublished ID zero was dropped", err)
	}

	second, err := ImportSDENames(ctx, pool, sdeFixture(t, 101, strings.ReplaceAll(sampleTypes, "三钛合金", "新版名称")), 101)
	if err != nil {
		t.Fatal(err)
	}
	names, err = store.ReadSDETypeNames(ctx, pool, []int64{34})
	if err != nil || names[0].Chinese != "新版名称" {
		t.Fatal("new release not activated", err)
	}
	if err = ActivateSDENames(ctx, pool, first.ID); err != nil {
		t.Fatal(err)
	}
	names, err = store.ReadSDETypeNames(ctx, pool, []int64{34})
	if err != nil || names[0].Chinese != "三钛合金" {
		t.Fatal("rollback failed", err)
	}
	if err = ActivateSDENames(ctx, pool, second.ID); err != nil {
		t.Fatal(err)
	}
	if err = ActivateSDENames(ctx, pool, 999999); err == nil {
		t.Fatal("unknown release activated")
	}

	var flushed strings.Builder
	for id := 1000; id < 2100; id++ {
		fmt.Fprintf(&flushed, "{\"_key\":%d,\"name\":{\"en\":\"fixture\"}}\n", id)
	}
	flushed.WriteString("{")

	tests := []struct {
		name, types     string
		build, expected int64
		extra           []string
	}{
		{"wrong build", sampleTypes, 102, 103, nil},
		{"duplicate type", sampleTypes + "\n" + `{"_key":34,"name":{"en":"duplicate"}}`, 102, 102, nil},
		{"missing ID", `{"name":{"en":"no ID"}}`, 102, 102, nil},
		{"missing English", `{"_key":34,"name":{"zh":"中文"}}`, 102, 102, nil},
		{"empty", "", 102, 102, nil},
		{"large drop", `{"_key":34,"name":{"en":"Only"}}`, 102, 102, nil},
		{"invalid JSON", sampleTypes + "\n{", 102, 102, nil},
		{"invalid after COPY batch", flushed.String(), 102, 102, nil},
		{"old build", sampleTypes, 99, 99, nil},
		{"duplicate entry", sampleTypes, 102, 102, []string{"types.jsonl"}},
		{"traversal", sampleTypes, 102, 102, []string{"../outside.jsonl"}},
		{"oversize line", `{"_key":34,"name":{"en":"` + strings.Repeat("x", 4<<20) + `"}}`, 102, 102, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ImportSDENames(ctx, pool, sdeFixture(t, tc.build, tc.types, tc.extra...), tc.expected); err == nil {
				t.Fatal("invalid import accepted")
			}
			var active, count int64
			if err := pool.QueryRow(ctx, `SELECT release_id FROM eve_sde_active_names`).Scan(&active); err != nil {
				t.Fatal(err)
			}
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM eve_sde_name_releases`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if active != second.ID || count != 2 {
				t.Fatal("failed import modified published data", active, count)
			}
		})
	}
}

func TestSDESolarSystemNamesAndAtomicFailure(t *testing.T) {
	pool := testutil.Database(t)
	ctx := context.Background()
	first, err := ImportSDENames(ctx, pool, sdeFixture(t, 100, sampleTypes), 100)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewStaticData(pool, t.TempDir(), 0)
	names, err := svc.SolarSystemNames(ctx, []int64{30000142, 30000143, 999})
	if err != nil || names[30000142].Name != "吉他" || names[30000143].Name != "Test System" || len(names) != 2 {
		t.Fatal(names, err)
	}
	for _, bad := range []string{"", `{"_key":30000142,"name":{"en":"Jita"}}` + "\n" + `{"_key":30000142,"name":{"en":"Duplicate"}}`, `{"_key":30000142,"name":{"zh":"缺英文"}}`} {
		if _, err = ImportSDENames(ctx, pool, sdeSystemFixture(t, 101, sampleTypes, bad), 101); err == nil {
			t.Fatal("invalid systems accepted")
		}
		status, e := svc.Status(ctx)
		if e != nil || status.ActiveReleaseID != first.ID {
			t.Fatal(status, e)
		}
	}
	// Simulate a retained mapper-1 release at the same build and archive hash.
	if _, err = pool.Exec(ctx, `UPDATE eve_sde_name_releases SET mapper_version=1,system_count=0 WHERE id=$1`, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `DELETE FROM eve_sde_solar_system_names WHERE release_id=$1`, first.ID); err != nil {
		t.Fatal(err)
	}
	second, err := ImportSDENames(ctx, pool, sdeFixture(t, 100, sampleTypes), 100)
	if err != nil || second.ID == first.ID || second.SystemCount != 2 {
		t.Fatal(second, err)
	}
	if err = ActivateSDENames(ctx, pool, first.ID); err != nil {
		t.Fatal(err)
	}
	names, err = svc.SolarSystemNames(ctx, []int64{30000142})
	if err != nil || len(names) != 0 {
		t.Fatal("old release must not mix new names", names, err)
	}
}

func TestSDESearchUsesActiveLocalNames(t *testing.T) {
	pool := testutil.Database(t)
	ctx := context.Background()
	svc := NewStaticData(pool, t.TempDir(), 0)
	if _, err := ImportSDENames(ctx, pool, sdeFixture(t, 100, sampleTypes), 100); err != nil {
		t.Fatal(err)
	}
	for _, term := range []string{"34", "三钛", "TRITANIUM"} {
		rows, err := svc.SearchTypes(ctx, term)
		if err != nil || len(rows) != 1 || rows[0].ID != 34 || rows[0].Name != "三钛合金" {
			t.Fatal(term, rows, err)
		}
	}
	for _, term := range []string{"", "%_", "#System"} {
		rows, err := svc.SearchTypes(ctx, term)
		if err != nil || len(rows) != 0 {
			t.Fatal(term, rows, err)
		}
	}
	if _, err := ImportSDENames(ctx, pool, sdeFixture(t, 101, strings.ReplaceAll(sampleTypes, "三钛合金", "新版名称")), 101); err != nil {
		t.Fatal(err)
	}
	rows, err := svc.SearchTypes(ctx, "三钛")
	if err != nil || len(rows) != 0 {
		t.Fatal("inactive names", rows, err)
	}
	rows, err = svc.SearchTypes(ctx, "Rifter")
	if err != nil || len(rows) != 1 || rows[0].Name != "Rifter" {
		t.Fatal(rows, err)
	}
}

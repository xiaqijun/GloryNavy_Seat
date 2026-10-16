package app

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/modules/identity"
)

func TestConfiguredCorporationHTTPBoundary(t *testing.T) {
	f := newCleanupFixture(t)
	ctx := context.Background()
	token, actor := f.sign(t, 101)
	otherToken, _ := f.sign(t, 202)
	externalToken, _ := f.sign(t, 303)
	for id, corp := range map[int64]int64{101: 10, 202: 20, 303: 30} {
		f.facts(t, id, corp)
	}
	f.exec(t, `UPDATE eve_role_snapshots SET corporation_name='Corporation '||corporation_id, roles=ARRAY['Director']`)
	f.exec(t, `UPDATE identity_corporation_cleanup SET corporation_ids=ARRAY[10,20]::bigint[]`)
	f.exec(t, `INSERT INTO access_roles(id,name,grants) VALUES('00000000-0000-4000-8000-000000000001','Broad manager','[{"permission":"corporation.welfare","corporations":[],"alliances":[]},{"permission":"corporation.welfare.compensation","corporations":[],"alliances":[]},{"permission":"corporation.journal","corporations":[],"alliances":[]}]')`)
	f.exec(t, `INSERT INTO access_assignments(user_id,role_id) VALUES($1,'00000000-0000-4000-8000-000000000001')`, actor.UserID)
	h, err := New(f.pool, slog.New(slog.NewTextHandler(io.Discard, nil)), "test", []string{"system", "identity", "eve", "access", "attendance", "skills", "exchange", "welfare", "wallet"}, AuthConfig{Origin: "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	get := func(tok, path string, status int) []byte {
		t.Helper()
		r := httptest.NewRequest("GET", path, nil)
		r.AddCookie(&http.Cookie{Name: httpapi.SessionCookie, Value: tok})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s: %d want %d: %s", path, w.Code, status, w.Body.String())
		}
		return w.Body.Bytes()
	}
	checkCorps := func(tok string, want []string) {
		t.Helper()
		for _, path := range []string{"/api/v1/welfare/context", "/api/v1/attendance/context", "/api/v1/skills/context"} {
			var out struct {
				Data struct {
					Corporations []struct {
						ID string `json:"id"`
					} `json:"corporations"`
				} `json:"data"`
			}
			if err := json.Unmarshal(get(tok, path, 200), &out); err != nil {
				t.Fatal(err)
			}
			ids := []string{}
			for _, c := range out.Data.Corporations {
				ids = append(ids, c.ID)
			}
			if !reflect.DeepEqual(ids, want) {
				t.Fatalf("%s corps %v want %v", path, ids, want)
			}
		}
	}
	checkCorps(token, []string{"10"})
	checkCorps(otherToken, []string{"20"})
	checkCorps(externalToken, []string{})
	get(externalToken, "/api/v1/welfare/losses?corporation_id=30&character_id=303", 404)
	get(token, "/api/v1/access/corporations/20/summary", 403)
	get(token, "/api/v1/eve/contracts/corporation/20", 404)
	get(token, "/api/v1/welfare/losses?corporation_id=30&character_id=303", 404)
	if err = f.acl.SetAdministrator(ctx, actor.UserID, true); err != nil {
		t.Fatal(err)
	}
	checkCorps(token, []string{"10", "20"})
	get(token, "/api/v1/access/corporations/20/summary", 200)
	get(token, "/api/v1/access/corporations/30/summary", 403)
	get(token, "/api/v1/welfare/losses?corporation_id=30&character_id=303", 404)
	settings, err := f.cleanup.Settings(ctx, actor.UserID)
	if err != nil || len(settings.CorporationOptions) != 3 || settings.CorporationOptions[0].Name != "Corporation 10" {
		t.Fatal("named configuration", settings, err)
	}
	// Saving a disabled cleanup policy still changes access immediately, without a restart.
	if _, err = f.cleanup.Save(ctx, actor.UserID, identity.CleanupInput{Enabled: false, CorporationIDs: []string{"20"}, Version: settings.Version}); err != nil {
		t.Fatal(err)
	}
	checkCorps(token, []string{"20"})
	get(token, "/api/v1/access/corporations/10/summary", 403)
	if err = f.acl.SetAdministrator(ctx, actor.UserID, false); err != nil {
		t.Fatal(err)
	}
	checkCorps(token, []string{})
	f.exec(t, `UPDATE identity_corporation_cleanup SET corporation_ids='{}'`)
	checkCorps(otherToken, []string{})
}

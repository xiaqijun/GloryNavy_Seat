package app

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/modules/access"
)

func TestAccessScopeOptionsHTTPNamesAndPermission(t *testing.T) {
	f := newCleanupFixture(t)
	token, actor := f.sign(t, 101)
	f.facts(t, 101, 10)
	_, _ = f.sign(t, 202)
	f.facts(t, 202, 20)
	f.exec(t, `UPDATE eve_role_snapshots SET corporation_name='Corporation '||corporation_id, alliance_id=100`)
	f.exec(t, `UPDATE identity_corporation_cleanup SET corporation_ids=ARRAY[10]::bigint[]`)
	f.exec(t, `INSERT INTO eve_entity_names(entity_id,name,category,language,expires_at) VALUES(100,'Connected Alliance','alliance','en',now()+interval '1 day')`)
	h, err := New(f.pool, slog.New(slog.NewTextHandler(io.Discard, nil)), "test", []string{"system", "identity", "eve", "access"}, AuthConfig{Origin: "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	get := func(tok string, status int) access.ScopeOptions {
		t.Helper()
		r := httptest.NewRequest("GET", "/api/v1/access/scope-options", nil)
		if tok != "" {
			r.AddCookie(&http.Cookie{Name: httpapi.SessionCookie, Value: tok})
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("status %d want %d: %s", w.Code, status, w.Body.String())
		}
		var payload struct {
			Data access.ScopeOptions `json:"data"`
		}
		if status == 200 {
			if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
		}
		return payload.Data
	}
	get("", 401)
	get(token, 403)
	if err = f.acl.SetAdministrator(context.Background(), actor.UserID, true); err != nil {
		t.Fatal(err)
	}
	out := get(token, 200)
	if len(out.Corporations) != 1 || out.Corporations[0].ID != 10 || out.Corporations[0].Name != "Corporation 10" || len(out.Alliances) != 1 || out.Alliances[0].Name != "Connected Alliance" {
		t.Fatalf("scope names: %+v", out)
	}
	f.exec(t, `UPDATE identity_corporation_cleanup SET corporation_ids='{}'`)
	out = get(token, 200)
	if len(out.Corporations) != 0 || len(out.Alliances) != 0 {
		t.Fatalf("empty configured scope: %+v", out)
	}
	if err = f.acl.SetAdministrator(context.Background(), actor.UserID, false); err != nil {
		t.Fatal(err)
	}
	get(token, 403)
}

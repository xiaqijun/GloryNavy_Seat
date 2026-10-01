package app

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/modules/access"
	"glorynavy.local/seat/internal/modules/community"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/testutil"
)

func TestAdministratorMemberReadAndDemotion(t *testing.T) {
	pool := testutil.Database(t)
	ctx := context.Background()
	users := identity.New(pool)
	acl := access.New(pool, nil, nil)
	create := func(id int64, name string) (string, *identity.Session) {
		t.Helper()
		token, err := users.SignIn(ctx, id, name, "test-owner", "")
		if err != nil {
			t.Fatal(err)
		}
		session, err := users.Session(ctx, token)
		if err != nil {
			t.Fatal(err)
		}
		return token, session
	}
	adminToken, admin := create(101, "Admin")
	otherToken, other := create(202, "Other")
	managerToken, manager := create(303, "Manager")
	if _, err := users.Complete(ctx, 203, "Other Alt", "test-owner", otherToken, identity.LoginIntent{Kind: "link", UserID: other.UserID}, nil); err != nil {
		t.Fatal(err)
	}
	if err := acl.SetAdministrator(ctx, admin.UserID, true); err != nil {
		t.Fatal(err)
	}
	role, err := acl.SaveRole(ctx, "local-operator", access.Role{ID: "01994763-4111-7000-8000-111111111111", Name: "Managers", Grants: []access.Grant{{Permission: "access.manage"}, {Permission: "eve.sync.manage"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err = acl.Assign(ctx, "local-operator", manager.UserID, role.ID, true); err != nil {
		t.Fatal(err)
	}
	for _, user := range []string{admin.UserID, other.UserID, manager.UserID} {
		if _, err := community.New(pool).Update(ctx, user, "123456", "Community "+user, "0"); err != nil {
			t.Fatal(err)
		}
	}
	for _, query := range []string{
		`INSERT INTO eve_contracts(owner_kind,owner_id,contract_id,source_character_id,source_generation,contract_type,status,payload) VALUES('character',202,9001,202,1,'auction','outstanding','{"title":"Other private contract"}'),('character',203,9001,203,1,'auction','outstanding','{"title":"Alt private contract"}')`,
		`INSERT INTO eve_contract_items(owner_kind,owner_id,contract_id,record_id,type_id,quantity,is_included,is_singleton) VALUES('character',202,9001,1,34,9007199254740993,true,false)`,
		`INSERT INTO eve_contract_bids(owner_kind,owner_id,contract_id,bid_id,bidder_id,amount,date_bid) VALUES('character',202,9001,1,202,9007199254740993,now())`,
	} {
		if _, err := pool.Exec(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	app, err := New(pool, slog.New(slog.NewTextHandler(io.Discard, nil)), "test", []string{"system", "identity", "eve", "access", "community"}, AuthConfig{Origin: "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	call := func(token, method, path string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, nil)
		if token != "" {
			req.AddCookie(&http.Cookie{Name: httpapi.SessionCookie, Value: token})
		}
		req.Header.Set("Origin", "https://example.com")
		if token == adminToken {
			req.Header.Set("X-CSRF-Token", admin.CSRFToken)
		}
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		return w
	}
	dataPath := "/api/v1/access/members/" + other.UserID + "/data"
	if w := call("", "GET", dataPath); w.Code != 401 {
		t.Fatal("anonymous member data", w.Code)
	}
	for _, token := range []string{otherToken, managerToken} {
		if w := call(token, "GET", dataPath); w.Code != 403 {
			t.Fatal("non-admin member data", w.Code, w.Body.String())
		}
	}
	w := call(adminToken, "GET", dataPath)
	var data struct {
		Data access.MemberData `json:"data"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &data) != nil || len(data.Data.Characters) != 2 || data.Data.Community == nil || data.Data.Community.KOOK.Value != "Community "+other.UserID {
		t.Fatal("admin member data", w.Code, w.Body.String())
	}
	for _, secret := range []string{"owner_hash", "csrf_token", "sealed", "refresh_token", "test-owner"} {
		if strings.Contains(w.Body.String(), secret) {
			t.Fatal("secret leaked", secret)
		}
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("private cache", w.Header())
	}
	if w = call(adminToken, "GET", "/api/v1/access/members/bad/data"); w.Code != 400 {
		t.Fatal("invalid member", w.Code)
	}
	if w = call(adminToken, "GET", "/api/v1/access/members/00000000-0000-0000-0000-000000000001/data"); w.Code != 404 {
		t.Fatal("missing member", w.Code)
	}
	ownersPath := "/api/v1/eve/contracts/owners?member=" + other.UserID
	if w = call(adminToken, "GET", ownersPath); w.Code != 200 || !strings.Contains(w.Body.String(), "Other Alt") {
		t.Fatal("member selector", w.Code, w.Body.String())
	}
	if w = call(managerToken, "GET", ownersPath); w.Code != 404 {
		t.Fatal("manager selector access", w.Code)
	}
	privatePaths := []string{"/api/v1/eve/contracts/character/202", "/api/v1/eve/contracts/character/202/9001", "/api/v1/eve/contracts/character/202/9001/items", "/api/v1/eve/contracts/character/202/9001/bids", "/api/v1/eve/sync/characters/202"}
	for _, path := range privatePaths {
		if w = call(adminToken, "GET", path); w.Code != 200 {
			t.Fatal("admin read", path, w.Code, w.Body.String())
		}
		for _, token := range []string{otherToken, managerToken} {
			want := 200
			if token == managerToken {
				want = 404
			}
			if w = call(token, "GET", path); w.Code != want {
				t.Fatal("owner/manager boundary", path, w.Code, w.Body.String())
			}
		}
	}
	for _, path := range []string{"/api/v1/eve/contracts/character/999", "/api/v1/eve/contracts/corporation/202", "/api/v1/eve/sync/characters/999"} {
		if w = call(adminToken, "GET", path); w.Code != 404 {
			t.Fatal("admin target validation", path, w.Code)
		}
	}
	for _, action := range []struct{ method, path string }{{"DELETE", "/api/v1/identity/characters/202"}, {"POST", "/api/v1/identity/characters/202/main"}, {"POST", "/api/v1/eve/sync/characters/202/refresh"}} {
		if w = call(adminToken, action.method, action.path); w.Code != 404 {
			t.Fatal("read became mutation", action.path, w.Code, w.Body.String())
		}
	}
	if w = call(adminToken, "GET", "/api/v1/identity/characters?user="+other.UserID); w.Code != 200 || strings.Contains(w.Body.String(), "Other") {
		t.Fatal("self identity changed")
	}
	if err = acl.SetAdministrator(ctx, admin.UserID, false); err != nil {
		t.Fatal(err)
	}
	if w = call(adminToken, "GET", dataPath); w.Code != 403 {
		t.Fatal("demotion data access", w.Code)
	}
	for _, path := range append(privatePaths, ownersPath) {
		if w = call(adminToken, "GET", path); w.Code != 404 {
			t.Fatal("demotion read access", path, w.Code)
		}
	}
	if err = acl.SetAdministrator(ctx, admin.UserID, true); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, "UPDATE identity_characters SET status='blocked' WHERE character_id=202"); err != nil {
		t.Fatal(err)
	}
	for _, path := range privatePaths {
		if w = call(adminToken, "GET", path); w.Code != 404 {
			t.Fatal("blocked character data", path, w.Code)
		}
	}
	if w = call(adminToken, "GET", ownersPath); w.Code != 200 || strings.Contains(w.Body.String(), `"id":"202"`) {
		t.Fatal("blocked selector", w.Code, w.Body.String())
	}
}

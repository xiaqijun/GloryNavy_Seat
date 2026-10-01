package app

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/testutil"
)

func TestMultiCharacterCorporationAccessRemainsScoped(t *testing.T) {
	pool := testutil.Database(t)
	ctx := context.Background()
	users := identity.New(pool)
	token, err := users.SignIn(ctx, 101, "A", "owner", "")
	if err != nil {
		t.Fatal(err)
	}
	session, _ := users.Session(ctx, token)
	_, err = users.Complete(ctx, 202, "B", "owner", token, identity.LoginIntent{Kind: "link", UserID: session.UserID}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{101, 202} {
		_, err = pool.Exec(ctx, "INSERT INTO eve_credentials(character_id,owner_hash,sealed,state) VALUES($1,$2,''::bytea,'ready')", id, httpapi.Hash("owner"))
		if err != nil {
			t.Fatal(err)
		}
		roles := []string{}
		if id == 101 {
			roles = []string{"Director"}
		}
		_, err = pool.Exec(ctx, "INSERT INTO eve_role_snapshots(character_id,owner_hash,corporation_id,corporation_name,ceo_id,roles,roles_at_hq,roles_at_base,roles_at_other,synced_at,valid_until) VALUES($1,$2,$3,'Corp',999,$4,'{}','{}','{}',now(),now()+interval '1 hour')", id, httpapi.Hash("owner"), id, roles)
		if err != nil {
			t.Fatal(err)
		}
	}
	app, err := New(pool, slog.New(slog.NewTextHandler(io.Discard, nil)), "test", []string{"system", "identity", "eve", "access"}, AuthConfig{Origin: "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	status := func(path string) int {
		r := httptest.NewRequest("GET", path, nil)
		r.AddCookie(&http.Cookie{Name: httpapi.SessionCookie, Value: token})
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		return w.Code
	}
	anonymous := httptest.NewRecorder()
	app.ServeHTTP(anonymous, httptest.NewRequest("GET", "/api/v1/eve/contracts/owners", nil))
	if anonymous.Code != 401 {
		t.Fatal("anonymous contract access allowed")
	}
	if status("/api/v1/access/corporations/101/summary") != 200 || status("/api/v1/access/corporations/202/summary") != 403 {
		t.Fatal("Director scope crossed corporations")
	}
	if status("/api/v1/eve/contracts/corporation/101") != 200 || status("/api/v1/eve/contracts/corporation/202") != 404 {
		t.Fatal("contract read scope crossed corporations")
	}
	if status("/api/v1/eve/contracts/character/101") != 200 || status("/api/v1/eve/contracts/character/999") != 404 {
		t.Fatal("personal contract read bypassed ownership")
	}
	if err = users.ChangeCharacter(ctx, token, 202, false, nil); err != nil {
		t.Fatal(err)
	}
	if status("/api/v1/access/corporations/101/summary") != 200 || status("/api/v1/access/corporations/202/summary") != 403 {
		t.Fatal("main changed permissions")
	}
	altToken, err := users.SignIn(ctx, 202, "B", "owner", "")
	if err != nil {
		t.Fatal(err)
	}
	token = altToken
	if err = users.ChangeCharacter(ctx, token, 101, true, eve.RemoveCharacterTx); err != nil {
		t.Fatal(err)
	}
	// The remaining user cannot access a removed character's corporation. Its snapshot
	// is gone, so the endpoint may report unavailable target data before policy denial.
	if code := status("/api/v1/access/corporations/101/summary"); code != 403 && code != 503 {
		t.Fatalf("removed role retained access: %d", code)
	}
	if status("/api/v1/access/corporations/202/summary") != 403 {
		t.Fatal("ordinary main gained permissions")
	}
	if status("/api/v1/eve/contracts/character/101") != 404 || status("/api/v1/eve/contracts/corporation/101") != 404 {
		t.Fatal("unlinked character retained contract read access")
	}
}

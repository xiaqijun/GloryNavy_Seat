package app

import (
	"context"
	"encoding/json"
	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/testutil"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWalletHostBindingAndCurrentAdministrator(t *testing.T) {
	pool := testutil.Database(t)
	ctx := context.Background()
	accounts := identity.New(pool)
	token, e := accounts.SignIn(ctx, 101, "Self", "owner", "")
	if e != nil {
		t.Fatal(e)
	}
	session, e := accounts.Session(ctx, token)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = accounts.SignIn(ctx, 102, "Other", "other", ""); e != nil {
		t.Fatal(e)
	}
	for _, ch := range []struct {
		id    int64
		owner string
	}{{101, "owner"}, {102, "other"}} {
		if _, e = pool.Exec(ctx, `INSERT INTO eve_credentials(character_id,owner_hash,sealed,scopes,state) VALUES($1,$2,'test',ARRAY['esi-wallet.read_character_wallet.v1'],'ready')`, ch.id, httpapi.Hash(ch.owner)); e != nil {
			t.Fatal(e)
		}
	}
	h, e := New(pool, slog.New(slog.NewTextHandler(io.Discard, nil)), "test", []string{"system", "identity", "eve", "access", "wallet"}, AuthConfig{Origin: "https://example.com"})
	if e != nil {
		t.Fatal(e)
	}
	call := func(path string, login bool) int {
		r := httptest.NewRequest("GET", path, nil)
		if login {
			r.AddCookie(&http.Cookie{Name: httpapi.SessionCookie, Value: token})
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	p := "/api/v1/wallet/records?owner_kind=character&part=journal&owner_id="
	if code := call(p+"101", false); code != 401 {
		t.Fatal("anonymous", code)
	}
	if code := call(p+"101", true); code != 200 {
		t.Fatal("self", code)
	}
	if code := call(p+"102", true); code != 404 {
		t.Fatal("other", code)
	}
	if _, e = pool.Exec(ctx, "INSERT INTO access_administrators(user_id) VALUES($1)", session.UserID); e != nil {
		t.Fatal(e)
	}
	if code := call(p+"102", true); code != 200 {
		t.Fatal("administrator", code)
	}
	_, _ = pool.Exec(ctx, "UPDATE eve_credentials SET owner_hash=$1 WHERE character_id=102", httpapi.Hash("transferred"))
	if code := call(p+"102", true); code != 404 {
		t.Fatal("owner mismatch", code)
	}
	_, _ = pool.Exec(ctx, "UPDATE eve_credentials SET owner_hash=$1 WHERE character_id=102", httpapi.Hash("other"))
	_, _ = pool.Exec(ctx, "DELETE FROM access_administrators WHERE user_id=$1", session.UserID)
	if code := call(p+"102", true); code != 404 {
		t.Fatal("revoked administrator", code)
	}
	if code := call("/api/v1/wallet/records?owner_kind=corporation&owner_id=10&division=1&part=journal", true); code != 404 {
		t.Fatal("self capability grants corporation", code)
	}
}

func TestWalletSummaryBatchesOwnCharactersAndUsesLatestJournalVariant(t *testing.T) {
	pool := testutil.Database(t)
	ctx := context.Background()
	accounts := identity.New(pool)
	token, e := accounts.SignIn(ctx, 201, "Main", "owner-a", "")
	if e != nil {
		t.Fatal(e)
	}
	session, e := accounts.Session(ctx, token)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = accounts.Complete(ctx, 202, "Alt", "owner-b", token, identity.LoginIntent{Kind: "link", UserID: session.UserID}, nil); e != nil {
		t.Fatal(e)
	}
	if _, e = accounts.SignIn(ctx, 203, "Other", "owner-c", ""); e != nil {
		t.Fatal(e)
	}
	for _, ch := range []struct {
		id    int64
		owner string
	}{{201, "owner-a"}, {202, "owner-b"}, {203, "owner-c"}} {
		if _, e = pool.Exec(ctx, `INSERT INTO eve_credentials(character_id,owner_hash,sealed,scopes,state) VALUES($1,$2,'test',ARRAY['esi-wallet.read_character_wallet.v1'],'ready')`, ch.id, httpapi.Hash(ch.owner)); e != nil {
			t.Fatal(e)
		}
	}
	observed := time.Now().UTC().Add(-time.Hour)
	for _, row := range []struct {
		id      int64
		part    string
		record  int64
		entry   string
		payload string
		at      time.Time
	}{
		{201, "balance", 0, "", `{"balance":1000.25}`, observed},
		{202, "balance", 0, "", `{"balance":0.75}`, observed},
		{203, "balance", 0, "", `{"balance":999999}`, observed},
		{201, "journal", 500, "a", `{"amount":10.00}`, observed.Add(-time.Minute)},
		{201, "journal", 500, "b", `{"amount":12.50}`, observed},
		{202, "journal", 501, "c", `{"amount":-2.25}`, observed},
	} {
		if _, e = pool.Exec(ctx, `INSERT INTO eve_wallet_observations(owner_kind,owner_id,division,kind,record_id,source_character_id,owner_hash,observed_at,occurred_at,payload,entry_key)
 VALUES('character',$1,0,$2,$3,$1,(SELECT owner_hash FROM eve_credentials WHERE character_id=$1),$4,$4,$5::jsonb,$6)`, row.id, row.part, row.record, row.at, row.payload, row.entry); e != nil {
			t.Fatal(e)
		}
	}
	h, e := New(pool, slog.New(slog.NewTextHandler(io.Discard, nil)), "test", []string{"system", "identity", "eve", "access", "wallet"}, AuthConfig{Origin: "https://example.com"})
	if e != nil {
		t.Fatal(e)
	}
	from := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
	request := func(login bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/api/v1/wallet/summary?from="+from, nil)
		if login {
			r.AddCookie(&http.Cookie{Name: httpapi.SessionCookie, Value: token})
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := request(false); w.Code != 401 {
		t.Fatal("anonymous", w.Code)
	}
	w := request(true)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var body struct {
		Data struct {
			Items []struct {
				OwnerID string `json:"owner_id"`
				Balance string `json:"balance"`
				Income  string `json:"income"`
				Expense string `json:"expense"`
			} `json:"items"`
		} `json:"data"`
	}
	if e = json.Unmarshal(w.Body.Bytes(), &body); e != nil {
		t.Fatal(e)
	}
	if len(body.Data.Items) != 2 || body.Data.Items[0].OwnerID != "201" || body.Data.Items[1].OwnerID != "202" ||
		body.Data.Items[0].Balance != "1000.25" || body.Data.Items[0].Income != "12.50" ||
		body.Data.Items[1].Balance != "0.75" || body.Data.Items[1].Expense != "2.25" {
		t.Fatal(body.Data.Items)
	}
	if _, e = pool.Exec(ctx, `UPDATE eve_credentials SET scopes='{}' WHERE character_id=202`); e != nil {
		t.Fatal(e)
	}
	w = request(true)
	if w.Code != 200 || strings.Contains(w.Body.String(), `"owner_id":"202"`) {
		t.Fatal("revoked scope exposed wallet", w.Code, w.Body.String())
	}
}

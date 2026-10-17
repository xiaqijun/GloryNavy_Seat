package community

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/modules/identity"
)

func oneBotFixture(t *testing.T) (*Service, *pgxpool.Pool, string, string) {
	t.Helper()
	s, pool, user, code := qqBindingFixture(t)
	s.LockAccounts = identity.New(pool).LockActiveAccounts
	if err := s.SetQQBotConfigKey(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))); err != nil {
		t.Fatal(err)
	}
	_, err := s.SaveOneBotSettings(context.Background(), OneBotSettingsInput{Enabled: true, APIBase: "http://127.0.0.1:3000", BotQQ: "999999", Token: strings.Repeat("t", 32), Secret: strings.Repeat("s", 32), Revision: "0", Groups: []OneBotGroup{{"888888", "Test group"}}})
	if err != nil {
		t.Fatal(err)
	}
	return s, pool, user, code
}
func oneBotBody(t *testing.T, id int, code string, edit func(map[string]any)) []byte {
	t.Helper()
	e := map[string]any{"time": time.Now().Unix(), "self_id": 999999, "post_type": "message", "message_type": "group", "sub_type": "normal", "user_id": 123456789, "group_id": 888888, "message_id": id, "message": []any{map[string]any{"type": "text", "data": map[string]string{"text": code}}}}
	if edit != nil {
		edit(e)
	}
	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
func oneBotRequest(s *Service, raw []byte, secret string) *httptest.ResponseRecorder {
	request := httptest.NewRequest("POST", "/api/v1/community/qq/onebot/webhook", bytes.NewReader(raw))
	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write(raw)
	request.Header.Set("X-Signature", "sha1="+hex.EncodeToString(mac.Sum(nil)))
	recorder := httptest.NewRecorder()
	Handler{Service: s}.oneBotWebhook(recorder, request)
	return recorder
}
func TestOneBotSignedBindingAndReplay(t *testing.T) {
	s, pool, user, code := oneBotFixture(t)
	raw := oneBotBody(t, 1, code, nil)
	first := oneBotRequest(s, raw, strings.Repeat("s", 32))
	if first.Code != 200 || !strings.Contains(first.Body.String(), "QQ 绑定成功") || !strings.Contains(first.Body.String(), `"type":"at"`) {
		t.Fatalf("binding %d %s", first.Code, first.Body.String())
	}
	duplicate := oneBotRequest(s, raw, strings.Repeat("s", 32))
	if duplicate.Body.String() != "{}" {
		t.Fatalf("duplicate replied: %s", duplicate.Body.String())
	}
	changed := oneBotRequest(s, oneBotBody(t, 1, code, func(e map[string]any) { e["user_id"] = 234567890 }), strings.Repeat("s", 32))
	if changed.Code != 503 {
		t.Fatal("replay conflict accepted", changed.Code)
	}
	profile, err := s.Get(context.Background(), user)
	if err != nil || profile.QQ.Confirmation != "confirmed" {
		t.Fatal(profile, err)
	}
	var bindings, events, confirmations int
	if err = pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM community_onebot_bindings),(SELECT count(*) FROM community_bot_events WHERE source='onebot'),(SELECT count(*) FROM community_confirmations WHERE source='onebot')`).Scan(&bindings, &events, &confirmations); err != nil || bindings != 1 || events != 1 || confirmations != 1 {
		t.Fatal(bindings, events, confirmations, err)
	}
	if _, err = s.Update(context.Background(), user, "234567890", "Binding Pilot", "1"); err != nil {
		t.Fatal(err)
	}
	profile, _ = s.Get(context.Background(), user)
	if profile.QQ.Confirmation != "pending" {
		t.Fatal("changed QQ stayed confirmed")
	}
}
func TestOneBotRejectsUntrustedOrUnrelatedMessages(t *testing.T) {
	s, pool, _, code := oneBotFixture(t)
	cases := []struct {
		name   string
		edit   func(map[string]any)
		secret string
		status int
	}{
		{"signature", nil, "bad", 401},
		{"bot", func(e map[string]any) { e["self_id"] = 111111 }, strings.Repeat("s", 32), 401},
		{"expired", func(e map[string]any) { e["time"] = time.Now().Add(-10 * time.Minute).Unix() }, strings.Repeat("s", 32), 401},
		{"group", func(e map[string]any) { e["group_id"] = 777777 }, strings.Repeat("s", 32), 200},
		{"self", func(e map[string]any) { e["user_id"] = 999999 }, strings.Repeat("s", 32), 200},
		{"private", func(e map[string]any) { e["message_type"] = "private" }, strings.Repeat("s", 32), 200},
		{"anonymous", func(e map[string]any) { e["anonymous"] = map[string]any{"id": 1} }, strings.Repeat("s", 32), 200},
		{"conversation", func(e map[string]any) {
			e["message"] = []oneBotSegment{{"text", map[string]string{"text": "hello " + code}}}
		}, strings.Repeat("s", 32), 200},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := oneBotRequest(s, oneBotBody(t, 100+i, code, tc.edit), tc.secret)
			if result.Code != tc.status || strings.Contains(result.Body.String(), "reply") {
				t.Fatal(result.Code, result.Body.String())
			}
		})
	}
	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM community_onebot_bindings").Scan(&n); err != nil || n != 0 {
		t.Fatal(n, err)
	}
	wrong := oneBotRequest(s, oneBotBody(t, 200, code, func(e map[string]any) { e["user_id"] = 234567890 }), strings.Repeat("s", 32))
	if !strings.Contains(wrong.Body.String(), "不一致") {
		t.Fatal(wrong.Body.String())
	}
	correct := oneBotRequest(s, oneBotBody(t, 201, code, nil), strings.Repeat("s", 32))
	if !strings.Contains(correct.Body.String(), "绑定成功") {
		t.Fatal("wrong sender consumed code", correct.Body.String())
	}
}
func TestOneBotConcurrentRetryRepliesOnce(t *testing.T) {
	s, _, _, code := oneBotFixture(t)
	raw := oneBotBody(t, 300, code, nil)
	var wg sync.WaitGroup
	out := make(chan string, 8)
	for range 8 {
		wg.Go(func() { out <- oneBotRequest(s, raw, strings.Repeat("s", 32)).Body.String() })
	}
	wg.Wait()
	close(out)
	replies := 0
	for response := range out {
		if strings.Contains(response, "绑定成功") {
			replies++
		} else if response != "{}" {
			t.Fatal(response)
		}
	}
	if replies != 1 {
		t.Fatal("duplicate replies", replies)
	}
}
func TestOneBotSettingsEncryptionConflictAndLegacyIsolation(t *testing.T) {
	s, pool, user, code := oneBotFixture(t)
	ctx := context.Background()
	// A legacy proof must not become proof of a numeric QQ identity.
	if err := s.ConfirmQQOfficial(ctx, "legacy", "old-openid", code, []byte("old")); err != nil {
		t.Fatal(err)
	}
	profile, err := s.Get(ctx, user)
	if err != nil || profile.QQ.Confirmation != "pending" {
		t.Fatal(profile, err)
	}
	settings, err := s.ListOneBotSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(settings)
	if bytes.Contains(data, []byte(strings.Repeat("s", 32))) || bytes.Contains(data, []byte(strings.Repeat("t", 32))) {
		t.Fatal("secret returned")
	}
	var cipher []byte
	if err = pool.QueryRow(ctx, "SELECT token_ciphertext FROM community_onebot_settings").Scan(&cipher); err != nil || bytes.Contains(cipher, []byte(strings.Repeat("t", 32))) {
		t.Fatal("plaintext storage", err)
	}
	input := OneBotSettingsInput{Enabled: false, APIBase: settings.APIBase, BotQQ: settings.BotQQ, Revision: settings.Revision, Groups: settings.Groups}
	saved, err := s.SaveOneBotSettings(ctx, input)
	if err != nil || saved.Enabled || !saved.TokenConfigured || !saved.SecretConfigured {
		t.Fatal(saved, err)
	}
	if _, err = s.SaveOneBotSettings(ctx, input); !errors.Is(err, ErrVersion) {
		t.Fatal("stale settings accepted", err)
	}
	profile, _ = s.Get(ctx, user)
	if profile.QQ.Confirmation != "pending" {
		t.Fatal("disabled bot restored old confirmation")
	}
	response := httptest.NewRecorder()
	Handler{Service: s}.officialQQWebhook(response, httptest.NewRequest("POST", "/", strings.NewReader(`{"op":13}`)))
	if response.Code != 503 {
		t.Fatal("legacy callback accepted")
	}
	groups, err := s.ListQQGroupOptions(ctx)
	if err != nil || len(groups.Items) != 0 {
		t.Fatal(groups, err)
	}
}
func TestOneBotBindingRollbackAndAccountGuard(t *testing.T) {
	s, pool, user, code := oneBotFixture(t)
	ctx := context.Background()
	s.LockAccounts = func(context.Context, pgx.Tx, []string) error { return errors.New("merged") }
	response := oneBotRequest(s, oneBotBody(t, 401, code, nil), strings.Repeat("s", 32))
	if !strings.Contains(response.Body.String(), "账号状态已变更") {
		t.Fatal(response.Body.String())
	}
	s.LockAccounts = identity.New(pool).LockActiveAccounts
	_, err := pool.Exec(ctx, `CREATE FUNCTION fail_onebot_binding() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test'; END $$; CREATE TRIGGER fail_onebot_binding BEFORE INSERT ON community_onebot_bindings FOR EACH ROW EXECUTE FUNCTION fail_onebot_binding();`)
	if err != nil {
		t.Fatal(err)
	}
	raw := oneBotBody(t, 402, code, nil)
	response = oneBotRequest(s, raw, strings.Repeat("s", 32))
	if response.Code != 503 {
		t.Fatal(response.Code)
	}
	var n int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM community_qq_link_challenges WHERE used_at IS NOT NULL").Scan(&n); err != nil || n != 0 {
		t.Fatal("rollback consumed code", n, err)
	}
	if _, err = pool.Exec(ctx, "DROP TRIGGER fail_onebot_binding ON community_onebot_bindings"); err != nil {
		t.Fatal(err)
	}
	response = oneBotRequest(s, raw, strings.Repeat("s", 32))
	if !strings.Contains(response.Body.String(), "绑定成功") {
		t.Fatal(response.Body.String())
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = s.MergeAccountTx(ctx, tx, user, "unused", true); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := s.Get(ctx, user)
	if err != nil || p.QQ.Confirmation != "pending" {
		t.Fatal("merge left proof", p, err)
	}
}
func TestOneBotClientUsesSegmentsAndRejectsRedirects(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("missing authorization")
		}
		var body struct {
			GroupID int64           `json:"group_id"`
			Message []oneBotSegment `json:"message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if r.URL.Path != "/send_group_msg" || body.GroupID != 888888 || len(body.Message) != 2 || body.Message[0].Type != "at" || body.Message[0].Data["qq"] != "123456789" {
			t.Errorf("invalid send: %+v", body)
		}
		fmt.Fprint(w, `{"status":"ok","retcode":0,"data":{"message_id":-1234}}`)
	}))
	defer server.Close()
	client := oneBotClient{server.URL, "test-token"}
	if err := client.send(context.Background(), "888888", "123456789", "test"); err != nil {
		t.Fatal(err)
	}
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, server.URL+"/send_group_msg", 307) }))
	defer redirect.Close()
	if err := (oneBotClient{redirect.URL, "test-token"}).send(context.Background(), "888888", "123456789", "test"); err == nil {
		t.Fatal("redirect accepted")
	}
	if calls != 1 {
		t.Fatal("redirect leaked token or send retried", calls)
	}
}

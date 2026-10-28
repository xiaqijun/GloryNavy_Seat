package community

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func useOfficialQQTestBot(t *testing.T, s *Service, bot *OfficialQQBot) {
	t.Helper()
	if err := s.SetQQBotConfigKey(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))); err != nil {
		t.Fatal(err)
	}
	appID, secret, base := bot.configSnapshot()
	cipher, err := s.sealQQBotSecret(secret)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.pool.Exec(context.Background(), `UPDATE community_qq_bot_settings SET app_id=$1,secret_ciphertext=$2,api_base=$3,enabled=true`, appID, cipher, base); err != nil {
		t.Fatal(err)
	}
	s.qqOfficial = bot
}

func TestOfficialQQSettingsStayDisabledAndRejectStaleSave(t *testing.T) {
	s, _, user, _ := qqBindingFixture(t)
	ctx := context.Background()
	if err := s.SetQQBotConfigKey(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))); err != nil {
		t.Fatal(err)
	}
	initial, err := s.ListQQBotSettings(ctx)
	if err != nil || initial.Enabled || initial.Revision != "1" {
		t.Fatal(initial, err)
	}
	input := QQBotSettingsInput{AppID: "test-app", APIBase: defaultQQBotAPIBase, Secret: "test-secret", Revision: initial.Revision}
	saved, err := s.SaveQQBotSettings(ctx, input)
	if err != nil || saved.Enabled || !saved.SecretConfigured || s.qqOfficial.Configured() {
		t.Fatal(saved, err)
	}
	if _, err = s.SaveQQBotSettings(ctx, input); !errors.Is(err, ErrVersion) {
		t.Fatal("stale save", err)
	}
	input.Revision = saved.Revision
	input.Secret = ""
	input.Enabled = true
	saved, err = s.SaveQQBotSettings(ctx, input)
	if err != nil || !saved.Enabled || !s.qqOfficial.Configured() {
		t.Fatal(saved, err)
	}
	code, _, err := s.CreateQQOfficialChallenge(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := s.qqOfficial.callbackSnapshot()
	input.Revision = saved.Revision
	input.Enabled = false
	saved, err = s.SaveQQBotSettings(ctx, input)
	if err != nil || saved.Enabled || !saved.SecretConfigured || s.qqOfficial.Configured() {
		t.Fatal(saved, err)
	}
	if err = s.ConfirmQQOfficial(ctx, "disabled-event", "member", code, []byte("disabled-event"), snapshot); !errors.Is(err, ErrQQOfficialDisabled) {
		t.Fatal("late disabled callback", err)
	}
	input.Revision = saved.Revision
	input.Enabled = true
	saved, err = s.SaveQQBotSettings(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ConfirmQQOfficial(ctx, "current-event", "member", code, []byte("current-event"), s.qqOfficial.callbackSnapshot()); err != nil {
		t.Fatal(err)
	}
	input.Revision = saved.Revision
	input.AppID = "new-app"
	if _, err = s.SaveQQBotSettings(ctx, input); !errors.Is(err, ErrQQBotSecretMissing) {
		t.Fatal("new app reused old secret", err)
	}
	input.Secret = "new-secret"
	if _, err = s.SaveQQBotSettings(ctx, input); err != nil {
		t.Fatal(err)
	}
	p, err := s.Get(ctx, user)
	if err != nil || p.QQ.Confirmation != "pending" {
		t.Fatal("old app identity retained", p, err)
	}
}

func TestOfficialQQTokenStringLifetimeAndImmutableCallback(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		var input map[string]string
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": input["appId"] + "-token", "expires_in": "7200"})
	}))
	defer server.Close()
	b := newOfficialQQBot("first", "first-secret", server.URL)
	for range 2 {
		if token, err := b.accessToken(context.Background()); err != nil || token != "first-token" {
			t.Fatal(token, err)
		}
	}
	if requests.Load() != 1 || time.Until(b.token.ExpiresAt) < time.Hour {
		t.Fatal("token cache lifetime", requests.Load(), b.token.ExpiresAt)
	}
	copy := b.callbackSnapshot()
	b.Reconfigure("second", "second-secret", server.URL)
	if app, _, _ := copy.configSnapshot(); app != "first" {
		t.Fatal("mutable callback", app)
	}
	if token, err := b.accessToken(context.Background()); err != nil || token != "second-token" || requests.Load() != 2 {
		t.Fatal(token, requests.Load(), err)
	}
}

func TestOneBotRetiredURLsDoNotBindOrSend(t *testing.T) {
	h := Handler{}
	for _, route := range h.Module().Routes {
		if !strings.HasPrefix(route.Path, "/qq/onebot/") {
			continue
		}
		w := httptest.NewRecorder()
		route.Handler.ServeHTTP(w, httptest.NewRequest(route.Method, route.Path, strings.NewReader(`{}`)))
		if w.Code != http.StatusGone || !strings.Contains(w.Body.String(), "qq_transport_retired") {
			t.Fatal(route.Path, w.Code, w.Body.String())
		}
	}
}

func TestOfficialQQRestoresCallbackAndProofAfterOneBot(t *testing.T) {
	s, _, user, code := oneBotFixture(t)
	bot, _ := qqMessageTestBot(t)
	s.qqOfficial = bot
	w := httptest.NewRecorder()
	Handler{Service: s}.officialQQWebhook(w, httptest.NewRequest("POST", "/", strings.NewReader(`{"op":13,"d":{"plain_token":"verify-me","event_ts":"123"}}`)))
	if w.Code != 200 {
		t.Fatalf("historical OneBot settings block official callback: %d %s", w.Code, w.Body.String())
	}
	if err := s.ConfirmQQOfficial(context.Background(), "official-restored", "official-member", code, []byte("official-restored")); err != nil {
		t.Fatal(err)
	}
	p, err := s.Get(context.Background(), user)
	if err != nil || p.QQ.Confirmation != "confirmed" {
		t.Fatalf("official proof ignored after OneBot: %+v %v", p, err)
	}
}

package community

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestQQGroupNotificationOutcomes(t *testing.T) {
	status, response := 200, `{"id":"message"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/groups/group-openid/messages" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["content"] != "test" || payload["msg_id"] != nil || payload["event_id"] != nil {
			t.Errorf("unexpected proactive payload: %v", payload)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()
	bot := newOfficialQQBot("app", "secret", server.URL)
	bot.HTTP = server.Client()
	bot.token = qqBotToken{AccessToken: "token", ExpiresAt: time.Now().Add(time.Hour)}
	if got := sendQQGroupNotification(t.Context(), bot, "group-openid", "test"); got.State != "sent" {
		t.Fatalf("success: %+v", got)
	}
	status, response = 400, `{"code":40034105}`
	if got := sendQQGroupNotification(t.Context(), bot, "group-openid", "test"); got.State != "rejected" || got.Code != 40034105 {
		t.Fatalf("rejection: %+v", got)
	}
	status, response = 500, `{"code":1001}`
	if got := sendQQGroupNotification(t.Context(), bot, "group-openid", "test"); got.State != "unknown" {
		t.Fatalf("server failure: %+v", got)
	}
	status, response = 202, `{}`
	if got := sendQQGroupNotification(t.Context(), bot, "group-openid", "test"); got.State != "unknown" {
		t.Fatalf("async response: %+v", got)
	}
}

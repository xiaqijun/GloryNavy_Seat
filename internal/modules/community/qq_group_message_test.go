package community

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

type qqMessageReply struct {
	Path      string
	Content   string `json:"content"`
	MessageID string `json:"msg_id"`
}

func qqMessageTestBot(t *testing.T) (*OfficialQQBot, <-chan qqMessageReply) {
	t.Helper()
	replies := make(chan qqMessageReply, 32)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reply qqMessageReply
		if err := json.NewDecoder(r.Body).Decode(&reply); err != nil {
			t.Error(err)
		}
		reply.Path = r.URL.Path
		replies <- reply
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0}`))
	}))
	t.Cleanup(server.Close)
	bot := newOfficialQQBot("test-app", "test-secret", server.URL)
	bot.HTTP = server.Client()
	bot.token = qqBotToken{AccessToken: "test-token", ExpiresAt: time.Now().Add(time.Hour)}
	return bot, replies
}

func qqMessageTestRequest(t *testing.T, bot *OfficialQQBot, eventType, eventID string, message officialQQMessage) *http.Request {
	t.Helper()
	data, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(officialQQEnvelope{Op: 0, Type: eventType, ID: eventID, Data: data})
	if err != nil {
		t.Fatal(err)
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	signature, err := bot.ValidationSignature(timestamp, string(body))
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/community/qq/official/webhook", bytes.NewReader(body))
	req.Header.Set("X-Signature-Timestamp", timestamp)
	req.Header.Set("X-Signature-Ed25519", signature)
	return req
}

func TestQQGroupMessageIgnoresChatAndIncompleteIdentity(t *testing.T) {
	bot, replies := qqMessageTestBot(t)
	// No database: unrelated chat must not even enter the binding transaction.
	h := Handler{Service: &Service{qqOfficial: bot}}
	for _, content := range []string{"", "大家好", "ABCDEFG", "ABCDEFGHJ", "12345678", "绑定 ABCDEFGH", "ABCDEFGH 多余内容", "我的验证码 ABCDEFGH", "<@123> ABCDEFGH", "ABCD\nEFGH", "ＡＢＣＤＥＦＧＨ", "ABCD​EFGH"} {
		message := officialQQMessage{ID: "message", Content: content, GroupOpenID: "group"}
		message.Author.MemberOpenID = "member"
		res := httptest.NewRecorder()
		h.officialQQWebhook(res, qqMessageTestRequest(t, bot, "GROUP_MESSAGE_CREATE", "event", message))
		if res.Code != http.StatusOK || res.Body.String() != `{"op":12,"d":0}` {
			t.Fatalf("chat %q: %d %s", content, res.Code, res.Body.String())
		}
	}
	for _, missing := range []string{"event", "message", "member", "group", "bot"} {
		message := officialQQMessage{ID: "message", Content: "ABCDEFGH", GroupOpenID: "group"}
		message.Author.MemberOpenID = "member"
		eventID := "event"
		switch missing {
		case "event":
			eventID = ""
		case "message":
			message.ID = ""
		case "member":
			message.Author.MemberOpenID = ""
			message.Author.UserOpenID = "wrong-scope"
		case "group":
			message.GroupOpenID = ""
		case "bot":
			message.Author.Bot = true
		}
		res := httptest.NewRecorder()
		h.officialQQWebhook(res, qqMessageTestRequest(t, bot, "GROUP_MESSAGE_CREATE", eventID, message))
		if res.Code != http.StatusOK {
			t.Fatalf("%s status=%d", missing, res.Code)
		}
	}
	select {
	case reply := <-replies:
		t.Fatalf("unrelated message triggered reply: %+v", reply)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestQQGroupMessageRequiresSignature(t *testing.T) {
	bot, replies := qqMessageTestBot(t)
	h := Handler{Service: &Service{qqOfficial: bot}}
	message := officialQQMessage{ID: "message", Content: "ABCDEFGH", GroupOpenID: "group"}
	message.Author.MemberOpenID = "member"
	for _, header := range []string{"X-Signature-Ed25519", "X-Signature-Timestamp"} {
		req := qqMessageTestRequest(t, bot, "GROUP_MESSAGE_CREATE", "event", message)
		req.Header.Set(header, "invalid")
		res := httptest.NewRecorder()
		h.officialQQWebhook(res, req)
		if res.Code != http.StatusUnauthorized {
			t.Fatalf("%s status=%d", header, res.Code)
		}
	}
	select {
	case reply := <-replies:
		t.Fatalf("unsigned message triggered reply: %+v", reply)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestQQMessageBindingUsesSenderAndKeepsLegacyEvents(t *testing.T) {
	s, pool, user, firstCode := qqBindingFixture(t)
	bot, replies := qqMessageTestBot(t)
	useOfficialQQTestBot(t, s, bot)
	h := Handler{Service: s}
	for i, eventType := range []string{"GROUP_MESSAGE_CREATE", "GROUP_AT_MESSAGE_CREATE", "C2C_MESSAGE_CREATE"} {
		code := firstCode
		if i > 0 {
			var err error
			code, _, err = s.CreateQQOfficialChallenge(t.Context(), user)
			if err != nil {
				t.Fatal(err)
			}
		}
		message := officialQQMessage{ID: "message-" + eventType, GroupOpenID: "group"}
		message.Author.UserOpenID = "user"
		message.Author.MemberOpenID = "member"
		message.Content = "  " + strings.ToLower(code) + "\n"
		if eventType == "GROUP_AT_MESSAGE_CREATE" {
			message.Content = "<@test-app> 绑定 " + code
		}
		if eventType == "C2C_MESSAGE_CREATE" {
			message.Content = "绑定 " + code
		}
		request := func(eventID string) *httptest.ResponseRecorder {
			res := httptest.NewRecorder()
			h.officialQQWebhook(res, qqMessageTestRequest(t, bot, eventType, eventID, message))
			if res.Code != http.StatusOK {
				t.Fatalf("%s: %d %s", eventType, res.Code, res.Body.String())
			}
			return res
		}
		request(eventType)
		wantedID, wantedPath := "member", "/v2/groups/group/messages"
		if eventType == "C2C_MESSAGE_CREATE" {
			wantedID, wantedPath = "user", "/v2/users/user/messages"
		}
		readReply := func(want string) {
			t.Helper()
			select {
			case reply := <-replies:
				if reply.Path != wantedPath || reply.MessageID != message.ID || !strings.Contains(reply.Content, want) {
					t.Fatalf("unexpected reply: %+v", reply)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("missing reply")
			}
		}
		readReply("QQ 已绑定")
		request(eventType) // Identical signed retry remains idempotent.
		readReply("QQ 已绑定")
		message.Author.MemberOpenID, message.Author.UserOpenID = "another-member", "another-user"
		request(eventType + "-used")
		if eventType == "C2C_MESSAGE_CREATE" {
			wantedPath = "/v2/users/another-user/messages"
		}
		readReply("绑定码无效或已过期")
		var openid string
		var used bool
		var events int
		err := pool.QueryRow(t.Context(), `SELECT (SELECT openid FROM community_qq_bindings WHERE user_id=$1), (SELECT used_at IS NOT NULL FROM community_qq_link_challenges WHERE code_hash=$2), (SELECT count(*) FROM community_bot_events)`, user, officialChallengeHash(code)).Scan(&openid, &used, &events)
		if err != nil || openid != wantedID || !used || events != i+1 {
			t.Fatalf("binding %s: openid=%s used=%v events=%d err=%v", eventType, openid, used, events, err)
		}
	}
}

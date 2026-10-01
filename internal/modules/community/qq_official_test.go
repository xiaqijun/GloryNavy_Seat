package community

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOfficialQQWebhookValidationDoesNotRequireEventHeaders(t *testing.T) {
	bot := newOfficialQQBot("app-id", "official-secret", "")
	h := Handler{Service: &Service{qqOfficial: bot}}
	reqBody := []byte(`{"op":13,"d":{"plain_token":"plain","event_ts":"1725442341"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/community/qq/official/webhook", bytes.NewReader(reqBody))
	res := httptest.NewRecorder()
	h.officialQQWebhook(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	var response struct {
		PlainToken string `json:"plain_token"`
		Signature  string `json:"signature"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.PlainToken != "plain" || response.Signature == "" {
		t.Fatalf("unexpected validation response: %+v", response)
	}
	if _, err := bot.ValidationSignature("1725442341", "plain"); err != nil || response.Signature == "" {
		t.Fatal("validation signature missing")
	}
}

func TestOfficialQQReplyUsesGroupOpenIDAndChecksBusinessErrors(t *testing.T) {
	var path string
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &payload)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"err_code":0,"id":"msg"}`))
	}))
	defer server.Close()
	bot := newOfficialQQBot("app-id", "official-secret", server.URL)
	bot.HTTP = server.Client()
	bot.token = qqBotToken{AccessToken: "access", ExpiresAt: time.Now().Add(time.Hour)}
	if err := bot.Reply(t.Context(), "group", "group-openid", "message-id", "ok"); err != nil {
		t.Fatal(err)
	}
	if path != "/v2/groups/group-openid/messages" || payload["msg_id"] != "message-id" {
		t.Fatalf("path=%s payload=%v", path, payload)
	}
	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"err_code":40034005,"message":"expired","trace_id":"trace"}`))
	})
	if err := bot.Reply(t.Context(), "c2c", "user-openid", "message-id", "again"); err == nil || !strings.Contains(err.Error(), "40034005") {
		t.Fatalf("business error not returned: %v", err)
	}
}

func TestOfficialQQAccessTokenUsesConfiguredAPIBase(t *testing.T) {
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"access","expires_in":7200}`))
	}))
	defer server.Close()
	bot := newOfficialQQBot("app-id", "official-secret", server.URL)
	bot.HTTP = server.Client()
	token, err := bot.accessToken(t.Context())
	if err != nil || token != "access" {
		t.Fatalf("token=%q err=%v", token, err)
	}
	if path != "/app/getAppAccessToken" {
		t.Fatalf("token path=%s", path)
	}
}

func TestOfficialQQGroupAdmissionUsesOfficialEndpoints(t *testing.T) {
	var paths []string
	var approval map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.String())
		if r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &approval)
		}
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"list":[],"next_cursor":""}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":0}`))
	}))
	defer server.Close()
	bot := newOfficialQQBot("app-id", "official-secret", server.URL)
	bot.HTTP = server.Client()
	bot.token = qqBotToken{AccessToken: "access", ExpiresAt: time.Now().Add(time.Hour)}
	bot.SetGroups([]string{"group-openid"})
	if err := bot.ApproveGroupJoinRequest(t.Context(), "group-openid", "member-openid", "request-id"); err != nil {
		t.Fatal(err)
	}
	if _, err := bot.GroupJoinRequests(t.Context(), "group-openid", "", 20); err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || paths[0] != "POST /v2/groups/group-openid/approval_join_request/member-openid" || !strings.Contains(paths[1], "GET /v2/groups/group-openid/join_request_list") {
		t.Fatalf("paths=%v", paths)
	}
	if approval["op"] != "approve" || approval["join_request_id"] != "request-id" {
		t.Fatalf("approval payload=%v", approval)
	}
}

func TestQQGroupCodeReadsVerificationMessageAndQuestions(t *testing.T) {
	if got := qqGroupCodeFromVerifyInfo(officialQQVerifyInfo{VerifyMessage: "申请码 GNV4X7Q2"}); got != "GNV4X7Q2" {
		t.Fatalf("verify message code=%q", got)
	}
	if got := qqGroupCodeFromVerifyInfo(officialQQVerifyInfo{VerifyMessage: "申请码：GNV4X7Q2。"}); got != "GNV4X7Q2" {
		t.Fatalf("punctuated verify message code=%q", got)
	}
	if got := qqGroupCodeFromVerifyInfo(officialQQVerifyInfo{ReviewQAList: []struct {
		Question string `json:"question"`
		Answer   string `json:"answer"`
	}{{Question: "申请码", Answer: "GNV4X7Q2"}}}); got != "GNV4X7Q2" {
		t.Fatalf("review answer code=%q", got)
	}
}

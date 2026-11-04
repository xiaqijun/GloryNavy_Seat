package community

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"glorynavy.local/seat/internal/modules/eve"
)

func TestQQStructureMarkdownDelivery(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		response  string
		wantState string
		wantCalls int
	}{
		{"accepted", 200, `{"id":"message"}`, "sent", 1},
		{"no markdown permission", 400, `{"code":50056}`, "sent", 2},
		{"proactive rejected", 400, `{"code":40034105}`, "rejected", 1},
		{"uncertain", 500, `{"code":50056}`, "unknown", 1},
		{"accepted without id", 202, `{}`, "unknown", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var payload struct {
					Type     int    `json:"msg_type"`
					Content  string `json:"content"`
					Markdown struct {
						Content string `json:"content"`
					} `json:"markdown"`
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatal(err)
				}
				if calls == 1 {
					if payload.Type != 2 || !strings.Contains(payload.Markdown.Content, "**建筑类型：**阿塔诺") || strings.Contains(payload.Markdown.Content, "<qqbot-at-user") {
						t.Errorf("unsafe or unexpected Markdown: %+v", payload)
					}
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(tc.response))
					return
				}
				if payload.Type != 0 || !strings.Contains(payload.Content, "建筑类型：阿塔诺") {
					t.Errorf("unexpected fallback: %+v", payload)
				}
				_, _ = w.Write([]byte(`{"id":"fallback"}`))
			}))
			defer server.Close()
			bot := newOfficialQQBot("app", "secret", server.URL)
			bot.HTTP = server.Client()
			bot.token = qqBotToken{AccessToken: "token", ExpiresAt: time.Now().Add(time.Hour)}
			result := sendQQStructureMarkdown(t.Context(), bot, "group-openid", []string{"SEAT 建筑提醒", "建筑：<qqbot-at-user id=123>\n建筑类型：阿塔诺"})
			if result.State != tc.wantState || calls != tc.wantCalls {
				t.Fatalf("result=%+v calls=%d", result, calls)
			}
		})
	}
}

func TestStructureStateNotificationUsesBeijingTime(t *testing.T) {
	s, pool, user, _ := qqBindingFixture(t)
	ctx := context.Background()
	var message string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Markdown struct {
				Content string `json:"content"`
			} `json:"markdown"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		message = payload.Markdown.Content
		_, _ = w.Write([]byte(`{"id":"message"}`))
	}))
	defer server.Close()
	bot := newOfficialQQBot("test-app", "test-secret", server.URL)
	bot.HTTP = server.Client()
	bot.token = qqBotToken{AccessToken: "test-token", ExpiresAt: time.Now().Add(time.Hour)}
	useOfficialQQTestBot(t, s, bot)
	if _, err := pool.Exec(ctx, `UPDATE community_qq_structure_watch SET enabled=true,app_id='test-app',group_openid='group-openid',corporation_id=42,owner_id=$1 WHERE id`, user); err != nil {
		t.Fatal(err)
	}
	observed := time.Date(2026, 11, 3, 16, 27, 0, 0, time.UTC)
	row := eve.Structure{ID: 100, CorporationID: 42, Name: "Test", State: "online", ObservedAt: observed}
	job := &StructureJobs{Service: s, Enabled: true, Administrator: func(context.Context, string) (bool, error) { return true, nil }, Read: func(context.Context, string, int64) ([]eve.Structure, error) { return []eve.Structure{row}, nil }}
	if err := job.scan(ctx); err != nil {
		t.Fatal(err)
	}
	row.State, row.ObservedAt = "offline", observed.Add(time.Minute)
	if err := job.scan(ctx); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(message, "2026-11-04 00:28（北京时间）") || strings.Contains(message, "16:28 UTC") {
		t.Fatalf("unexpected state change time: %s", message)
	}
}

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

func TestStructureStateNotificationUsesBeijingTime(t *testing.T) {
	s, pool, user, _ := qqBindingFixture(t)
	ctx := context.Background()
	var message string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Type    int    `json:"msg_type"`
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Type != 0 {
			t.Errorf("expected plain text, got type %d", payload.Type)
		}
		message = payload.Content
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

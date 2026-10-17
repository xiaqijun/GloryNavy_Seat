package community

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"glorynavy.local/seat/internal/modules/community/internal/store"
	"glorynavy.local/seat/migrations"
)

type approvalBotFixture struct {
	mu                       sync.Mutex
	sends                    int
	offline, unknown, absent bool
	message                  []oneBotSegment
}

func newApprovalFixture(t *testing.T) (*ApprovalJobs, *pgxpool.Pool, string, *approvalBotFixture) {
	t.Helper()
	s, pool, user, code := oneBotFixture(t)
	response := oneBotRequest(s, oneBotBody(t, 55, code, nil), strings.Repeat("s", 32))
	if response.Code != 200 {
		t.Fatal(response.Code)
	}
	bot := &approvalBotFixture{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		bot.mu.Lock()
		defer bot.mu.Unlock()
		if req.Header.Get("Authorization") != "Bearer "+strings.Repeat("t", 32) {
			t.Error("missing token")
		}
		var data any
		switch req.URL.Path {
		case "/get_login_info":
			data = map[string]any{"user_id": 999999}
		case "/get_status":
			data = map[string]any{"online": !bot.offline, "good": true}
		case "/get_group_member_list":
			var input struct {
				NoCache bool `json:"no_cache"`
			}
			_ = json.NewDecoder(req.Body).Decode(&input)
			if input.NoCache {
				t.Error("forced list refresh can invalidate NapCat cache")
			}
			data = []map[string]any{}
			if !bot.absent {
				data = []map[string]any{{"user_id": 123456789}}
			}
		case "/get_group_member_info":
			var input struct {
				NoCache bool   `json:"no_cache"`
				UserID  string `json:"user_id"`
			}
			_ = json.NewDecoder(req.Body).Decode(&input)
			if !input.NoCache || input.UserID != "123456789" {
				t.Error("member lookup is not fresh or targets another user")
			}
			data = map[string]any{"user_id": 123456789, "group_id": 888888}
			if bot.absent {
				data = map[string]any{}
			}
		case "/send_group_msg":
			var input struct {
				GroupID int64           `json:"group_id"`
				Message []oneBotSegment `json:"message"`
			}
			if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
				t.Error(err)
			}
			if input.GroupID != 888888 {
				t.Error("wrong group")
			}
			bot.sends++
			bot.message = input.Message
			data = map[string]any{"message_id": 123}
			if bot.unknown {
				data = map[string]any{}
			}
		default:
			t.Error(req.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "retcode": 0, "data": data})
	}))
	t.Cleanup(server.Close)
	settings, err := s.ListOneBotSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if settings.Groups[0].NotifyApprovals {
		t.Fatal("migration enabled unsolicited reminders")
	}
	settings.Groups[0].NotifyApprovals = true
	_, err = s.SaveOneBotSettings(context.Background(), OneBotSettingsInput{Enabled: true, APIBase: server.URL, BotQQ: settings.BotQQ, Revision: settings.Revision, Groups: settings.Groups})
	if err != nil {
		t.Fatal(err)
	}
	return &ApprovalJobs{Service: s, Enabled: true, Origin: "https://seat.example", Pending: func(context.Context, string) ([]string, error) { return []string{"opaque-key"}, nil }}, pool, user, bot
}
func resetApprovalRate(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `UPDATE community_approval_deliveries SET created_at=now()-interval '2 minutes'`); err != nil {
		t.Fatal(err)
	}
}
func TestApprovalNotificationsDeduplicateConcurrentAndReopened(t *testing.T) {
	r, pool, _, bot := newApprovalFixture(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := r.deliver(ctx, "888888"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if bot.sends != 1 {
		t.Fatal("duplicate sends", bot.sends)
	}
	if len(bot.message) != 3 || bot.message[0].Type != "at" || bot.message[0].Data["qq"] != "123456789" || bot.message[2].Data["text"] != "https://seat.example/approvals" {
		t.Fatal(bot.message)
	}
	resetApprovalRate(t, pool)
	restarted := *r
	if err := restarted.deliver(ctx, "888888"); err != nil || bot.sends != 1 {
		t.Fatal("restart redelivered", err, bot.sends)
	}
	// A config save must not cascade-delete durable deduplication receipts.
	settings, err := r.Service.ListOneBotSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.Service.SaveOneBotSettings(ctx, OneBotSettingsInput{Enabled: true, APIBase: settings.APIBase, BotQQ: settings.BotQQ, Revision: settings.Revision, Groups: settings.Groups})
	if err != nil {
		t.Fatal(err)
	}
	if err = r.deliver(ctx, "888888"); err != nil || bot.sends != 1 {
		t.Fatal(err, bot.sends)
	}
	r.Pending = func(context.Context, string) ([]string, error) { return nil, errors.New("source failed") }
	if err = r.deliver(ctx, "888888"); err == nil {
		t.Fatal("source failure hidden")
	}
	r.Pending = restarted.Pending
	if err = r.deliver(ctx, "888888"); err != nil || bot.sends != 1 {
		t.Fatal("source outage reset receipt", err, bot.sends)
	}
	r.Pending = func(context.Context, string) ([]string, error) { return []string{}, nil }
	if err = r.deliver(ctx, "888888"); err != nil {
		t.Fatal(err)
	}
	r.Pending = restarted.Pending
	if err = r.deliver(ctx, "888888"); err != nil || bot.sends != 2 {
		t.Fatal("new pending episode not notified", err, bot.sends)
	}
}
func TestApprovalNotificationsRecheckBeforeSend(t *testing.T) {
	for _, scenario := range []string{"resolved", "revoked", "profile_changed", "disabled", "removed", "not_member", "unbound", "module_disabled", "left_group"} {
		t.Run(scenario, func(t *testing.T) {
			r, pool, user, bot := newApprovalFixture(t)
			ctx := context.Background()
			calls := 0
			if scenario == "not_member" {
				bot.absent = true
			}
			if scenario == "module_disabled" {
				r.Enabled = false
			}
			if scenario == "unbound" {
				_, err := pool.Exec(ctx, `DELETE FROM community_onebot_bindings WHERE user_id=$1`, user)
				if err != nil {
					t.Fatal(err)
				}
			}
			r.Pending = func(context.Context, string) ([]string, error) {
				calls++
				if scenario == "left_group" && calls == 1 {
					bot.mu.Lock()
					bot.absent = true
					bot.mu.Unlock()
				}
				if calls == 2 {
					switch scenario {
					case "resolved", "revoked":
						return []string{}, nil
					case "profile_changed":
						_, err := r.Service.Update(ctx, user, "234567890", "Binding Pilot", "1")
						if err != nil {
							t.Fatal(err)
						}
					case "disabled", "removed":
						settings, err := r.Service.ListOneBotSettings(ctx)
						if err != nil {
							t.Fatal(err)
						}
						if scenario == "removed" {
							settings.Groups = []OneBotGroup{}
						}
						_, err = r.Service.SaveOneBotSettings(ctx, OneBotSettingsInput{Enabled: scenario != "disabled", APIBase: settings.APIBase, BotQQ: settings.BotQQ, Revision: settings.Revision, Groups: settings.Groups})
						if err != nil {
							t.Fatal(err)
						}
					}
				}
				return []string{"pending"}, nil
			}
			_ = r.deliver(ctx, "888888")
			if bot.sends != 0 {
				t.Fatal("sent stale notification", scenario)
			}
		})
	}
}
func TestApprovalNotificationsOfflineRetryAndAmbiguousAtMostOnce(t *testing.T) {
	r, pool, _, bot := newApprovalFixture(t)
	ctx := context.Background()
	bot.offline = true
	if err := r.deliver(ctx, "888888"); err == nil {
		t.Fatal("offline send")
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM community_approval_deliveries`).Scan(&count); err != nil || count != 0 {
		t.Fatal("offline consumed attempt", err, count)
	}
	bot.offline = false
	bot.unknown = true
	if err := r.deliver(ctx, "888888"); err != nil {
		t.Fatal(err)
	}
	resetApprovalRate(t, pool)
	if err := r.deliver(ctx, "888888"); err != nil || bot.sends != 1 {
		t.Fatal("ambiguous duplicate", err, bot.sends)
	}
	var state string
	if err := pool.QueryRow(ctx, `SELECT state FROM community_approval_deliveries`).Scan(&state); err != nil || state != "unknown" {
		t.Fatal(state, err)
	}
}
func TestApprovalNotificationMergeAndMigrationGuard(t *testing.T) {
	r, pool, user, _ := newApprovalFixture(t)
	ctx := context.Background()
	if err := r.deliver(ctx, "888888"); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result, err := r.Service.MergeAccountTx(ctx, tx, user, "unused", true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(result), `"approval_notices_cancelled":1`) {
		t.Fatal(string(result))
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = store.ExpireApprovalNotices(ctx, pool); err != nil {
		t.Fatal(err)
	}
	db := stdlib.OpenDB(*pool.Config().ConnConfig)
	defer db.Close()
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = provider.DownTo(ctx, 91); err == nil || !strings.Contains(err.Error(), "Approval notifications are in use") {
		t.Fatal("unsafe rollback", err)
	}
}

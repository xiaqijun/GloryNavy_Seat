package community

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/eve"
)

func TestFleetGroupRequiresMatchingCorporationAndUnchangedTarget(t *testing.T) {
	s, pool, user, _ := qqBindingFixture(t)
	s.LockAccounts = func(context.Context, pgx.Tx, []string) error { return nil }
	ctx := context.Background()
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/v2/groups/official-group/messages" {
			t.Errorf("path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"id":"message"}`))
	}))
	defer server.Close()
	bot := newOfficialQQBot("app-a", "secret", server.URL)
	bot.HTTP = server.Client()
	bot.token = qqBotToken{AccessToken: "token", ExpiresAt: time.Now().Add(time.Hour)}
	useOfficialQQTestBot(t, s, bot)
	if _, err := pool.Exec(ctx, `UPDATE community_qq_structure_watch SET enabled=true,app_id='app-a',group_openid='official-group',corporation_id=42,owner_id=$1 WHERE id`, user); err != nil {
		t.Fatal(err)
	}
	r := &StructureJobs{Service: s, Enabled: true, Administrator: func(context.Context, string) (bool, error) { return true, nil }, Read: func(context.Context, string, int64) ([]eve.Structure, error) { return []eve.Structure{}, nil }}
	if _, err := r.FleetGroupTarget(ctx, 43); err == nil {
		t.Fatal("wrong corporation")
	}
	target, err := r.FleetGroupTarget(ctx, 42)
	if err != nil {
		t.Fatal(err)
	}
	got := r.FleetGroup(ctx, 42, target, "fleet notice")
	if got.State != "sent" || calls != 1 {
		t.Fatal(got, calls)
	}
	if _, err = pool.Exec(ctx, `UPDATE community_qq_structure_watch SET revision=revision+1 WHERE id`); err != nil {
		t.Fatal(err)
	}
	got = r.FleetGroup(ctx, 42, target, "fleet notice")
	if got.Reason != "group_changed" || calls != 1 {
		t.Fatal(got, calls)
	}
}

package community

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"glorynavy.local/seat/internal/modules/eve"
)

func TestStructureSettingsProtectRevisionAndOwnership(t *testing.T) {
	s, pool, user, _ := qqBindingFixture(t)
	ctx := context.Background()
	bot := newOfficialQQBot("app-a", "secret", "")
	useOfficialQQTestBot(t, s, bot)
	admin := true
	readable := true
	job := &StructureJobs{Service: s, Enabled: true, Administrator: func(context.Context, string) (bool, error) { return admin, nil }, Read: func(_ context.Context, _ string, corp int64) ([]eve.Structure, error) {
		if !readable || corp != 42 {
			return nil, errors.New("denied")
		}
		return []eve.Structure{}, nil
	}}
	initial, err := job.Settings(ctx, user)
	if err != nil || initial.Revision != "1" || initial.Enabled {
		t.Fatal(initial, err)
	}
	in := StructureWatchInput{Enabled: true, GroupOpenID: "official-group", CorporationID: "42", Revision: initial.Revision}
	invalid := in
	invalid.GroupOpenID = "123456"
	if _, err = job.SaveSettings(ctx, user, invalid); !errors.Is(err, ErrStructureWatchInvalid) {
		t.Fatal("numeric group must be rejected", err)
	}
	saved, err := job.SaveSettings(ctx, user, in)
	if err != nil || !saved.Enabled || saved.Revision != "2" {
		t.Fatal(saved, err)
	}
	if _, err = job.SaveSettings(ctx, user, in); !errors.Is(err, ErrVersion) {
		t.Fatal("stale edit", err)
	}
	in.Revision = saved.Revision
	unchanged, err := job.SaveSettings(ctx, user, in)
	if err != nil || unchanged.Revision != saved.Revision {
		t.Fatal("no-op changed revision", unchanged, err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO community_qq_structure_notices(watch_revision,reference,kind,corporation_id,structure_id,body,status) VALUES(2,'test','fuel',42,12,'detail','unknown')`); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO community_qq_structure_notices(watch_revision,reference,kind,corporation_id,structure_id,body,status) VALUES(1,'other','fuel',43,13,'other','sent')`); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO community_qq_structure_notices(watch_revision,reference,kind,corporation_id,structure_id,body,status,attempted_at) VALUES(2,'interrupted','fuel',42,14,'interrupted','sending',now()-interval '3 minutes')`); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO community_qq_structure_notices(watch_revision,reference,kind,corporation_id,structure_id,body,status) VALUES(1,'old-pending','fuel',42,15,'old','pending')`); err != nil {
		t.Fatal(err)
	}
	records, err := job.Deliveries(ctx, user)
	if err != nil || len(records) != 3 || records[0].Status != "obsolete" || records[1].Status != "unknown" || records[2].Status != "unknown" {
		t.Fatal(records, err)
	}
	var stored string
	if err = pool.QueryRow(ctx, `SELECT status FROM community_qq_structure_notices WHERE reference='interrupted'`).Scan(&stored); err != nil || stored != "sending" {
		t.Fatal("uncertain result must remain unchanged", stored, err)
	}
	readable = false
	if _, err = job.Deliveries(ctx, user); !errors.Is(err, ErrStructureWatchSource) {
		t.Fatal("records exposed without access", err)
	}
	outOfScope := in
	outOfScope.Enabled = false
	outOfScope.CorporationID = "43"
	if _, err = job.SaveSettings(ctx, user, outOfScope); !errors.Is(err, ErrStructureWatchSource) {
		t.Fatal("out-of-scope disabled target", err)
	}
	in.Enabled = false
	disabled, err := job.SaveSettings(ctx, user, in)
	if err != nil || disabled.Enabled || disabled.Revision != "3" {
		t.Fatal("must allow disabling if source unavailable", disabled, err)
	}
	readable = true
	if _, err = pool.Exec(ctx, `UPDATE community_qq_bot_settings SET app_id='app-b' WHERE id`); err != nil {
		t.Fatal(err)
	}
	in.Enabled, in.Revision = true, disabled.Revision
	if _, err = job.SaveSettings(ctx, user, in); !errors.Is(err, ErrStructureWatchBot) {
		t.Fatal("old OpenID cannot follow a new app", err)
	}
	in.GroupOpenID = "new-app-group"
	if _, err = job.SaveSettings(ctx, user, in); err != nil {
		t.Fatal("new app group", err)
	}
	admin = false
	if _, err = job.Settings(ctx, user); !errors.Is(err, ErrStructureWatchForbidden) {
		t.Fatal("non-admin read", err)
	}
	if _, err = job.SaveSettings(ctx, user, in); !errors.Is(err, ErrStructureWatchForbidden) {
		t.Fatal("non-admin write", err)
	}
}

func TestStructureSettingsConcurrentRevision(t *testing.T) {
	s, _, user, _ := qqBindingFixture(t)
	bot := newOfficialQQBot("app-a", "secret", "")
	useOfficialQQTestBot(t, s, bot)
	job := &StructureJobs{Service: s, Enabled: true, Administrator: func(context.Context, string) (bool, error) { return true, nil }, Read: func(context.Context, string, int64) ([]eve.Structure, error) { return []eve.Structure{}, nil }}
	results := make(chan error, 2)
	var start sync.WaitGroup
	start.Add(1)
	for _, group := range []string{"first-group", "second-group"} {
		go func(group string) {
			start.Wait()
			_, err := job.SaveSettings(context.Background(), user, StructureWatchInput{Enabled: true, GroupOpenID: group, CorporationID: "42", Revision: "1"})
			results <- err
		}(group)
	}
	start.Done()
	first, second := <-results, <-results
	if !((first == nil && errors.Is(second, ErrVersion)) || (second == nil && errors.Is(first, ErrVersion))) {
		t.Fatalf("expected one save and one conflict: %v, %v", first, second)
	}
}

func TestStructureSettingsRaceWithAutomaticDisable(t *testing.T) {
	s, pool, user, _ := qqBindingFixture(t)
	ctx := context.Background()
	started, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"code":40034105}`))
	}))
	defer server.Close()
	bot := newOfficialQQBot("app-a", "secret", server.URL)
	bot.HTTP = server.Client()
	bot.token = qqBotToken{AccessToken: "test-token", ExpiresAt: time.Now().Add(time.Hour)}
	useOfficialQQTestBot(t, s, bot)
	if _, err := pool.Exec(ctx, `UPDATE community_qq_structure_watch SET enabled=true,app_id='app-a',group_openid='official-group',corporation_id=42,owner_id=$1 WHERE id`, user); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	row := eve.Structure{ID: 10, CorporationID: 42, Name: "Test", State: "online", ObservedAt: now}
	job := &StructureJobs{Service: s, Enabled: true, Administrator: func(context.Context, string) (bool, error) { return true, nil }, Read: func(context.Context, string, int64) ([]eve.Structure, error) { return []eve.Structure{row}, nil }}
	if err := job.scan(ctx); err != nil {
		t.Fatal(err)
	}
	row.State = "armor_reinforce"
	row.ObservedAt = now.Add(time.Minute)
	scanResult := make(chan error, 1)
	go func() { scanResult <- job.scan(ctx) }()
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("send not started")
	}
	saveResult := make(chan error, 1)
	go func() {
		_, err := job.SaveSettings(ctx, user, StructureWatchInput{Enabled: false, GroupOpenID: "official-group", CorporationID: "42", Revision: "1"})
		saveResult <- err
	}()
	close(release)
	select {
	case err := <-scanResult:
		if err != nil {
			t.Fatal("scan", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("scan deadlocked")
	}
	select {
	case err := <-saveResult:
		if !errors.Is(err, ErrVersion) {
			t.Fatal("stale save", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("save deadlocked")
	}
}

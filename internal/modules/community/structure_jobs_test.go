package community

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"glorynavy.local/seat/internal/modules/eve"
)

func TestStructureNoticeDetailsAndTimes(t *testing.T) {
	expires := time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)
	unanchor := expires.Add(time.Hour)
	row := eve.Structure{ID: 123, Name: "5-O8B1 - Glory Navy", TypeName: "堡垒", SolarSystemName: "5-O8B1", FuelExpires: &expires, UnanchorsAt: &unanchor}
	message := structureNoticeDetails(row) + structureNoticeTimes(row)
	for _, want := range []string{"建筑：5-O8B1 - Glory Navy", "建筑类型：堡垒", "星系：5-O8B1", "燃料到期：2026-11-03 17:00（北京时间）", "计划拆除：2026-11-03 18:00（北京时间）"} {
		if !strings.Contains(message, want) {
			t.Fatalf("missing %q in %q", want, message)
		}
	}
	row.Name, row.Kind, row.UnanchorsAt = "", "pos", nil
	message = structureNoticeDetails(row) + structureNoticeTimes(row)
	if !strings.Contains(message, "建筑：编号 123\n建筑类型：堡垒") || !strings.Contains(message, "燃料预计耗尽：") || strings.Contains(message, "计划拆除") {
		t.Fatalf("unexpected POS message: %s", message)
	}
	row.Name, row.TypeName, row.SolarSystemName, row.FuelExpires = "unknown", "", "", nil
	if got := structureNoticeDetails(row) + structureNoticeTimes(row); got != "建筑：unknown\n建筑类型：未知" {
		t.Fatalf("unexpected missing-field message: %s", got)
	}
}

func TestStructureNotificationContainsIdentityAndExpiry(t *testing.T) {
	s, pool, user, _ := qqBindingFixture(t)
	ctx := context.Background()
	var content string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Type    int    `json:"msg_type"`
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Type != 0 {
			t.Errorf("expected plain text, got message type %d", body.Type)
		}
		content = body.Content
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
	now := time.Now().UTC()
	expires := now.Add(12 * time.Hour).Truncate(time.Minute)
	row := eve.Structure{ID: 100, CorporationID: 42, Name: "Test structure", TypeName: "堡垒", SolarSystemName: "5-O8B1", Kind: "upwell", State: "online", ObservedAt: now, FuelExpires: &expires}
	job := &StructureJobs{Service: s, Enabled: true, Administrator: func(context.Context, string) (bool, error) { return true, nil }, Read: func(context.Context, string, int64) ([]eve.Structure, error) { return []eve.Structure{row}, nil }}
	if err := job.scan(ctx); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"建筑：Test structure", "建筑类型：堡垒", "星系：5-O8B1", "⛽ 燃料：24 小时内", expires.In(time.FixedZone("UTC+8", 8*60*60)).Format("2006-01-02 15:04") + "（北京时间）"} {
		if !strings.Contains(content, want) {
			t.Fatalf("missing %q in sent message %q", want, content)
		}
	}
}

func TestStructureJobsExtensionHasLifecycleBindings(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		extension := (&StructureJobs{Enabled: enabled}).Extension()
		if extension.Register == nil || extension.Bind == nil {
			t.Fatalf("enabled=%v: lifecycle callbacks missing", enabled)
		}
	}
}

func TestStructureNoticeTypeHasOwnLabel(t *testing.T) {
	item := eve.Structure{ID: 123, Name: "5-O8B1 - Glory Navy", TypeID: 35835, TypeName: "阿塔诺", SolarSystemName: "5-O8B1"}
	got := structureNoticeDetails(item)
	if !strings.Contains(got, "建筑类型：阿塔诺") {
		t.Fatalf("missing explicit building type: %s", got)
	}
	item.TypeName = ""
	got = structureNoticeDetails(item)
	if !strings.Contains(got, "建筑类型：未知（ID 35835）") {
		t.Fatalf("missing type fallback: %s", got)
	}
}

func TestStructureNotificationsPersistBaselineAndDoNotResend(t *testing.T) {
	s, pool, user, _ := qqBindingFixture(t)
	ctx := context.Background()
	var sends atomic.Int64
	var reject atomic.Bool
	var ambiguous atomic.Bool
	messages := make(chan string, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sends.Add(1)
		var payload struct {
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		messages <- payload.Content
		if reject.Load() {
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"code":40034105}`))
			return
		}
		if ambiguous.Load() && strings.Contains(payload.Content, "建筑：Beta") {
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{}`))
			return
		}
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
	now := time.Now().UTC()
	rows := []eve.Structure{
		{ID: 100, CorporationID: 42, Name: "Alpha", State: "online", ObservedAt: now},
		{ID: 101, CorporationID: 42, Name: "Beta", State: "online", ObservedAt: now, Services: []eve.StructureService{{Name: "market", State: "online"}}},
	}
	admin := true
	job := &StructureJobs{Service: s, Enabled: true, Administrator: func(context.Context, string) (bool, error) { return admin, nil }, Read: func(context.Context, string, int64) ([]eve.Structure, error) { return rows, nil }}
	if err := job.scan(ctx); err != nil {
		t.Fatal("baseline", err)
	}
	if sends.Load() != 0 {
		t.Fatal("baseline must not send")
	}
	rows[0].State = "armor_reinforce"
	rows[0].ObservedAt = now.Add(time.Minute)
	rows[1].Services[0].State = "offline"
	rows[1].ObservedAt = now.Add(time.Minute)
	ambiguous.Store(true)
	if err := job.scan(ctx); err != nil {
		t.Fatal("change", err)
	}
	if sends.Load() != 2 {
		t.Fatal("expected two separate messages", sends.Load())
	}
	first, second := <-messages, <-messages
	if !strings.Contains(first, "建筑：Alpha") || !strings.Contains(first, "🛡️ 建筑状态：在线 → 装甲增强") || strings.Contains(first, "建筑：Beta") {
		t.Fatalf("unexpected first message: %q", first)
	}
	if !strings.Contains(second, "建筑：Beta") || !strings.Contains(second, "🔴 服务 market：在线 → 离线") || strings.Contains(second, "建筑：Alpha") {
		t.Fatalf("unexpected second message: %q", second)
	}
	if err := job.scan(ctx); err != nil {
		t.Fatal("repeat", err)
	}
	if sends.Load() != 2 {
		t.Fatal("duplicate message", sends.Load())
	}
	var delivered, unknown int
	if err := pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE structure_id=100 AND status='sent'), count(*) FILTER (WHERE structure_id=101 AND status='unknown') FROM community_qq_structure_notices`).Scan(&delivered, &unknown); err != nil || delivered != 1 || unknown != 1 {
		t.Fatal("independent receipts", delivered, unknown, err)
	}
	admin = false
	rows[0].State = "hull_reinforce"
	rows[0].ObservedAt = now.Add(2 * time.Minute)
	rows[1].State = "offline"
	rows[1].ObservedAt = now.Add(2 * time.Minute)
	if err := job.scan(ctx); err != nil {
		t.Fatal(err)
	}
	if sends.Load() != 2 {
		t.Fatal("sent after administrator access revoked")
	}
	admin = true
	reject.Store(true)
	if err := job.scan(ctx); err != nil {
		t.Fatal("rejection", err)
	}
	var enabled bool
	var revision int64
	if err := pool.QueryRow(ctx, `SELECT enabled,revision FROM community_qq_structure_watch WHERE id`).Scan(&enabled, &revision); err != nil || enabled || revision != 2 {
		t.Fatal("permission rejection must disable watch and advance revision", enabled, revision, err)
	}
	if err := job.scan(ctx); err != nil {
		t.Fatal("disabled", err)
	}
	if sends.Load() != 3 {
		t.Fatalf("permission refusal must stop later sends: %d", sends.Load())
	}
	var pending int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM community_qq_structure_notices WHERE structure_id=101 AND status='pending'`).Scan(&pending); err != nil || pending != 1 {
		t.Fatal("later notice was claimed without sending", pending, err)
	}
}

func TestStructureActiveReferencesReflectCurrentFuelAndCorporation(t *testing.T) {
	now := time.Now().UTC()
	expires := now.Add(20 * time.Hour)
	unanchor := now.Add(2 * time.Hour)
	rows := []eve.Structure{{ID: 1, CorporationID: 42, ObservedAt: now, FuelExpires: &expires, UnanchorsAt: &unanchor}, {ID: 2, CorporationID: 43, ObservedAt: now, FuelExpires: &expires}}
	ids, refs := structureActiveReferences(rows, 42, now)
	if !ids[1] || ids[2] || len(refs) != 2 {
		t.Fatalf("unexpected current objects: %v %v", ids, refs)
	}
	ref := structureReference("fuel", "1", expires.UTC().Format("2006-01-02"), "24 小时内")
	if !refs[ref] {
		t.Fatal("current fuel stage missing")
	}
	later := now.Add(10 * 24 * time.Hour)
	rows[0].FuelExpires = &later
	_, refs = structureActiveReferences(rows, 42, now)
	if refs[ref] {
		t.Fatal("refilled fuel must invalidate earlier notice")
	}
}

func TestStructureNotificationsDiscardStaleAndDoNotRetryUnknown(t *testing.T) {
	s, pool, user, _ := qqBindingFixture(t)
	ctx := context.Background()
	var sends atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sends.Add(1)
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	bot := newOfficialQQBot("test-app", "test-secret", server.URL)
	bot.HTTP = server.Client()
	bot.token = qqBotToken{AccessToken: "test-token", ExpiresAt: time.Now().Add(time.Hour)}
	useOfficialQQTestBot(t, s, bot)
	if _, err := pool.Exec(ctx, `UPDATE community_qq_structure_watch SET enabled=true,app_id='test-app',group_openid='group-openid',corporation_id=42,owner_id=$1 WHERE id`, user); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	soon, later := now.Add(time.Hour), now.Add(10*24*time.Hour)
	row := eve.Structure{ID: 100, CorporationID: 42, Name: "Test", State: "online", ObservedAt: now, FuelExpires: &soon}
	reads := 0
	refilled := true
	job := &StructureJobs{Service: s, Enabled: true, Administrator: func(context.Context, string) (bool, error) { return true, nil }, Read: func(context.Context, string, int64) ([]eve.Structure, error) {
		reads++
		copy := row
		if refilled && reads == 2 {
			copy.FuelExpires = &later
		}
		return []eve.Structure{copy}, nil
	}}
	if err := job.scan(ctx); err != nil {
		t.Fatal(err)
	}
	var obsolete int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM community_qq_structure_notices WHERE status='obsolete'`).Scan(&obsolete); err != nil || obsolete != 1 || sends.Load() != 0 {
		t.Fatal("stale warning", obsolete, sends.Load(), err)
	}
	refilled = false
	row.State = "armor_reinforce"
	row.ObservedAt = now.Add(time.Minute)
	row.FuelExpires = nil
	if err := job.scan(ctx); err != nil {
		t.Fatal(err)
	}
	if err := job.scan(ctx); err != nil {
		t.Fatal(err)
	}
	var unknown int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM community_qq_structure_notices WHERE status='unknown'`).Scan(&unknown); err != nil || unknown != 1 || sends.Load() != 1 {
		t.Fatal("unknown result", unknown, sends.Load(), err)
	}
}

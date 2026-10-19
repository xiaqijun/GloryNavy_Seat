package welfare

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
	"glorynavy.local/seat/internal/platform/locale"
)

// Synthetic public response, deliberately includes repeated pilots and an NPC.
const battleFixture = `{"status":"done","result":{"report":{
 "start_time":"2026-10-19T04:00:00+08:00","end_time":"2026-10-19T04:40:00+08:00","kill_count":2,"total_value":5000000000,
 "systems":[{"system_name":"15W-GC"}],
 "teams":[
 {"team_id":"A","team_name":"Alliance A","kills_count":1,"losses_count":1,"kills_value":2000000000,"losses_value":3000000000,"members":[{"org_id":100}]},
 {"team_id":"B","team_name":"Corporation B","kills_count":1,"losses_count":1,"kills_value":3000000000,"losses_value":2000000000,"members":[{"org_id":200}]}],
 "killmails":[
 {"killmail_id":987,"victim":{"character_id":1,"corporation_id":10,"alliance_id":100},"attackers":[{"character_id":2,"corporation_id":200},{"character_id":3,"corporation_id":200},{"character_id":0,"corporation_id":200}]},
 {"killmail_id":988,"victim":{"character_id":2,"corporation_id":200},"attackers":[{"character_id":1,"corporation_id":10,"alliance_id":100},{"character_id":4,"corporation_id":11,"alliance_id":100}]}]
 }}}`

func battleServer(t *testing.T, fn http.HandlerFunc) *BattleReportClient {
	t.Helper()
	server := httptest.NewServer(fn)
	t.Cleanup(server.Close)
	c := NewBattleReportClient()
	c.origin = server.URL
	return c
}

func TestBattleReportProjection(t *testing.T) {
	var job battleJob
	if err := json.Unmarshal([]byte(battleFixture), &job); err != nil {
		t.Fatal(err)
	}
	s, err := summarizeBattle(987, job.Result.Report, time.Now())
	if err != nil || s.KillCount != 2 || s.TotalISK != 5e9 || len(s.Teams) != 2 {
		t.Fatalf("summary: %+v %v", s, err)
	}
	if s.Teams[0].Pilots != 2 || s.Teams[1].Pilots != 2 || !s.Teams[0].Own || s.Teams[1].Own || s.Teams[1].LossesISK != 2e9 {
		t.Fatalf("dedup/team mapping: %+v", s.Teams)
	}
	for _, mutate := range []func(*battleSource){
		func(r *battleSource) { r.Killmails[0].ID = 999 },
		func(r *battleSource) { r.Killmails[1].ID = 987 },
		func(r *battleSource) { r.Teams[0].LossesISK = -1 },
		func(r *battleSource) { r.Teams[1].Members[0].ID = 100 },
		func(r *battleSource) { r.End = r.Start.Add(-time.Hour) },
		func(r *battleSource) { r.KillCount = 3 },
	} {
		json.Unmarshal([]byte(battleFixture), &job)
		mutate(job.Result.Report)
		if _, err := summarizeBattle(987, job.Result.Report, time.Now()); err == nil {
			t.Fatal("invalid source accepted")
		}
	}
}

func TestBattleReportCacheAndPolling(t *testing.T) {
	var calls atomic.Int32
	const jobID = "00000000-0000-4000-8000-000000000001"
	c := battleServer(t, func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("private credentials forwarded")
		}
		if n == 1 {
			var request struct {
				ID   int64  `json:"killmail_id"`
				Lang string `json:"lang"`
			}
			json.NewDecoder(r.Body).Decode(&request)
			if r.Method != "POST" || r.URL.Path != "/api/public/battle_reports/expand" || request.ID != 987 || request.Lang != "zh-CN" {
				t.Errorf("invalid request: %+v", request)
			}
			w.WriteHeader(202)
			fmt.Fprintf(w, `{"status":"running","job_id":%q}`, jobID)
			return
		}
		if n == 2 && (r.Method != "GET" || r.URL.Path != "/api/public/battle_reports/jobs/"+jobID) {
			t.Error("wrong poll")
		}
		fmt.Fprint(w, battleFixture)
	})
	ctx := context.Background()
	for range 3 {
		if v := c.Get(ctx, 987); v.Status != "pending" {
			t.Fatalf("pending: %+v", v)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("pending poll not throttled")
	}
	e := c.cached("987:zh-CN")
	e.next = time.Time{}
	c.remember("987:zh-CN", e)
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if v := c.Get(ctx, 987); v.Status != "ready" {
				t.Errorf("ready: %+v", v)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 2 {
		t.Fatal("duplicate concurrent requests")
	}
	if c.Get(locale.With(ctx, "en"), 987).Status != "ready" || calls.Load() != 3 {
		t.Fatal("locale cache mixed")
	}
	e = c.cached("987:zh-CN")
	e.value = BattleReport{Status: "pending"}
	e.started = time.Now().Add(-4 * time.Minute)
	e.next = time.Time{}
	c.remember("987:zh-CN", e)
	if c.Get(ctx, 987).Status != "unavailable" || calls.Load() != 3 {
		t.Fatal("unbounded pending job")
	}
	for i := range 300 {
		c.remember(fmt.Sprint(i), battleCache{started: time.Now()})
	}
	if len(c.cache) != 256 {
		t.Fatal("unbounded cache")
	}
}

func TestBattleReportFailuresAreBounded(t *testing.T) {
	for _, body := range []string{`<html>unavailable</html>`, `{"status":"failed"}`, `{"status":"running","job_id":"../../private"}`, strings.Repeat("x", (16<<20)+1)} {
		t.Run(fmt.Sprint(len(body)), func(t *testing.T) {
			var calls atomic.Int32
			c := battleServer(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); fmt.Fprint(w, body) })
			for range 2 {
				if v := c.Get(context.Background(), 987); v.Status != "unavailable" || v.Summary != nil {
					t.Fatal("bad source shown")
				}
			}
			if calls.Load() != 1 {
				t.Fatal("failure not cached")
			}
		})
	}
	c := battleServer(t, func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/private", 302) })
	if c.Get(context.Background(), 987).Status != "unavailable" {
		t.Fatal("redirect followed")
	}
	var nilClient *BattleReportClient
	if nilClient.Get(context.Background(), 987).Status != "unavailable" {
		t.Fatal("disabled client")
	}
}

func TestBattleReportLossAuthorizationBeforeCache(t *testing.T) {
	var calls atomic.Int32
	c := battleServer(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); fmt.Fprint(w, battleFixture) })
	allowed := true
	s := &Service{BattleReports: c, Losses: func(_ context.Context, actor string, corp, char, before, id int64) ([]Loss, error) {
		if !allowed || actor != userID || corp != 10 || char != 123 || before != 0 || id != 987 {
			return nil, pgx.ErrNoRows
		}
		return []Loss{{ID: 987, CharacterID: 123, CorporationID: 10}}, nil
	}}
	v, err := s.lossBattleReport(context.Background(), userID, 10, 123, 987)
	if err != nil || v.Status != "ready" {
		t.Fatalf("authorized: %+v %v", v, err)
	}
	allowed = false
	if _, err = s.lossBattleReport(context.Background(), userID, 10, 123, 987); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("revoked scope reads cached data")
	}
	s.Losses = func(context.Context, string, int64, int64, int64, int64) ([]Loss, error) {
		return []Loss{{ID: 987, CharacterID: 456, CorporationID: 10}}, nil
	}
	if _, err = s.lossBattleReport(context.Background(), userID, 10, 123, 987); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("wrong victim accepted")
	}
	if calls.Load() != 1 {
		t.Fatal("unauthorized external access")
	}
}

func TestBattleReportCaseAuthorizationAndNoWrites(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	var calls atomic.Int32
	s.BattleReports = battleServer(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); fmt.Fprint(w, battleFixture) })
	saved, err := store.Save(ctx, s.Pool, Case{AccountID: userID, CorporationID: 10, Kind: "solo", State: "submitted", Detail: raw(Detail{KillmailID: 987}), Keys: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	for _, actor := range []string{userID, adminID} {
		v, err := s.caseBattleReport(ctx, actor, saved.ID)
		if err != nil || v.Status != "ready" {
			t.Fatalf("%s: %+v %v", actor, v, err)
		}
	}
	if _, err = s.caseBattleReport(ctx, otherID, saved.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("unrelated account read")
	}
	s.Scope = func(context.Context, string, int64, bool) (bool, error) { return false, nil }
	if _, err = s.caseBattleReport(ctx, adminID, saved.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("revoked manager read")
	}
	after, err := store.Read(ctx, s.Pool, saved.ID)
	if err != nil || string(raw(after)) != string(raw(saved)) || calls.Load() != 1 {
		t.Fatal("read changed claim or repeated upstream request")
	}
	saved.Kind = "srp"
	saved.ID = 0
	saved, err = store.Save(ctx, s.Pool, saved)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.caseBattleReport(ctx, userID, saved.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("non-PVP report exposed")
	}
}

// Explicit opt-in only; reads the public example and never accesses a business DB.
func TestBattleReportLivePublicExample(t *testing.T) {
	if os.Getenv("TEST_BATTLE_REPORT_LIVE") != "1" {
		t.Skip("public integration opt-in")
	}
	c := NewBattleReportClient()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	for {
		v := c.Get(ctx, 139267042)
		if v.Status == "ready" {
			t.Logf("public example: KM=%d teams=%+v", v.Summary.KillCount, v.Summary.Teams)
			return
		}
		if v.Status != "pending" {
			t.Fatalf("public report: %+v", v)
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(3 * time.Second):
		}
	}
}

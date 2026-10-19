package welfare

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/platform/locale"
	"golang.org/x/sync/singleflight"
)

const battleReportOrigin = "https://scit.scers.cn:45161"

// Public estimates only: never used by valuation, approval or fulfillment.
type BattleReport struct {
	Status  string         `json:"status"`
	Summary *BattleSummary `json:"summary,omitempty"`
}
type BattleSummary struct {
	Start      time.Time    `json:"start"`
	End        time.Time    `json:"end"`
	ObservedAt time.Time    `json:"observed_at"`
	Systems    []string     `json:"systems"`
	KillCount  int          `json:"kill_count"`
	TotalISK   float64      `json:"total_isk"`
	Teams      []BattleTeam `json:"teams"`
}
type BattleTeam struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Pilots    int     `json:"pilots"`
	Kills     int     `json:"kills"`
	Losses    int     `json:"losses"`
	KillsISK  float64 `json:"kills_isk"`
	LossesISK float64 `json:"losses_isk"`
	Own       bool    `json:"own"`
}
type battleParty struct {
	Character   int64 `json:"character_id"`
	Corporation int64 `json:"corporation_id"`
	Alliance    int64 `json:"alliance_id"`
}
type battleSource struct {
	Start     time.Time `json:"start_time"`
	End       time.Time `json:"end_time"`
	KillCount int       `json:"kill_count"`
	TotalISK  float64   `json:"total_value"`
	Systems   []struct {
		Name string `json:"system_name"`
	} `json:"systems"`
	Teams []struct {
		ID        string  `json:"team_id"`
		Name      string  `json:"team_name"`
		Kills     int     `json:"kills_count"`
		Losses    int     `json:"losses_count"`
		KillsISK  float64 `json:"kills_value"`
		LossesISK float64 `json:"losses_value"`
		Members   []struct {
			ID int64 `json:"org_id"`
		} `json:"members"`
	} `json:"teams"`
	Killmails []struct {
		ID        int64         `json:"killmail_id"`
		Victim    battleParty   `json:"victim"`
		Attackers []battleParty `json:"attackers"`
	} `json:"killmails"`
}
type battleJob struct {
	ID     string `json:"job_id"`
	Status string `json:"status"`
	Result *struct {
		Report *battleSource `json:"report"`
	} `json:"result"`
}
type battleCache struct {
	value   BattleReport
	job     string
	started time.Time
	next    time.Time
}
type BattleReportClient struct {
	client *http.Client
	origin string
	mu     sync.Mutex
	cache  map[string]battleCache
	flight singleflight.Group
	slots  chan struct{}
}

func NewBattleReportClient() *BattleReportClient {
	return &BattleReportClient{client: &http.Client{Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("battle report redirect refused") }}, origin: battleReportOrigin, cache: map[string]battleCache{}, slots: make(chan struct{}, 4)}
}

func (c *BattleReportClient) cached(key string) battleCache {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cache[key]
}
func (c *BattleReportClient) remember(key string, entry battleCache) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.cache[key]; !exists && len(c.cache) >= 256 {
		oldest := ""
		var at time.Time
		for k, v := range c.cache {
			if oldest == "" || v.started.Before(at) {
				oldest = k
				at = v.started
			}
		}
		delete(c.cache, oldest)
	}
	c.cache[key] = entry
}

// Cache only public projections. Authorization is always checked before Get,
// including when the same report was previously requested by another actor.
func (c *BattleReportClient) Get(ctx context.Context, id int64) BattleReport {
	failed := BattleReport{Status: "unavailable"}
	if c == nil || id <= 0 {
		return failed
	}
	key := strconv.FormatInt(id, 10) + ":" + locale.Language(ctx)
	if entry := c.cached(key); time.Now().Before(entry.next) {
		return entry.value
	}
	result := c.flight.DoChan(key, func() (any, error) {
		entry := c.cached(key)
		now := time.Now()
		if now.Before(entry.next) {
			return entry.value, nil
		}
		select {
		case c.slots <- struct{}{}:
			defer func() { <-c.slots }()
		default:
			return failed, nil
		}
		if entry.started.IsZero() || entry.value.Status != "pending" {
			entry = battleCache{started: now}
		}
		entry.value = failed
		entry.next = now.Add(time.Minute)
		if now.Sub(entry.started) > 3*time.Minute {
			c.remember(key, entry)
			return failed, nil
		}
		job, err := c.fetch(ctx, id, entry.job)
		if err == nil {
			switch job.Status {
			case "done":
				if job.Result != nil && job.Result.Report != nil {
					if summary, err := summarizeBattle(id, job.Result.Report, now); err == nil {
						entry.value = BattleReport{Status: "ready", Summary: summary}
						entry.next = now.Add(15 * time.Minute)
					}
				}
			case "pending", "running", "queued", "processing":
				if battleJobID.MatchString(job.ID) {
					entry.job = job.ID
					entry.value = BattleReport{Status: "pending"}
					entry.next = now.Add(3 * time.Second)
				}
			}
		}
		c.remember(key, entry)
		return entry.value, nil
	})
	select {
	case <-ctx.Done():
		return failed
	case r := <-result:
		if r.Err != nil {
			return failed
		}
		return r.Val.(BattleReport)
	}
}

var battleJobID = regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)

func (c *BattleReportClient) fetch(ctx context.Context, id int64, jobID string) (battleJob, error) {
	method, path := http.MethodGet, "/api/public/battle_reports/jobs/"+jobID
	var body io.Reader
	if jobID == "" {
		method = http.MethodPost
		path = "/api/public/battle_reports/expand"
		body = bytes.NewReader(raw(map[string]any{"killmail_id": id, "lang": locale.Language(ctx)}))
	} else if !battleJobID.MatchString(jobID) {
		return battleJob{}, ErrInvalid
	}
	req, err := http.NewRequestWithContext(ctx, method, c.origin+path, body)
	if err != nil {
		return battleJob{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "GloryNavy-SeAT/0.1 (+https://seat.kisectool.com)")
	res, err := c.client.Do(req)
	if err != nil {
		return battleJob{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusAccepted {
		return battleJob{}, fmt.Errorf("battle report status %d", res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, (16<<20)+1))
	if err != nil || len(data) > 16<<20 {
		return battleJob{}, ErrInvalid
	}
	var job battleJob
	err = json.Unmarshal(data, &job)
	return job, err
}
func summarizeBattle(seed int64, report *battleSource, now time.Time) (*BattleSummary, error) {
	validMoney := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v <= 1e18 }
	if report.Start.IsZero() || report.End.Before(report.Start) || len(report.Teams) == 0 || len(report.Teams) > 32 || len(report.Killmails) == 0 || len(report.Killmails) > 20000 || report.KillCount != len(report.Killmails) || !validMoney(report.TotalISK) {
		return nil, ErrInvalid
	}
	s := &BattleSummary{Start: report.Start, End: report.End, ObservedAt: now, KillCount: report.KillCount, TotalISK: report.TotalISK, Systems: []string{}, Teams: []BattleTeam{}}
	for _, system := range report.Systems {
		if system.Name == "" || len(system.Name) > 160 || len(s.Systems) >= 128 {
			return nil, ErrInvalid
		}
		s.Systems = append(s.Systems, system.Name)
	}
	if len(s.Systems) == 0 {
		return nil, ErrInvalid
	}
	orgs := map[int64]int{}
	ids := map[string]bool{}
	pilots := []map[int64]bool{}
	for i, team := range report.Teams {
		if team.ID == "" || len(team.ID) > 80 || ids[team.ID] || team.Name == "" || len(team.Name) > 600 || team.Kills < 0 || team.Kills > report.KillCount || team.Losses < 0 || team.Losses > report.KillCount || !validMoney(team.KillsISK) || !validMoney(team.LossesISK) {
			return nil, ErrInvalid
		}
		ids[team.ID] = true
		s.Teams = append(s.Teams, BattleTeam{ID: team.ID, Name: team.Name, Kills: team.Kills, Losses: team.Losses, KillsISK: team.KillsISK, LossesISK: team.LossesISK})
		pilots = append(pilots, map[int64]bool{})
		for _, member := range team.Members {
			if member.ID <= 0 {
				return nil, ErrInvalid
			}
			if _, exists := orgs[member.ID]; exists {
				return nil, ErrInvalid
			}
			orgs[member.ID] = i
		}
	}
	teamOf := func(p battleParty) (int, bool) {
		org := p.Alliance
		if org == 0 {
			org = p.Corporation
		}
		i, ok := orgs[org]
		return i, ok
	}
	found := false
	seen := map[int64]bool{}
	for _, km := range report.Killmails {
		if km.ID <= 0 || seen[km.ID] {
			return nil, ErrInvalid
		}
		seen[km.ID] = true
		if km.ID == seed {
			found = true
			if i, ok := teamOf(km.Victim); ok {
				s.Teams[i].Own = true
			}
		}
		if i, ok := teamOf(km.Victim); ok && km.Victim.Character > 0 {
			pilots[i][km.Victim.Character] = true
		}
		for _, p := range km.Attackers {
			if i, ok := teamOf(p); ok && p.Character > 0 {
				pilots[i][p.Character] = true
			}
		}
	}
	if !found {
		return nil, ErrInvalid
	}
	for i := range s.Teams {
		s.Teams[i].Pilots = len(pilots[i])
	}
	return s, nil
}

func (s *Service) caseBattleReport(ctx context.Context, actor string, id int64) (BattleReport, error) {
	c, err := s.Read(ctx, actor, id)
	if err != nil {
		return BattleReport{}, err
	}
	var d Detail
	if c.Kind != "solo" || json.Unmarshal(c.Detail, &d) != nil || d.KillmailID <= 0 {
		return BattleReport{}, pgx.ErrNoRows
	}
	return s.BattleReports.Get(ctx, d.KillmailID), nil
}
func (s *Service) lossBattleReport(ctx context.Context, actor string, corp, character, id int64) (BattleReport, error) {
	if s.Losses == nil || corp <= 0 || character <= 0 || id <= 0 {
		return BattleReport{}, ErrInvalid
	}
	rows, err := s.Losses(ctx, actor, corp, character, 0, id)
	if err != nil {
		return BattleReport{}, err
	}
	if len(rows) != 1 || rows[0].ID != id || rows[0].CharacterID != character || rows[0].CorporationID != corp {
		return BattleReport{}, pgx.ErrNoRows
	}
	return s.BattleReports.Get(ctx, id), nil
}
func (h Handler) battleReport(w http.ResponseWriter, r *http.Request) {
	var value BattleReport
	var err error
	if rawID := chi.URLParam(r, "id"); rawID != "" {
		id := number(rawID)
		if id <= 0 {
			respond(w, r, nil, ErrInvalid)
			return
		}
		value, err = h.Service.caseBattleReport(r.Context(), h.User(r), id)
	} else {
		q := r.URL.Query()
		value, err = h.Service.lossBattleReport(r.Context(), h.User(r), number(q.Get("corporation_id")), number(q.Get("character_id")), number(q.Get("killmail_id")))
	}
	respond(w, r, value, err)
}

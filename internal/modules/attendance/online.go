package attendance

import (
	"context"
	"crypto/subtle"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/eve"
	"slices"
	"sort"
	"time"
)

type Day struct {
	Samples int64   `json:"samples"`
	Date    string  `json:"date"`
	Seconds float64 `json:"seconds"`
}
type OnlineMember struct {
	Samples    int64                 `json:"samples"`
	UserID     string                `json:"user_id"`
	Name       string                `json:"name"`
	Seconds    float64               `json:"seconds"`
	Characters []eve.OnlineCharacter `json:"characters"`
}
type OnlineReport struct {
	Since              time.Time      `json:"since"`
	Until              time.Time      `json:"until"`
	Days               []Day          `json:"days"`
	Members            []OnlineMember `json:"members"`
	Seconds            float64        `json:"seconds"`
	ObservedCharacters int            `json:"observed_characters"`
	Estimated          bool           `json:"estimated"`
}
type span struct{ start, end time.Time }

func union(spans []span) []span {
	sort.Slice(spans, func(i, j int) bool { return spans[i].start.Before(spans[j].start) })
	result := []span{}
	for _, s := range spans {
		if !s.end.After(s.start) {
			continue
		}
		n := len(result)
		if n == 0 || s.start.After(result[n-1].end) {
			result = append(result, s)
		} else if s.end.After(result[n-1].end) {
			result[n-1].end = s.end
		}
	}
	return result
}

// Report uses the account union, not the sum of character durations. Chinese
// calendar days are fixed UTC+08:00; unknown gaps never become online intervals.
func (s *Service) Report(ctx context.Context, user, member string, corp int64, days int) (OnlineReport, error) {
	now := time.Now()
	zone := time.FixedZone("UTC+08:00", 8*3600)
	local := now.In(zone)
	since := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, zone).AddDate(0, 0, -days+1)
	result := OnlineReport{Since: since, Until: now, Days: []Day{}, Members: []OnlineMember{}, Estimated: true}
	if days != 7 && days != 30 {
		return result, ErrInvalid
	}
	bindings, err := s.ReportBindings(ctx, user, member, corp)
	if err != nil {
		return result, err
	}
	if len(bindings) > 2000 {
		return result, ErrInvalid
	}
	ids := make([]int64, 0, len(bindings))
	for _, b := range bindings {
		ids = append(ids, b.ID)
	}
	// Recheck administrator/role visibility, which is independent of binding locks.
	again, err := s.ReportBindings(ctx, user, member, corp)
	if err != nil {
		return result, err
	}
	current := map[int64]string{}
	for _, b := range again {
		current[b.ID] = b.UserID
	}
	for _, b := range bindings {
		if current[b.ID] != b.UserID {
			return result, pgx.ErrNoRows
		}
	}
	// Prevent an unlink/rebind between object authorization and reading samples.
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(context.Background())
	locked, err := s.Bindings(ctx, tx, ids)
	if err != nil {
		return result, err
	}
	if len(locked) != len(bindings) {
		return result, pgx.ErrNoRows
	}
	owners := map[int64]Binding{}
	for _, b := range locked {
		owners[b.ID] = b
	}
	for _, b := range bindings {
		v, ok := owners[b.ID]
		if !ok || v.UserID != b.UserID || subtle.ConstantTimeCompare(v.OwnerHash, b.OwnerHash) != 1 {
			return result, pgx.ErrNoRows
		}
	}
	data, err := s.EVE.OnlineDataTx(ctx, tx, ids, since, now, corp)
	if err != nil {
		return result, err
	}
	accounts := map[string]*OnlineMember{}
	for _, b := range bindings {
		if accounts[b.UserID] == nil {
			accounts[b.UserID] = &OnlineMember{UserID: b.UserID, Name: b.Name, Characters: []eve.OnlineCharacter{}}
		}
	}
	observed := map[int64]eve.OnlineCharacter{}
	for _, c := range data.Characters {
		observed[c.ID] = c
	}
	for _, b := range bindings {
		c, ok := observed[b.ID]
		if !ok {
			c = eve.OnlineCharacter{ID: b.ID, State: "unknown"}
		}
		c.Name = b.Name
		accounts[b.UserID].Characters = append(accounts[b.UserID].Characters, c)

	}
	accountSpans := map[string][]span{}
	for _, p := range data.Spans {
		if b, ok := owners[p.CharacterID]; ok {
			accountSpans[b.UserID] = append(accountSpans[b.UserID], span{p.Start, p.End})
		}
	}
	daily := map[string]float64{}
	sampleCounts := map[string]int64{}
	sampled := map[int64]bool{}
	for _, c := range data.Counts {
		sampleCounts[c.Day] += c.Count
		sampled[c.CharacterID] = true
		if b, ok := owners[c.CharacterID]; ok {
			accounts[b.UserID].Samples += c.Count
		}
	}
	result.ObservedCharacters = len(sampled)
	for account, spans := range accountSpans {
		for _, p := range union(spans) {
			seconds := p.end.Sub(p.start).Seconds()
			accounts[account].Seconds += seconds
			result.Seconds += seconds
			for cursor := p.start.In(zone); cursor.Before(p.end); {
				boundary := time.Date(cursor.Year(), cursor.Month(), cursor.Day()+1, 0, 0, 0, 0, zone)
				end := p.end
				if boundary.Before(end) {
					end = boundary
				}
				daily[cursor.Format("2006-01-02")] += end.Sub(cursor).Seconds()
				cursor = end
			}
		}
	}
	for i := 0; i < days; i++ {
		date := since.AddDate(0, 0, i).Format("2006-01-02")
		result.Days = append(result.Days, Day{Samples: sampleCounts[date], Date: date, Seconds: daily[date]})
	}
	for _, m := range accounts {
		slices.SortFunc(m.Characters, func(a, b eve.OnlineCharacter) int {
			if a.ID < b.ID {
				return -1
			}
			if a.ID > b.ID {
				return 1
			}
			return 0
		})
		result.Members = append(result.Members, *m)
	}
	sort.Slice(result.Members, func(i, j int) bool {
		a, b := result.Members[i], result.Members[j]
		if a.Seconds != b.Seconds {
			return a.Seconds > b.Seconds
		}
		return a.UserID < b.UserID
	})
	return result, nil
}

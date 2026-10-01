package eve

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
	"slices"
	"time"
)

type onlineArgs syncArgs

const onlineSampleMaxGap = 5 * time.Minute

func (onlineArgs) Kind() string { return "eve.character-online.v1" }

type onlineWorker struct {
	river.WorkerDefaults[onlineArgs]
	s *SyncService
}

func (w *onlineWorker) Work(ctx context.Context, j *river.Job[onlineArgs]) error {
	if !w.s.onlineEnabled {
		return river.JobSnooze(time.Hour)
	}
	return w.s.work(ctx, syncArgs(j.Args), j.ID, "online")
}
func (s *SyncService) SetOnlineEnabled(enabled bool) { s.onlineEnabled = enabled }

type onlineObservation struct {
	value bool
	at    time.Time
}

func (s *SyncService) collectOnline(ctx context.Context, t store.EveSyncTarget, c store.EveCredential) (syncResult, error) {
	var result syncResult
	var data struct {
		Online *bool `json:"online"`
	}
	response, err := s.auth.esi.Request(ctx, ESIRequest{Method: "GET", Path: fmt.Sprintf("/characters/%d/online/", c.CharacterID), CharacterID: c.CharacterID, Generation: c.GrantGeneration, Scopes: []string{OnlineReadScope}}, &data)
	if err != nil {
		return result, err
	}
	if data.Online == nil || response.ValidatedAt.IsZero() || time.Since(response.ValidatedAt) > onlineSampleMaxGap {
		return result, syncFault{Reason: "invalid_response", Status: 200, Temporary: true}
	}
	if !response.Cached {
		result.online = &onlineObservation{*data.Online, response.ValidatedAt}
	}
	result.next = response.ExpiresAt
	result.content = response.ContentUpdatedAt
	// no-cache upstream responses must not create a tight polling loop.
	if floor := response.ValidatedAt.Add(time.Minute); result.next.Before(floor) {
		result.next = floor
	}
	return result, nil
}

type OnlineSpan struct {
	CharacterID int64
	Start, End  time.Time
}
type OnlineCharacter struct {
	ID         int64      `json:"id,string"`
	Name       string     `json:"name"`
	State      string     `json:"state"`
	ObservedAt *time.Time `json:"observed_at"`
}
type SampleCount struct {
	CharacterID int64
	Day         string
	Count       int64
}
type OnlineData struct {
	Counts     []SampleCount
	Characters []OnlineCharacter
	Spans      []OnlineSpan
}

// Caller supplies only actively bound, authorized-to-view IDs. No account joins here.
func (s *AuthorizationService) OnlineData(ctx context.Context, ids []int64, since, until time.Time, corporation ...int64) (OnlineData, error) {
	return s.OnlineDataTx(ctx, nil, ids, since, until, corporation...)
}
func (s *AuthorizationService) OnlineDataTx(ctx context.Context, tx pgx.Tx, ids []int64, since, until time.Time, corporation ...int64) (OnlineData, error) {
	corp := int64(0)
	if len(corporation) > 0 {
		corp = corporation[0]
	}
	result := OnlineData{Characters: []OnlineCharacter{}, Spans: []OnlineSpan{}}
	q := store.New(s.pool)
	if tx != nil {
		q = store.New(tx)
	}
	rows, err := q.OnlineLatest(ctx, ids)
	if err != nil {
		return result, err
	}
	for _, r := range rows {
		state := "unknown"
		if r.State == "reauthorize" {
			state = "reauthorize"
		} else if !slices.Contains(r.Scopes, OnlineReadScope) {
			state = "missing_scope"
		} else if (corp == 0 || r.ObservedCorporationID == corp) && r.Generation == r.GrantGeneration && r.Online.Valid && r.ObservedAt.Valid && until.Sub(r.ObservedAt.Time) >= 0 && until.Sub(r.ObservedAt.Time) <= onlineSampleMaxGap {
			state = "offline"
			if r.Online.Bool {
				state = "online"
			}
		}
		result.Characters = append(result.Characters, OnlineCharacter{r.CharacterID, r.Name.String, state, optionalTime(r.ObservedAt)})
	}
	spans, err := q.OnlineIntervals(ctx, store.OnlineIntervalsParams{CorporationID: corp, Ids: ids, Since: timestamp(since), UntilAt: timestamp(until)})
	if err != nil {
		return result, err
	}
	counts, err := q.OnlineSampleCounts(ctx, store.OnlineSampleCountsParams{CorporationID: corp, Ids: ids, Since: timestamp(since), UntilAt: timestamp(until)})
	if err != nil {
		return result, err
	}
	for _, r := range counts {
		result.Counts = append(result.Counts, SampleCount{r.CharacterID, r.Day, r.Samples})
	}
	for _, r := range spans {
		result.Spans = append(result.Spans, OnlineSpan{r.CharacterID, r.StartsAt.Time, r.EndsAt.Time})
	}
	return result, nil
}

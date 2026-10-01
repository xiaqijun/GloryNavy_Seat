package eve

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"sync"
	"time"

	"glorynavy.local/seat/internal/httpapi"
	"golang.org/x/sync/singleflight"
)

const publicCombatURL = "https://zkillboard.com/api/stats/corporationID/98530802/kills/"

type PublicCombatMonth struct {
	Month string   `json:"month"`
	Kills *int64   `json:"kills"`
	Value *float64 `json:"value"`
}
type PublicCombat struct {
	Months    []PublicCombatMonth `json:"months"`
	UpdatedAt time.Time           `json:"updated_at"`
	Stale     bool                `json:"stale"`
}

// No names, character IDs, locations or individual online states are published.
type PublicOnline struct {
	Characters *int      `json:"characters"`
	Covered    int       `json:"covered_characters"`
	Bound      int       `json:"bound_characters"`
	UpdatedAt  time.Time `json:"updated_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}
type PublicActivity struct {
	CorporationID int64         `json:"corporation_id"`
	Combat        *PublicCombat `json:"combat"`
	Online        *PublicOnline `json:"online"`
}

// Each source has its own refresh clock and failure behavior. Public visitors
// cannot change the corporation, upstream URL or trigger per-character ESI calls.
type publicSnapshot[T any] struct {
	mu     sync.Mutex
	data   *T
	next   time.Time
	flight singleflight.Group
}

func (s *publicSnapshot[T]) get(ctx context.Context, timeout, ttl time.Duration, fetch func(context.Context) (*T, error), stale func(*T) *T) *T {
	s.mu.Lock()
	if time.Now().Before(s.next) {
		data := s.data
		s.mu.Unlock()
		return data
	}
	s.mu.Unlock()
	result := s.flight.DoChan("refresh", func() (any, error) {
		// A caller can have observed the old deadline just before another flight finished.
		s.mu.Lock()
		if time.Now().Before(s.next) {
			data := s.data
			s.mu.Unlock()
			return data, nil
		}
		s.mu.Unlock()
		refresh, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
		defer cancel()
		data, err := fetch(refresh)
		s.mu.Lock()
		defer s.mu.Unlock()
		s.next = time.Now().Add(ttl)
		if err != nil {
			s.next = time.Now().Add(time.Minute)
			data = nil
			if stale != nil && s.data != nil {
				data = stale(s.data)
			}
		}
		s.data = data
		return data, nil
	})
	select {
	case <-ctx.Done():
		return nil
	case r := <-result:
		return r.Val.(*T)
	}
}

type PublicActivityService struct {
	client *http.Client
	Online func(context.Context) (*PublicOnline, error)
	combat publicSnapshot[PublicCombat]
	online publicSnapshot[PublicOnline]
}

func NewPublicActivity() *PublicActivityService {
	return &PublicActivityService{client: &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (s *PublicActivityService) fetchCombat(ctx context.Context) (*PublicCombat, error) {
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, publicCombatURL, nil)
	if err != nil {
		return nil, err
	}
	r.Header.Set("User-Agent", "GloryNavy/1.0 (+https://seat.kisectool.com)")
	r.Header.Set("Accept", "application/json")
	response, err := s.client.Do(r)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, errors.New("public combat unavailable")
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 2*1024*1024+1))
	if err != nil || len(raw) > 2*1024*1024 {
		return nil, errors.New("invalid public combat response")
	}
	return decodePublicCombat(raw, time.Now().UTC())
}
func decodePublicCombat(raw []byte, now time.Time) (*PublicCombat, error) {
	var data struct {
		ID     int64  `json:"id"`
		Type   string `json:"type"`
		Months map[string]struct {
			Year   int      `json:"year"`
			Month  int      `json:"month"`
			Kills  *int64   `json:"shipsDestroyed"`
			Value  *float64 `json:"iskDestroyed"`
			Losses *int64   `json:"shipsLost"`
		} `json:"months"`
	}
	if json.Unmarshal(raw, &data) != nil || data.ID != publicCorporationID || data.Type != "corporationID" || data.Months == nil {
		return nil, errors.New("invalid public combat response")
	}
	result := &PublicCombat{UpdatedAt: now, Months: make([]PublicCombatMonth, 0, 6)}
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	for i := -5; i <= 0; i++ {
		month := start.AddDate(0, i, 0)
		row := PublicCombatMonth{Month: month.Format("2006-01")}
		if value, ok := data.Months[month.Format("200601")]; ok {
			if value.Year != month.Year() || value.Month != int(month.Month()) {
				return nil, errors.New("invalid combat month")
			}
			// zKill omits the kill side for loss-only months. A wholly absent month remains unknown.
			if value.Kills == nil && value.Value == nil && (value.Losses == nil || *value.Losses < 0) {
				result.Months = append(result.Months, row)
				continue
			}
			if (value.Kills == nil) != (value.Value == nil) {
				return nil, errors.New("incomplete combat totals")
			}
			kills, isk := int64(0), float64(0)
			if value.Kills != nil {
				kills = *value.Kills
			}
			if value.Value != nil {
				isk = *value.Value
			}
			if kills < 0 || kills > 9007199254740991 || isk < 0 || math.IsNaN(isk) || math.IsInf(isk, 0) {
				return nil, errors.New("invalid combat totals")
			}
			row.Kills = &kills
			row.Value = &isk
		}
		result.Months = append(result.Months, row)
	}
	return result, nil
}
func (s *PublicActivityService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var result PublicActivity
	result.CorporationID = publicCorporationID
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		result.Combat = s.combat.get(r.Context(), 8*time.Second, 10*time.Minute, s.fetchCombat, func(old *PublicCombat) *PublicCombat { copy := *old; copy.Stale = true; return &copy })
	}()
	go func() {
		defer wg.Done()
		if s.Online != nil {
			s.online.mu.Lock()
			if s.online.data != nil && !time.Now().Before(s.online.data.ExpiresAt) {
				s.online.next = time.Time{}
			}
			s.online.mu.Unlock()
			result.Online = s.online.get(r.Context(), 3*time.Second, 30*time.Second, s.Online, nil)
		}
	}()
	wg.Wait()
	// Online observations must not survive their original freshness window.
	if result.Online != nil && !time.Now().Before(result.Online.ExpiresAt) {
		result.Online = nil
	}
	w.Header().Set("Cache-Control", "no-store")
	httpapi.Respond(w, r, http.StatusOK, result)
}

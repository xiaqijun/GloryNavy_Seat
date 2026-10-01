package eve

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"glorynavy.local/seat/internal/httpapi"
	"golang.org/x/sync/singleflight"
)

const publicCorporationID int64 = 98530802

type publicESI interface {
	Request(context.Context, ESIRequest, any) (ESIResponse, error)
}

// PublicOverview contains an explicit projection of public ESI information.
// Never add authenticated member, asset, wallet or activity data here.
type PublicOverview struct {
	CorporationID int64           `json:"corporation_id"`
	Name          string          `json:"name"`
	Ticker        string          `json:"ticker"`
	MemberCount   int             `json:"member_count"`
	Founded       time.Time       `json:"date_founded"`
	Alliance      *PublicAlliance `json:"alliance,omitempty"`
	UpdatedAt     time.Time       `json:"updated_at"`
	Stale         bool            `json:"stale"`
}
type PublicAlliance struct {
	ID               int64  `json:"id"`
	Name             string `json:"name"`
	Ticker           string `json:"ticker"`
	CorporationCount *int   `json:"corporation_count"`
}
type PublicCorporationService struct {
	esi    publicESI
	mu     sync.Mutex
	cached *PublicOverview
	next   time.Time
	flight singleflight.Group
}

func NewPublicCorporation(esi *ESIService) *PublicCorporationService {
	s := &PublicCorporationService{}
	if esi != nil {
		s.esi = esi
	}
	return s
}

func (s *PublicCorporationService) load(ctx context.Context) (PublicOverview, error) {
	s.mu.Lock()
	if time.Now().Before(s.next) {
		cached := s.cached
		s.mu.Unlock()
		if cached != nil {
			return *cached, nil
		}
		return PublicOverview{}, errESI
	}
	s.mu.Unlock()
	result := s.flight.DoChan("public-corporation", func() (any, error) {
		// One short shared refresh survives an individual visitor navigating away.
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 12*time.Second)
		defer cancel()
		data, until, err := s.fetch(ctx)
		s.mu.Lock()
		defer s.mu.Unlock()
		if err != nil {
			s.next = time.Now().Add(time.Minute)
			if s.cached != nil {
				previous := *s.cached
				previous.Stale = true
				s.cached = &previous
				return previous, nil
			}
			return PublicOverview{}, errESI
		}
		s.cached, s.next = &data, until
		return data, nil
	})
	select {
	case <-ctx.Done():
		return PublicOverview{}, ctx.Err()
	case r := <-result:
		if r.Err != nil {
			return PublicOverview{}, r.Err
		}
		return r.Val.(PublicOverview), nil
	}
}

func (s *PublicCorporationService) fetch(ctx context.Context) (PublicOverview, time.Time, error) {
	if s.esi == nil {
		return PublicOverview{}, time.Time{}, errESI
	}
	var corporation struct {
		Name        string    `json:"name"`
		Ticker      string    `json:"ticker"`
		MemberCount *int      `json:"member_count"`
		Founded     time.Time `json:"date_founded"`
		AllianceID  int64     `json:"alliance_id"`
	}
	observation, err := s.esi.Request(ctx, ESIRequest{Method: http.MethodGet, Path: fmt.Sprintf("/corporations/%d/", publicCorporationID)}, &corporation)
	if err != nil || corporation.MemberCount == nil || *corporation.MemberCount < 0 || corporation.Name == "" || corporation.Founded.IsZero() {
		return PublicOverview{}, time.Time{}, errESI
	}
	now := time.Now().UTC()
	updated := observation.ValidatedAt
	if updated.IsZero() {
		updated = observation.ContentUpdatedAt
	}
	if updated.IsZero() {
		updated = now
	}
	until := observation.ExpiresAt
	if !until.After(now) {
		until = now.Add(time.Minute)
	}
	data := PublicOverview{CorporationID: publicCorporationID, Name: corporation.Name, Ticker: corporation.Ticker, MemberCount: *corporation.MemberCount, Founded: corporation.Founded, UpdatedAt: updated}
	if corporation.AllianceID > 0 {
		alliance := PublicAlliance{ID: corporation.AllianceID}
		a, err := s.esi.Request(ctx, ESIRequest{Method: http.MethodGet, Path: fmt.Sprintf("/alliances/%d/", corporation.AllianceID)}, &alliance)
		if err == nil && alliance.Name != "" {
			if a.ExpiresAt.After(now) && a.ExpiresAt.Before(until) {
				until = a.ExpiresAt
			}
			var ids []int64
			b, err := s.esi.Request(ctx, ESIRequest{Method: http.MethodGet, Path: fmt.Sprintf("/alliances/%d/corporations/", corporation.AllianceID)}, &ids)
			if err == nil && ids != nil {
				count := len(ids)
				alliance.CorporationCount = &count
				if b.ExpiresAt.After(now) && b.ExpiresAt.Before(until) {
					until = b.ExpiresAt
				}
			} else {
				until = now.Add(time.Minute)
			}
			data.Alliance = &alliance
		} else {
			until = now.Add(time.Minute)
		}
	}
	return data, until, nil
}

func (s *PublicCorporationService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	data, err := s.load(r.Context())
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=30")
		httpapi.Failure(w, r, 503, "public_profile_unavailable", "公开军团资料暂不可用")
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	httpapi.Respond(w, r, http.StatusOK, data)
}

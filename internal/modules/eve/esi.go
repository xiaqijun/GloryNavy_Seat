package eve

import (
	"context"
	"crypto/cipher"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/modules/eve/internal/esiclient"
	"net/http"
	"time"
)

const compatibilityDate = esiclient.CompatibilityDate

func retryUntil(h http.Header, now time.Time) time.Time { return esiclient.RetryUntil(h, now) }

var errESI = errors.New("ESI data unavailable")

type retryError = esiclient.RetryError
type syncFault = esiclient.Fault
type esiObservation = esiclient.Observation
type esiObservationKey = esiclient.ObservationKey
type esiCredential = esiclient.Credential
type esiCredentialKey = esiclient.CredentialKey

// ESIService is the token-free gateway injected into trusted business modules.
// The caller must establish object authorization before selecting a character.
type ESIService struct {
	shared      *esiclient.Client
	http        *http.Client
	credentials *AuthorizationService
}
type ESIRequest struct {
	Method, Path            string
	Body                    []byte
	CharacterID, Generation int64
	Scopes                  []string
	ExpectPages, LimitTrust bool
	ForceRefresh            bool
	Mutation                bool
}
type ESIResponse struct {
	Cached                                               bool
	ExpiresAt, ContentUpdatedAt, TrustUntil, ValidatedAt time.Time
	Pages                                                int
}

func newESI(pool *pgxpool.Pool, box cipher.AEAD) *ESIService {
	return &ESIService{shared: esiclient.New(pool, box), http: &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (s *AuthorizationService) ESI() *ESIService { return s.esi }
func (c *ESIService) Request(ctx context.Context, r ESIRequest, out any) (ESIResponse, error) {
	o := &esiObservation{ExpectPages: r.ExpectPages, LimitTrust: r.LimitTrust}
	ctx = context.WithValue(ctx, esiObservationKey{}, o)
	if r.ForceRefresh {
		ctx = context.WithValue(ctx, esiclient.ForceRefreshKey{}, true)
	}
	if r.Mutation {
		ctx = context.WithValue(ctx, esiclient.MutationKey{}, true)
	}
	var until time.Time
	var err error
	if r.CharacterID == 0 && r.Generation == 0 && len(r.Scopes) == 0 {
		until, err = c.request(ctx, r.Method, r.Path, "", r.Body, out)
	} else if r.CharacterID > 0 && r.Generation > 0 && len(r.Scopes) > 0 && c.credentials != nil {
		until, err = c.authorized(ctx, r.CharacterID, r.Generation, r.Scopes, r.Method, r.Path, r.Body, out)
	} else {
		err = syncFault{Reason: "credential_context_missing"}
	}
	return ESIResponse{Cached: o.Cached, ExpiresAt: until, ContentUpdatedAt: o.Content, TrustUntil: o.TrustUntil, ValidatedAt: o.ValidatedAt, Pages: o.Pages}, err
}
func (c *ESIService) request(ctx context.Context, method, path, token string, body []byte, out any) (time.Time, error) {
	return c.shared.Request(ctx, c.http, method, path, token, body, out)
}
func (c *ESIService) authorized(ctx context.Context, id, generation int64, scopes []string, method, path string, body []byte, out any) (time.Time, error) {
	return c.withToken(ctx, id, generation, scopes, func(ctx context.Context, token string) (time.Time, error) {
		return c.request(ctx, method, path, token, body, out)
	})
}
func (c *ESIService) withToken(ctx context.Context, id, generation int64, scopes []string, call func(context.Context, string) (time.Time, error)) (time.Time, error) {
	token, err := c.credentials.currentToken(ctx, id, generation, "", scopes...)
	if err != nil {
		return time.Time{}, err
	}
	ctx = context.WithValue(ctx, esiCredentialKey{}, esiCredential{ID: id, Generation: generation})
	until, err := call(ctx, token)
	var fault syncFault
	if errors.As(err, &fault) && fault.Status == 401 {
		token, err = c.credentials.currentToken(ctx, id, generation, token, scopes...)
		if err == nil {
			until, err = call(ctx, token)
		}
	}
	return until, err
}
func characterPath(id int64) string { return fmt.Sprintf("/characters/%d/", id) }

type roleData struct {
	Roles []string `json:"roles"`
	HQ    []string `json:"roles_at_hq"`
	Base  []string `json:"roles_at_base"`
	Other []string `json:"roles_at_other"`
}
type corporationData struct {
	Name     string `json:"name"`
	CEO      int64  `json:"ceo_id"`
	Alliance int64  `json:"alliance_id"`
}
type esiSnapshot struct {
	Content       time.Time
	Barrier       time.Time
	CorporationID int64
	Corporation   corporationData
	Roles         roleData
	Next          time.Time
}

func (c *ESIService) snapshot(ctx context.Context, id, generation int64) (esiSnapshot, error) {
	var snapshot esiSnapshot
	observation := &esiObservation{LimitTrust: true}
	ctx = context.WithValue(ctx, esiObservationKey{}, observation)
	_, err := c.withToken(ctx, id, generation, []string{CorporationRolesScope}, func(ctx context.Context, token string) (time.Time, error) {
		var err error
		snapshot, err = c.snapshotWithToken(ctx, id, token)
		return snapshot.Next, err
	})
	snapshot.Content = observation.Content
	if !observation.TrustUntil.IsZero() && observation.TrustUntil.Before(snapshot.Next) {
		snapshot.Next = observation.TrustUntil
	}
	return snapshot, err
}
func (c *ESIService) snapshotWithToken(ctx context.Context, id int64, token string) (esiSnapshot, error) {
	s := esiSnapshot{Next: time.Now().Add(2 * time.Hour)}
	var aff []struct {
		Character   int64 `json:"character_id"`
		Corporation int64 `json:"corporation_id"`
	}
	body, _ := json.Marshal([]int64{id})
	until, err := c.request(ctx, "POST", "/characters/affiliation/", "", body, &aff)
	if err != nil {
		return s, err
	}
	if len(aff) != 1 || aff[0].Character != id || aff[0].Corporation <= 0 {
		return s, errESI
	}
	s.CorporationID = aff[0].Corporation
	if until.After(s.Barrier) {
		s.Barrier = until
	}
	if until.Before(s.Next) {
		s.Next = until
	}
	until, err = c.request(ctx, "GET", fmt.Sprintf("/characters/%d/roles/", id), token, nil, &s.Roles)
	if err != nil {
		return s, err
	}
	if s.Roles.Roles == nil {
		return s, errESI
	}
	if until.After(s.Barrier) {
		s.Barrier = until
	}
	if until.Before(s.Next) {
		s.Next = until
	}
	until, err = c.request(ctx, "GET", fmt.Sprintf("/corporations/%d/", s.CorporationID), "", nil, &s.Corporation)
	if err != nil {
		return s, err
	}
	if s.Corporation.CEO <= 0 || s.Corporation.Name == "" {
		return s, errESI
	}
	if until.After(s.Barrier) {
		s.Barrier = until
	}
	if until.Before(s.Next) {
		s.Next = until
	}
	// Long-lived ESI caches must not silently create long-lived site privileges.
	if s.Next.After(time.Now().Add(2 * time.Hour)) {
		return s, errESI
	}
	return s, nil
}

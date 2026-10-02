package esiclient

import (
	"bytes"
	"context"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
)

type CredentialKey struct{}
type MutationKey struct{}
type Credential struct{ ID, Generation int64 }
type ObservationKey struct{}
type Observation struct {
	Cached                           bool
	Content, TrustUntil, ValidatedAt time.Time
	LimitTrust                       bool
	ExpectPages                      bool
	Pages                            int
}
type Client struct {
	pool      *pgxpool.Pool
	box       cipher.AEAD
	userAgent string
	Observer  func(context.Context, Credential, RequestEvent)
}

// ForceRefreshKey asks the ESI client to revalidate a cached response. It is
// used when a local projection is incomplete and must be enriched from the
// upstream detail, even if the normal route cache is still fresh.
type ForceRefreshKey struct{}

// RequestEvent carries operational metadata only, never headers, paths or bodies.
type RequestEvent struct {
	Status                             int
	Reason                             string
	Network, CacheHit, Limited, Failed bool
}

var pathNumber = regexp.MustCompile(`/[0-9]+`)

func (s *Client) Request(ctx context.Context, client *http.Client, method, path, token string, body []byte, out any) (until time.Time, resultErr error) {
	now := time.Now()
	parsed, parseErr := url.Parse(path)
	if parseErr != nil || !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || parsed.Host != "" || parsed.Scheme != "" || parsed.Fragment != "" || (method != "GET" && method != "POST") {
		return now, Fault{Reason: "invalid_request"}
	}
	cred, _ := ctx.Value(CredentialKey{}).(Credential)
	mutation, _ := ctx.Value(MutationKey{}).(bool)
	if mutation && (method != "POST" || routeKey(method, path) != "POST /characters/{id}/fittings/" || token == "") {
		return now, Fault{Reason: "invalid_request"}
	}
	event := RequestEvent{}
	defer func() {
		if token == "" || cred.ID == 0 || s.Observer == nil {
			return
		}
		var retry RetryError
		var fault Fault
		if resultErr != nil {
			event.Failed = true
			event.Reason = "storage_error"
			if errors.As(resultErr, &retry) {
				event.Limited = true
				event.Failed = false
				event.Reason = "rate_limited"
			}
			if errors.As(resultErr, &fault) {
				event.Reason = fault.Reason
			}
		}
		s.Observer(ctx, cred, event)
	}()
	if token != "" && cred.ID == 0 {
		return now, Fault{"credential_context_missing", 0, false}
	}
	scope := "public"
	if token != "" {
		scope = fmt.Sprintf("character:%d:%d", cred.ID, cred.Generation)
	}
	cacheIdentity := method + "\n" + path + "\n" + string(body) + "\n" + scope + "\n" + CompatibilityDate
	digest := sha256.Sum256([]byte(cacheIdentity))
	key := hex.EncodeToString(digest[:])
	q := store.New(s.pool)
	route := routeKey(method, path)
	identity := "public-egress"
	if token != "" {
		identity = fmt.Sprintf("character:%d", cred.ID)
	}
	policy, routeErr := s.policy(ctx, route)
	if routeErr != nil {
		return now, routeErr
	}
	group := policy.Group
	limitKey := "route:" + route + ":" + identity
	if group != "" {
		limitKey = "group:" + group + ":" + identity
	}
	rate := rateObservation{route: route, group: group, identity: identity, character: cred.ID}
	// Authorization snapshots also make public requests inside a credential
	// context. Only an actually attached bearer token selects a private bucket.
	if token == "" {
		rate.character = 0
	}
	defer func() { s.recordRate(ctx, rate) }()
	var cached store.EveEsiCache
	err := pgx.ErrNoRows
	forceRefresh, _ := ctx.Value(ForceRefreshKey{}).(bool)
	if !mutation {
		cached, err = q.ReadESICache(ctx, key)
	}
	found := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return now, err
	}
	var data []byte
	if found {
		data = cached.Body
		if token != "" {
			n := s.box.NonceSize()
			if len(data) < n {
				return now, Fault{"cache_invalid", 0, false}
			}
			data, err = s.box.Open(nil, data[:n], data[n:], []byte(key))
			if err != nil {
				return now, Fault{"cache_invalid", 0, false}
			}
		}
		if !forceRefresh && cached.ExpiresAt.Time.After(now) {
			event.CacheHit = true
			rate.cache = true
			if o, ok := ctx.Value(ObservationKey{}).(*Observation); ok && o.ExpectPages {
				if cached.PageCount < 1 {
					return now, Fault{"pagination_invalid", 0, true}
				}
				o.Pages = int(cached.PageCount)
			}
			if o, ok := ctx.Value(ObservationKey{}).(*Observation); ok && o.LimitTrust {
				trust := cached.UpdatedAt.Time.Add(2 * time.Hour)
				if !trust.After(now) {
					return cached.ExpiresAt.Time, RetryError{cached.ExpiresAt.Time}
				}
				if o.TrustUntil.IsZero() || trust.Before(o.TrustUntil) {
					o.TrustUntil = trust
				}
			}
			s.observe(ctx, cached.ContentUpdatedAt.Time)
			if o, ok := ctx.Value(ObservationKey{}).(*Observation); ok {
				o.ValidatedAt = cached.UpdatedAt.Time
				o.Cached = true
			}
			return cached.ExpiresAt.Time, json.Unmarshal(data, out)
		}
	}
	networkCtx, cancelNetwork := context.WithTimeout(ctx, requestDeadline)
	defer cancelNetwork()
	reserved, err := s.reserve(networkCtx, limitKey, policy)
	if err != nil {
		var retry RetryError
		rate.localWait = errors.As(err, &retry)
		return now, err
	}
	req, err := http.NewRequestWithContext(networkCtx, method, "https://esi.evetech.net"+path, bytes.NewReader(body))
	if err != nil {
		return now, Fault{"invalid_request", 0, false}
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Compatibility-Date", CompatibilityDate)
	userAgent := s.userAgent
	if userAgent == "" {
		userAgent = "GloryNavy/0.1.0"
	}
	req.Header.Set("User-Agent", userAgent)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if found && method == "GET" && cached.Etag != "" {
		req.Header.Set("If-None-Match", cached.Etag)
	} else if found && method == "GET" && cached.LastModified != "" {
		req.Header.Set("If-Modified-Since", cached.LastModified)
	}
	event.Network = true
	rate.network = true
	res, err := client.Do(req)
	if err != nil {
		if settleErr := s.settle(ctx, reserved, route, identity, nil); settleErr != nil {
			return now, settleErr
		}
		return now, Fault{"network_error", 0, true}
	}
	defer res.Body.Close()
	event.Status = res.StatusCode
	rate.response(res)
	if err = s.settle(ctx, reserved, route, identity, res); err != nil {
		return now, err
	}
	if res.StatusCode == 420 || res.StatusCode == 429 {
		until := RetryUntil(res.Header, time.Now())
		return until, RetryError{until}
	}
	if (mutation && res.StatusCode != 201) || (!mutation && res.StatusCode != 200 && res.StatusCode != 304) {
		reason := "upstream_unavailable"
		temporary := res.StatusCode >= 500
		switch res.StatusCode {
		case 401:
			reason = "access_token_rejected"
		case 403:
			reason = "access_denied"
		case 404:
			reason = "resource_unavailable"
		}
		return now, Fault{reason, res.StatusCode, temporary}
	}
	expires, persistCache := cacheExpiry(res.Header, route, time.Now())
	content := now
	pages := int32(0)
	if o, ok := ctx.Value(ObservationKey{}).(*Observation); ok && o.ExpectPages {
		if res.StatusCode == 304 && res.Header.Get("X-Pages") == "" {
			pages = cached.PageCount
		} else {
			n, e := strconv.ParseInt(res.Header.Get("X-Pages"), 10, 32)
			if e != nil || n < 1 || n > 10000 {
				return now, Fault{"pagination_invalid", res.StatusCode, true}
			}
			pages = int32(n)
		}
		if pages < 1 {
			return now, Fault{"pagination_invalid", res.StatusCode, true}
		}
		o.Pages = int(pages)
	}
	if res.StatusCode == 304 {
		if !found || len(data) == 0 {
			return now, Fault{"cache_invalid", 304, false}
		}
		content = cached.ContentUpdatedAt.Time
	} else {
		incoming, e := readResponseBody(res.Body, res.StatusCode)
		if e != nil {
			return now, e
		}
		if found && bytes.Equal(incoming, data) {
			content = cached.ContentUpdatedAt.Time
		}
		data = incoming
	}
	if json.Unmarshal(data, out) != nil {
		return now, Fault{"invalid_response", res.StatusCode, false}
	}
	if mutation {
		return time.Now(), nil
	}
	s.observe(ctx, content)
	if o, ok := ctx.Value(ObservationKey{}).(*Observation); ok {
		o.ValidatedAt = time.Now()
	}
	etag := res.Header.Get("ETag")
	modified := res.Header.Get("Last-Modified")
	if res.StatusCode == 304 {
		if etag == "" {
			etag = cached.Etag
		}
		if modified == "" {
			modified = cached.LastModified
		}
	}
	if len(etag) > 1024 || len(modified) > 128 {
		return now, Fault{"invalid_response", res.StatusCode, false}
	}
	persisted := data
	var character pgtype.Int8
	if token != "" {
		character = pgtype.Int8{Int64: cred.ID, Valid: true}
		nonce := make([]byte, s.box.NonceSize())
		if _, err = rand.Read(nonce); err != nil {
			return now, err
		}
		persisted = s.box.Seal(nonce, nonce, data, []byte(key))
	}
	cacheTx, err := s.pool.Begin(ctx)
	if err != nil {
		return now, err
	}
	defer cacheTx.Rollback(context.Background())
	cq := store.New(cacheTx)
	if token != "" {
		current, e := cq.LockCacheCredential(ctx, cred.ID)
		if e != nil {
			return now, e
		}
		if current.GrantGeneration != cred.Generation || current.State == "reauthorize" {
			return now, Fault{"identity_changed", 0, false}
		}
	}
	if !persistCache {
		if err = cq.DeleteESICache(ctx, key); err != nil {
			return now, err
		}
		return expires, cacheTx.Commit(ctx)
	}
	if err = cq.LockCacheQuota(ctx); err != nil {
		return now, err
	}
	usage, err := cq.CacheUsage(ctx)
	if err != nil {
		return now, err
	}
	previousSize := 0
	if latest, e := cq.ReadESICache(ctx, key); e == nil {
		previousSize = len(latest.Body)
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return now, e
	}
	if (previousSize > 0 || usage.Entries < 10000) && usage.Bytes-int64(previousSize)+int64(len(persisted)) <= 64<<20 {
		err = cq.WriteESICache(ctx, store.WriteESICacheParams{CacheKey: key, CharacterID: character, Generation: cred.Generation, Body: persisted, Etag: etag, LastModified: modified, ExpiresAt: timestamp(expires), ContentUpdatedAt: timestamp(content), PageCount: pages})
		if err != nil {
			return now, err
		}
	}
	if err = cacheTx.Commit(ctx); err != nil {
		return now, err
	}
	return expires, nil
}

func (s *Client) observe(ctx context.Context, t time.Time) {
	if o, ok := ctx.Value(ObservationKey{}).(*Observation); ok && t.After(o.Content) {
		o.Content = t
	}
}

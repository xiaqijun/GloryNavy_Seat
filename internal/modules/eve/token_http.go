package eve

import (
	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Explicit allowlist: credential ciphertext, owner hashes and token values must
// never reach an HTTP response. Observation reads do not decrypt credentials.
type tokenObservationDTO struct {
	CharacterID          int64      `json:"character_id,string"`
	Name                 string     `json:"name"`
	State                string     `json:"state"`
	Generation           int64      `json:"generation,string"`
	Scopes               []string   `json:"scopes"`
	ObservedSince        *time.Time `json:"observed_since"`
	AccessExpiresAt      *time.Time `json:"access_expires_at"`
	LastUsedAt           *time.Time `json:"last_used_at"`
	ReuseCount           int64      `json:"reuse_count"`
	LastRefreshAttemptAt *time.Time `json:"last_refresh_attempt_at"`
	LastRefreshSuccessAt *time.Time `json:"last_refresh_success_at"`
	LastRefreshReason    string     `json:"last_refresh_reason"`
	RefreshSuccesses     int64      `json:"refresh_successes"`
	RefreshFailures      int64      `json:"refresh_failures"`
	ConsecutiveFailures  int64      `json:"consecutive_failures"`
	LastRequestAt        *time.Time `json:"last_request_at"`
	LastRequestStatus    int32      `json:"last_request_status"`
	LastRequestReason    string     `json:"last_request_reason"`
	NetworkRequests      int64      `json:"network_requests"`
	CacheHits            int64      `json:"cache_hits"`
	RateLimitWaits       int64      `json:"rate_limit_waits"`
	RequestFailures      int64      `json:"request_failures"`
}

func tokenDTO(r store.ListTokenObservationsRow, now time.Time) tokenObservationDTO {
	state := "valid"
	switch {
	case r.CredentialState == "reauthorize":
		state = "reauthorize"
	case r.ConsecutiveFailures > 0:
		state = "refresh_failed"
	case !r.AccessExpiresAt.Valid:
		state = "unknown"
	case !r.AccessExpiresAt.Time.After(now.Add(time.Minute)):
		state = "refresh_due"
	}
	return tokenObservationDTO{
		CharacterID: r.CharacterID, Name: r.DisplayName, State: state, Generation: r.GrantGeneration, Scopes: r.Scopes,
		ObservedSince: optionalTime(r.ObservedSince), AccessExpiresAt: optionalTime(r.AccessExpiresAt), LastUsedAt: optionalTime(r.LastUsedAt), ReuseCount: r.ReuseCount,
		LastRefreshAttemptAt: optionalTime(r.LastRefreshAttemptAt), LastRefreshSuccessAt: optionalTime(r.LastRefreshSuccessAt), LastRefreshReason: r.LastRefreshReason,
		RefreshSuccesses: r.RefreshSuccesses, RefreshFailures: r.RefreshFailures, ConsecutiveFailures: r.ConsecutiveFailures,
		LastRequestAt: optionalTime(r.LastRequestAt), LastRequestStatus: r.LastRequestStatus, LastRequestReason: r.LastRequestReason,
		NetworkRequests: r.NetworkRequests, CacheHits: r.CacheHits, RateLimitWaits: r.RateLimitWaits, RequestFailures: r.RequestFailures,
	}
}

func (h SyncHTTP) tokens(w http.ResponseWriter, r *http.Request) {
	after := int64(0)
	if raw := r.URL.Query().Get("after"); raw != "" {
		var err error
		after, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || after < 0 {
			httpapi.Failure(w, r, 400, "invalid_cursor", "分页参数无效")
			return
		}
	}
	filter := r.URL.Query().Get("state")
	switch filter {
	case "", "valid", "refresh_due", "refresh_failed", "reauthorize", "unknown":
	default:
		httpapi.Failure(w, r, 400, "invalid_filter", "筛选条件无效")
		return
	}
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	if len([]rune(search)) > 80 {
		httpapi.Failure(w, r, 400, "invalid_filter", "搜索内容过长")
		return
	}
	rows, err := store.New(h.Service.pool).ListTokenObservations(r.Context(), store.ListTokenObservationsParams{AfterID: after, Search: search, StateFilter: filter})
	if err != nil {
		h.fail(w, r)
		return
	}
	next := ""
	if len(rows) > 30 {
		rows = rows[:30]
		next = strconv.FormatInt(rows[29].CharacterID, 10)
	}
	items := make([]tokenObservationDTO, 0, len(rows))
	now := time.Now()
	for _, row := range rows {
		items = append(items, tokenDTO(row, now))
	}
	httpapi.Respond(w, r, 200, map[string]any{"tokens": items, "next_cursor": next, "observed_at": now})
}

func (h SyncHTTP) tokenEvents(w http.ResponseWriter, r *http.Request) {
	id, err := parseSyncID(r)
	if err != nil {
		httpapi.Failure(w, r, 404, "character_unavailable", "角色不可用")
		return
	}
	q := store.New(h.Service.pool)
	exists, err := q.TokenObservationExists(r.Context(), id)
	if err != nil {
		h.fail(w, r)
		return
	}
	if !exists {
		httpapi.Failure(w, r, 404, "character_unavailable", "角色不可用")
		return
	}
	rows, err := q.ListTokenEvents(r.Context(), id)
	if err != nil {
		h.fail(w, r)
		return
	}
	events := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		events = append(events, map[string]any{"id": strconv.FormatInt(row.ID, 10), "generation": strconv.FormatInt(row.Generation, 10), "occurred_at": row.OccurredAt.Time, "outcome": row.Outcome, "reason": row.Reason, "duration_ms": row.DurationMs})
	}
	httpapi.Respond(w, r, 200, map[string]any{"events": events})
}

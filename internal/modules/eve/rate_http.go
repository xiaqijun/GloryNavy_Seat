package eve

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/modules/eve/internal/esiclient"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
)

func optionalCount(n pgtype.Int8) *int64 {
	if !n.Valid {
		return nil
	}
	return &n.Int64
}

// Explicit public metadata fields; internal bucket keys and credentials stay private.
type rateBucketDTO struct {
	ID                 string     `json:"id"`
	Group              string     `json:"group"`
	CharacterID        string     `json:"character_id"`
	Name               string     `json:"name"`
	ObservedSince      time.Time  `json:"observed_since"`
	HeaderAt           *time.Time `json:"header_at"`
	Capacity           *int64     `json:"capacity"`
	WindowSeconds      *int64     `json:"window_seconds"`
	PolicySource       string     `json:"policy_source"`
	Remaining          *int64     `json:"remaining"`
	RetryAt            *time.Time `json:"retry_at"`
	LocalRemaining     *int64     `json:"local_remaining"`
	LocalRecoveryAt    *time.Time `json:"local_recovery_at"`
	LocalBlockedUntil  *time.Time `json:"local_blocked_until"`
	EgressBlockedUntil *time.Time `json:"egress_blocked_until"`
	UsedTokens         int64      `json:"used_tokens"`
	NetworkRequests    int64      `json:"network_requests"`
	UnmeasuredRequests int64      `json:"unmeasured_requests"`
}

func bucketDTO(b store.ListESIBucketsRow, now time.Time) rateBucketDTO {
	character := ""
	if b.CharacterID > 0 {
		character = strconv.FormatInt(b.CharacterID, 10)
	}
	local := &b.LocalRemaining
	recovery := optionalTime(b.LocalResetAt)
	if !b.LocalCapacity.Valid || b.LocalCapacity.Int64 <= 0 {
		local = nil
		recovery = nil
	}
	capacity, window := optionalCount(b.Capacity), optionalCount(b.WindowSeconds)
	source := "response"
	if capacity == nil || window == nil {
		c, w := esiclient.BucketPolicy(b.GroupName)
		if c > 0 {
			capacity, window, source = &c, &w, "openapi"
		} else {
			source = "unknown"
		}
	}
	return rateBucketDTO{ID: strconv.FormatInt(b.ID, 10), Group: b.GroupName, CharacterID: character, Name: b.DisplayName, ObservedSince: b.ObservedSince.Time, HeaderAt: optionalTime(b.HeaderAt), Capacity: capacity, WindowSeconds: window, PolicySource: source, Remaining: optionalCount(b.Remaining), RetryAt: optionalTime(b.RetryAt), LocalRemaining: local, LocalRecoveryAt: recovery, LocalBlockedUntil: optionalTime(b.BlockedUntil), EgressBlockedUntil: optionalTime(b.EgressBlockedUntil), UsedTokens: b.UsedTokens, NetworkRequests: b.NetworkRequests, UnmeasuredRequests: b.UnmeasuredRequests}
}

func (h SyncHTTP) rateLimits(w http.ResponseWriter, r *http.Request) {
	after := int64(0)
	if raw := r.URL.Query().Get("after"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n < 0 {
			httpapi.Failure(w, r, 400, "invalid_cursor", "分页参数无效")
			return
		}
		after = n
	}
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	if len([]rune(search)) > 80 {
		httpapi.Failure(w, r, 400, "invalid_filter", "搜索内容过长")
		return
	}
	rows, err := store.New(h.Service.pool).ListESIBuckets(r.Context(), store.ListESIBucketsParams{AfterID: after, Search: search})
	if err != nil {
		h.fail(w, r)
		return
	}
	next := ""
	if len(rows) > 30 {
		rows = rows[:30]
		next = strconv.FormatInt(rows[29].ID, 10)
	}
	now := time.Now()
	items := make([]rateBucketDTO, 0, len(rows))
	for _, row := range rows {
		items = append(items, bucketDTO(row, now))
	}
	httpapi.Respond(w, r, 200, map[string]any{"buckets": items, "next_cursor": next, "observed_at": now})
}

func (h SyncHTTP) rateRoutes(w http.ResponseWriter, r *http.Request) {
	id, err := parseSyncID(r)
	if err != nil {
		httpapi.Failure(w, r, 404, "bucket_unavailable", "令牌桶不存在")
		return
	}
	q := store.New(h.Service.pool)
	exists, err := q.ESIBucketExists(r.Context(), id)
	if err != nil {
		h.fail(w, r)
		return
	}
	if !exists {
		httpapi.Failure(w, r, 404, "bucket_unavailable", "令牌桶不存在")
		return
	}
	after := r.URL.Query().Get("after")
	if len(after) > 300 {
		httpapi.Failure(w, r, 400, "invalid_cursor", "分页参数无效")
		return
	}
	rows, err := q.ListESIRouteUsage(r.Context(), store.ListESIRouteUsageParams{BucketID: id, AfterRoute: after})
	if err != nil {
		h.fail(w, r)
		return
	}
	next := ""
	if len(rows) > 30 {
		rows = rows[:30]
		next = rows[29].Route
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, map[string]any{"route": row.Route, "network_requests": row.NetworkRequests, "cache_hits": row.CacheHits, "local_waits": row.LocalWaits, "upstream_limits": row.UpstreamLimits, "used_tokens": row.UsedTokens, "measured_responses": row.MeasuredResponses, "unmeasured_requests": row.UnmeasuredRequests, "last_status": row.LastStatus, "last_used": optionalCount(row.LastUsed), "last_response_at": optionalTime(row.LastResponseAt)})
	}
	httpapi.Respond(w, r, 200, map[string]any{"routes": items, "next_cursor": next})
}

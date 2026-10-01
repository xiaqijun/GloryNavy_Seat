package eve

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/module"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
)

type SyncHTTP struct {
	Service *SyncService
	User    func(*http.Request) string
	Owns    func(context.Context, string, int64) (bool, error)
	CanRead func(context.Context, string, int64) (bool, error)
}
type syncTargetDTO struct {
	ID             int64      `json:"id,string"`
	CharacterID    int64      `json:"character_id,string"`
	Name           string     `json:"name"`
	Resource       string     `json:"resource"`
	State          string     `json:"state"`
	Reason         string     `json:"reason"`
	Freshness      string     `json:"freshness"`
	LastAttempt    *time.Time `json:"last_attempt_at"`
	LastSuccess    *time.Time `json:"last_success_at"`
	ContentUpdated *time.Time `json:"content_updated_at"`
	NextDue        time.Time  `json:"next_due_at"`
	PendingDetails int32      `json:"pending_details"`
	FailedDetails  int32      `json:"failed_details"`
}

func optionalTime(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}
func targetDTO(t store.EveSyncTarget) syncTargetDTO {
	fresh := "never"
	if t.LastSuccessAt.Valid {
		fresh = "stale"
		if t.ValidUntil.Time.After(time.Now()) && t.State != "blocked" {
			fresh = "fresh"
		}
	}
	return syncTargetDTO{t.ID, t.CharacterID, t.DisplayName, t.Resource, t.State, t.Reason, fresh, optionalTime(t.LastAttemptAt), optionalTime(t.LastSuccessAt), optionalTime(t.ContentUpdatedAt), t.NextDueAt.Time, 0, 0}
}
func (h SyncHTTP) detailStatus(ctx context.Context, items []syncTargetDTO) error {
	for i := range items {
		if !isContracts(items[i].Resource) {
			continue
		}
		stats, e := store.New(h.Service.pool).ContractDetailStats(ctx, items[i].ID)
		if e != nil {
			return e
		}
		items[i].PendingDetails = stats.Pending
		items[i].FailedDetails = stats.Failed
	}
	return nil
}
func (h SyncHTTP) Routes() []module.Route {
	return []module.Route{
		{Method: "GET", Path: "/sync/rate-limits", Permission: "eve.sync.manage", Handler: http.HandlerFunc(h.rateLimits)},
		{Method: "GET", Path: "/sync/rate-limits/{id}/routes", Permission: "eve.sync.manage", Handler: http.HandlerFunc(h.rateRoutes)},
		{Method: "GET", Path: "/sync/tokens", Permission: "eve.sync.manage", Handler: http.HandlerFunc(h.tokens)},
		{Method: "GET", Path: "/sync/tokens/{id}/events", Permission: "eve.sync.manage", Handler: http.HandlerFunc(h.tokenEvents)},
		{Method: "GET", Path: "/sync/characters/{id}", Permission: "eve.sync.self", Handler: http.HandlerFunc(h.character)},
		{Method: "POST", Path: "/sync/characters/{id}/refresh", Permission: "eve.sync.self", Handler: http.HandlerFunc(h.refresh)},
		{Method: "GET", Path: "/sync/targets", Permission: "eve.sync.manage", Handler: http.HandlerFunc(h.targets)},
		{Method: "GET", Path: "/sync/targets/{id}/runs", Permission: "eve.sync.manage", Handler: http.HandlerFunc(h.runs)},
		{Method: "POST", Path: "/sync/targets/{id}/retry", Permission: "eve.sync.manage", Handler: http.HandlerFunc(h.refresh)},
	}
}
func (h SyncHTTP) fail(w http.ResponseWriter, r *http.Request) {
	httpapi.Failure(w, r, 503, "sync_unavailable", "同步服务暂不可用")
}
func parseSyncID(r *http.Request) (int64, error) {
	raw := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != raw {
		return 0, errors.New("invalid ID")
	}
	return id, nil
}
func (h SyncHTTP) owned(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := parseSyncID(r)
	if err != nil {
		httpapi.Failure(w, r, 404, "character_unavailable", "角色不可用")
		return 0, false
	}
	check := h.Owns
	if r.Method == http.MethodGet && h.CanRead != nil {
		check = h.CanRead
	}
	ok, err := check(r.Context(), h.User(r), id)
	if err != nil {
		h.fail(w, r)
		return 0, false
	}
	if !ok {
		httpapi.Failure(w, r, 404, "character_unavailable", "角色不可用")
		return 0, false
	}
	return id, true
}
func (h SyncHTTP) character(w http.ResponseWriter, r *http.Request) {
	id, ok := h.owned(w, r)
	if !ok {
		return
	}
	rows, err := store.New(h.Service.pool).ListCharacterSync(r.Context(), id)
	if err != nil {
		h.fail(w, r)
		return
	}
	items := make([]syncTargetDTO, 0, len(rows))
	for _, row := range rows {
		items = append(items, targetDTO(row))
	}
	if h.detailStatus(r.Context(), items) != nil {
		h.fail(w, r)
		return
	}
	httpapi.Respond(w, r, 200, map[string]any{"targets": items, "available": h.Service.available.Load()})
}
func (h SyncHTTP) targets(w http.ResponseWriter, r *http.Request) {
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
	case "", "idle", "queued", "running", "deferred", "failed", "blocked":
	default:
		httpapi.Failure(w, r, 400, "invalid_filter", "筛选条件无效")
		return
	}
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	if len([]rune(search)) > 80 {
		httpapi.Failure(w, r, 400, "invalid_filter", "搜索内容过长")
		return
	}
	rows, err := store.New(h.Service.pool).ListSyncTargets(r.Context(), store.ListSyncTargetsParams{AfterID: after, StateFilter: filter, Search: search})
	if err != nil {
		h.fail(w, r)
		return
	}
	next := ""
	if len(rows) > 30 {
		rows = rows[:30]
		next = strconv.FormatInt(rows[29].ID, 10)
	}
	items := make([]syncTargetDTO, 0, len(rows))
	for _, row := range rows {
		items = append(items, targetDTO(row))
	}
	if h.detailStatus(r.Context(), items) != nil {
		h.fail(w, r)
		return
	}
	httpapi.Respond(w, r, 200, map[string]any{"targets": items, "next_cursor": next, "available": h.Service.available.Load()})
}
func (h SyncHTTP) runs(w http.ResponseWriter, r *http.Request) {
	id, err := parseSyncID(r)
	if err != nil {
		httpapi.Failure(w, r, 404, "target_unavailable", "同步目标不可用")
		return
	}
	q := store.New(h.Service.pool)
	if _, err = q.GetSyncTarget(r.Context(), id); errors.Is(err, pgx.ErrNoRows) {
		httpapi.Failure(w, r, 404, "target_unavailable", "同步目标不可用")
		return
	} else if err != nil {
		h.fail(w, r)
		return
	}
	rows, err := q.ListSyncRuns(r.Context(), id)
	if err != nil {
		h.fail(w, r)
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, map[string]any{"id": strconv.FormatInt(row.ID, 10), "started_at": row.StartedAt.Time, "finished_at": optionalTime(row.FinishedAt), "outcome": row.Outcome, "reason": row.Reason, "http_status": row.HttpStatus})
	}
	httpapi.Respond(w, r, 200, map[string]any{"runs": items})
}
func (h SyncHTTP) refresh(w http.ResponseWriter, r *http.Request) {
	management := strings.HasSuffix(r.URL.Path, "/retry")
	var id int64
	if management {
		var err error
		id, err = parseSyncID(r)
		if err != nil {
			httpapi.Failure(w, r, 404, "target_unavailable", "同步目标不可用")
			return
		}
	} else {
		var ok bool
		id, ok = h.owned(w, r)
		if !ok {
			return
		}
	}
	if !h.Service.available.Load() {
		h.fail(w, r)
		return
	}
	var input struct {
		Resource string `json:"resource"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil && !errors.Is(err, io.EOF) {
		httpapi.Failure(w, r, 400, "invalid_request", "请求内容无效")
		return
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		httpapi.Failure(w, r, 400, "invalid_request", "请求内容无效")
		return
	}
	if input.Resource != "" && input.Resource != "profile" && input.Resource != "authorization" && input.Resource != "fittings" && input.Resource != "skills" && input.Resource != "skillqueue" && input.Resource != "killmails" && input.Resource != "online" && !isContracts(input.Resource) && !isWallet(input.Resource) {
		httpapi.Failure(w, r, 400, "invalid_resource", "数据项无效")
		return
	}
	tx, err := h.Service.pool.Begin(r.Context())
	if err != nil {
		h.fail(w, r)
		return
	}
	defer tx.Rollback(context.Background())
	q := store.New(tx)
	var rows []store.EveSyncTarget
	if management {
		row, e := q.GetSyncTarget(r.Context(), id)
		err = e
		if err == nil {
			rows = []store.EveSyncTarget{row}
		}
	} else {
		rows, err = q.ListCharacterSync(r.Context(), id)
	}
	if errors.Is(err, pgx.ErrNoRows) || len(rows) == 0 && err == nil {
		httpapi.Failure(w, r, 404, "target_unavailable", "同步目标不可用")
		return
	}
	if err != nil {
		h.fail(w, r)
		return
	}
	results := make([]map[string]any, 0, len(rows))
	var user pgtype.UUID
	if user.Scan(h.User(r)) != nil {
		h.fail(w, r)
		return
	}
	for _, row := range rows {
		if input.Resource != "" && row.Resource != input.Resource {
			continue
		}
		target, err := q.LockSyncTarget(r.Context(), row.ID)
		if err != nil {
			h.fail(w, r)
			return
		}
		if target.State == "blocked" {
			httpapi.Failure(w, r, 409, "sync_blocked", syncReason(target.Reason))
			return
		}
		outcome, err := h.Service.enqueue(r.Context(), tx, target)
		if err != nil {
			h.fail(w, r)
			return
		}
		action := "self_refresh"
		if management {
			action = "admin_retry"
		}
		if err = q.SyncAudit(r.Context(), store.SyncAuditParams{UserID: user, CharacterID: target.CharacterID, TargetID: target.ID, Action: action, Outcome: outcome}); err != nil {
			h.fail(w, r)
			return
		}
		results = append(results, map[string]any{"target_id": strconv.FormatInt(target.ID, 10), "outcome": outcome, "next_due_at": target.NextDueAt.Time})
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.fail(w, r)
		return
	}
	httpapi.Respond(w, r, 202, map[string]any{"results": results})
}
func syncReason(reason string) string {
	switch reason {
	case "reauthorize", "access_token_rejected", "missing_scope":
		return "请更新 EVE 授权"
	case "access_denied", "missing_role":
		return "ESI 拒绝访问，请检查游戏职务或更新授权"
	case "identity_changed":
		return "角色身份已变化，请重新登录"
	default:
		return "同步需要处理，请联系管理员"
	}
}

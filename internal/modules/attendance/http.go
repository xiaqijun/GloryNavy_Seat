package attendance

import (
	"encoding/json"
	"errors"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/module"
	"glorynavy.local/seat/internal/modules/attendance/internal/store"
	"io"
	"net/http"
	"strconv"
)

type Handler struct {
	Service *Service
	User    func(*http.Request) string
}

func (h Handler) Module() module.Definition {
	routes := []module.Route{}
	for _, r := range []struct {
		method, path string
		handler      http.HandlerFunc
	}{
		{"GET", "/context", h.context}, {"GET", "/events", h.list}, {"POST", "/events", h.create},
		{"GET", "/events/{id}", h.detail}, {"POST", "/events/{id}/capture", h.change("capture")},
		{"GET", "/events/{id}/characters/{character_id}/battle", h.battle},
		{"POST", "/events/{id}/characters/{character_id}/battle/refresh", h.refreshBattle},
		{"POST", "/events/{id}/losses/{loss_id}/review", h.reviewLoss},
		{"GET", "/pap", h.papReport}, {"GET", "/pap/pending", h.pendingPAP}, {"GET", "/events/{id}/pap", h.papHistory}, {"POST", "/events/{id}/pap", h.setPAP},
		{"GET", "/alliance-pap", h.alliancePAP},
		{"GET", "/alliance-pap/summary", h.alliancePAPSummary},
		{"GET", "/alliance-pap/members", h.alliancePAPMembers},
		{"GET", "/alliance-pap/conversions", h.allianceConversions},
		{"GET", "/alliance-pap/conversion", h.allianceConversion}, {"POST", "/alliance-pap/conversion", h.allianceConversion},
		{"GET", "/pap-requirement", h.papRequirement}, {"POST", "/pap-requirement", h.papRequirement},
		{"GET", "/events/{id}/conversion", h.conversion}, {"POST", "/events/{id}/conversion", h.conversion},
		{"POST", "/events/{id}/manual", h.change("manual")}, {"POST", "/events/{id}/close", h.change("close")},
		{"POST", "/events/{id}/reopen", h.change("reopen")}, {"GET", "/events/{id}/audit", h.audit}, {"GET", "/online", h.online},
	} {
		routes = append(routes, module.Route{Method: r.method, Path: r.path, Permission: "attendance.self", Handler: r.handler})
	}
	return module.Definition{Manifest: module.Manifest{ID: "attendance", Version: "0.1.0", APIVersion: 1, Requires: []module.Dependency{{ID: "identity", APIVersion: 1}, {ID: "eve", APIVersion: 1}, {ID: "access", APIVersion: 1}}}, Permissions: []string{"attendance.self"}, Routes: routes}
}

func (h Handler) alliancePAP(w http.ResponseWriter, r *http.Request) {
	out, err := h.Service.AlliancePAPReport(r.Context(), h.User(r))
	respond(w, r, out, err)
}
func (h Handler) alliancePAPSummary(w http.ResponseWriter, r *http.Request) {
	out, err := h.Service.AlliancePAPFulfillmentReport(r.Context(), h.User(r))
	respond(w, r, out, err)
}
func (h Handler) alliancePAPMembers(w http.ResponseWriter, r *http.Request) {
	out, err := h.Service.AlliancePAPMembersReport(r.Context(), h.User(r), r.URL.Query().Get("month"))
	respond(w, r, out, err)
}
func (h Handler) allianceConversions(w http.ResponseWriter, r *http.Request) {
	out, err := h.Service.AlliancePAPConversionMonths(r.Context(), h.User(r))
	respond(w, r, map[string]any{"months": out}, err)
}
func (h Handler) allianceConversion(w http.ResponseWriter, r *http.Request) {
	var c *PAPConversion
	if r.Method == "POST" {
		c = new(PAPConversion)
		if !readBody(w, r, c) {
			return
		}
	}
	month := ""
	if r.Method == "GET" {
		month = r.URL.Query().Get("month")
	}
	out, err := h.Service.ConvertAlliancePAP(r.Context(), h.User(r), c, month)
	if c != nil {
		respond(w, r, map[string]bool{"saved": err == nil}, err)
		return
	}
	respond(w, r, out, err)
}
func (h Handler) battle(w http.ResponseWriter, r *http.Request) {
	id, err := number(chi.URLParam(r, "id"))
	char, e := number(chi.URLParam(r, "character_id"))
	if err != nil || e != nil {
		respond(w, r, nil, ErrInvalid)
		return
	}
	data, err := h.Service.BattleDetail(r.Context(), h.User(r), id, char)
	respond(w, r, data, err)
}
func (h Handler) reviewLoss(w http.ResponseWriter, r *http.Request) {
	id, err := number(chi.URLParam(r, "id"))
	loss, e := number(chi.URLParam(r, "loss_id"))
	if err != nil || e != nil {
		respond(w, r, nil, ErrInvalid)
		return
	}
	var c LossReview
	if !readBody(w, r, &c) {
		return
	}
	err = h.Service.ReviewLoss(r.Context(), h.User(r), id, loss, c)
	respond(w, r, map[string]bool{"saved": err == nil}, err)
}
func respond(w http.ResponseWriter, r *http.Request, data any, err error) {
	switch {
	case errors.Is(err, ErrCoinRateRequired):
		httpapi.Failure(w, r, 409, "coin_rate_required", "请先由管理员在兑换中心配置 PAP 发币比例")
	case errors.Is(err, pgx.ErrNoRows):
		httpapi.Failure(w, r, 404, "attendance_unavailable", "记录不存在或无权访问")
	case errors.Is(err, ErrInvalid):
		httpapi.Failure(w, r, 400, "invalid_attendance", "请检查军团、角色、时间及必填内容")
	case errors.Is(err, ErrConflict):
		httpapi.Failure(w, r, 409, "attendance_conflict", "记录已更新，请刷新后重试")
	case errors.Is(err, ErrUnavailable):
		httpapi.Failure(w, r, 422, "fleet_unavailable", "请确认角色已授权、身处舰队且有名单访问权限，稍后重试")
	case err != nil:
		httpapi.Failure(w, r, 503, "attendance_unavailable", "考勤服务暂不可用")
	default:
		httpapi.Respond(w, r, 200, data)
	}
}
func readBody(w http.ResponseWriter, r *http.Request, v any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil || d.Decode(new(any)) != io.EOF {
		respond(w, r, nil, ErrInvalid)
		return false
	}
	return true
}
func number(raw string) (int64, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != raw {
		return 0, ErrInvalid
	}
	return id, nil
}
func (h Handler) context(w http.ResponseWriter, r *http.Request) {
	corps, err := h.Service.Corporations(r.Context(), h.User(r))
	if err != nil {
		respond(w, r, nil, err)
		return
	}
	own, err := h.Service.Own(r.Context(), h.User(r))
	chars := []map[string]any{}
	for _, c := range own {
		chars = append(chars, map[string]any{"id": strconv.FormatInt(c.ID, 10), "name": c.Name})
	}
	respond(w, r, map[string]any{"corporations": corps, "characters": chars}, err)
}
func (h Handler) list(w http.ResponseWriter, r *http.Request) {
	var before int64
	var err error
	if raw := r.URL.Query().Get("before"); raw != "" {
		before, err = number(raw)
	}
	if err != nil {
		respond(w, r, nil, err)
		return
	}
	data, err := h.Service.List(r.Context(), h.User(r), before)
	respond(w, r, data, err)
}
func (h Handler) create(w http.ResponseWriter, r *http.Request) {
	var c Create
	if !readBody(w, r, &c) {
		return
	}
	data, err := h.Service.Create(r.Context(), h.User(r), c)
	respond(w, r, data, err)
}
func (h Handler) detail(w http.ResponseWriter, r *http.Request) {
	id, err := number(chi.URLParam(r, "id"))
	if err != nil {
		respond(w, r, nil, err)
		return
	}
	data, err := h.Service.Detail(r.Context(), h.User(r), id)
	respond(w, r, data, err)
}
func (h Handler) change(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := number(chi.URLParam(r, "id"))
		if err != nil {
			respond(w, r, nil, err)
			return
		}
		var c Change
		if !readBody(w, r, &c) {
			return
		}
		data, err := h.Service.Change(r.Context(), h.User(r), id, action, c)
		respond(w, r, data, err)
	}
}
func (h Handler) online(w http.ResponseWriter, r *http.Request) {
	days := 7
	var corp int64
	var err error
	q := r.URL.Query()
	if raw := q.Get("days"); raw != "" {
		days, err = strconv.Atoi(raw)
	}
	if err == nil && q.Get("corporation_id") != "" {
		corp, err = number(q.Get("corporation_id"))
	}
	if err != nil {
		respond(w, r, nil, ErrInvalid)
		return
	}
	data, err := h.Service.Report(r.Context(), h.User(r), q.Get("member"), corp, days)
	respond(w, r, data, err)
}
func (h Handler) audit(w http.ResponseWriter, r *http.Request) {
	id, err := number(chi.URLParam(r, "id"))
	if err != nil {
		respond(w, r, nil, err)
		return
	}
	var after int64
	if raw := r.URL.Query().Get("after"); raw != "" {
		after, err = number(raw)
	}
	if err != nil {
		respond(w, r, nil, err)
		return
	}
	ev, err := store.New(h.Service.Pool).GetEvent(r.Context(), id)
	if err == nil {
		err = h.Service.require(r.Context(), h.User(r), ev.CorporationID)
	}
	if err != nil {
		respond(w, r, nil, err)
		return
	}
	rows, err := store.New(h.Service.Pool).ListAudit(r.Context(), store.ListAuditParams{EventID: id, AfterID: after})
	next := ""
	if len(rows) > 50 {
		rows = rows[:50]
		next = strconv.FormatInt(rows[49].ID, 10)
	}
	items := []map[string]any{}
	for _, a := range rows {
		var p auditPayload
		if json.Unmarshal(a.Payload, &p) != nil {
			respond(w, r, nil, ErrUnavailable)
			return
		}
		items = append(items, map[string]any{"id": strconv.FormatInt(a.ID, 10), "actor_id": a.ActorID.String(), "action": a.Action, "reason": p.Reason, "pap_points": p.PAPPoints, "pap_delta": p.PAPDelta, "pap_revoked": p.PAPRevoked, "recorded": p.Recorded, "excluded": p.Excluded, "excluded_external": p.ExcludedExternal, "excluded_unbound": p.ExcludedUnbound, "observed_at": p.ObservedAt, "created_at": a.CreatedAt.Time, "character_id": strconv.FormatInt(p.CharacterID, 10), "present": p.Present, "loss_id": strconv.FormatInt(p.LossID, 10), "loss_state": p.LossState})
	}
	respond(w, r, map[string]any{"items": items, "next_cursor": next}, err)
}

func (h Handler) refreshBattle(w http.ResponseWriter, r *http.Request) {
	id, err := number(chi.URLParam(r, "id"))
	char, e := number(chi.URLParam(r, "character_id"))
	if err != nil || e != nil {
		respond(w, r, nil, ErrInvalid)
		return
	}
	var c Change
	if !readBody(w, r, &c) {
		return
	}
	err = h.Service.RefreshBattle(r.Context(), h.User(r), id, char, c)
	respond(w, r, map[string]bool{"saved": err == nil}, err)
}

func (h Handler) setPAP(w http.ResponseWriter, r *http.Request) {
	id, err := number(chi.URLParam(r, "id"))
	if err != nil {
		respond(w, r, nil, err)
		return
	}
	var c PAPChange
	if !readBody(w, r, &c) {
		return
	}
	err = h.Service.SetPAP(r.Context(), h.User(r), id, c)
	respond(w, r, map[string]bool{"saved": err == nil}, err)
}
func (h Handler) papHistory(w http.ResponseWriter, r *http.Request) {
	id, err := number(chi.URLParam(r, "id"))
	var after int64
	if raw := r.URL.Query().Get("after"); raw != "" && err == nil {
		after, err = number(raw)
	}
	if err != nil {
		respond(w, r, nil, err)
		return
	}
	out, err := h.Service.PAPHistory(r.Context(), h.User(r), id, after)
	respond(w, r, out, err)
}
func (h Handler) papReport(w http.ResponseWriter, r *http.Request) {
	var corp int64
	var err error
	if raw := r.URL.Query().Get("corporation_id"); raw != "" {
		corp, err = number(raw)
	}
	page := int64(0)
	if raw := r.URL.Query().Get("page"); raw != "" && err == nil {
		page, err = strconv.ParseInt(raw, 10, 32)
	}
	if err != nil {
		respond(w, r, nil, ErrInvalid)
		return
	}
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "month"
	}
	out, err := h.Service.PAPReport(r.Context(), h.User(r), corp, period, int32(page))
	respond(w, r, out, err)
}

func (h Handler) pendingPAP(w http.ResponseWriter, r *http.Request) {
	out, err := h.Service.PendingPAP(r.Context(), h.User(r))
	respond(w, r, out, err)
}

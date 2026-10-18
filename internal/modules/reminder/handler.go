package reminder

import (
	"encoding/json"
	"errors"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/module"
	"io"
	"net/http"
)

type Handler struct {
	Service *Service
	User    func(*http.Request) string
}

func (h Handler) Module() module.Definition {
	return module.Definition{Manifest: module.Manifest{ID: "reminder", Version: "0.1.0", APIVersion: 1, Requires: []module.Dependency{{ID: "identity", APIVersion: 1}, {ID: "access", APIVersion: 1}, {ID: "eve", APIVersion: 1}, {ID: "approval", APIVersion: 1}}}, Permissions: []string{"reminder.manage"}, Routes: []module.Route{
		{Method: "GET", Path: "/settings", Permission: "reminder.manage", Handler: http.HandlerFunc(h.settings)},
		{Method: "PUT", Path: "/settings", Permission: "reminder.manage", Handler: http.HandlerFunc(h.save)},
		{Method: "GET", Path: "/deliveries", Permission: "reminder.manage", Handler: http.HandlerFunc(h.deliveries)},
		{Method: "POST", Path: "/deliveries/{id}/retry", Permission: "reminder.manage", Handler: http.HandlerFunc(h.retry)},
	}}
}
func respond(w http.ResponseWriter, r *http.Request, v any, err error) {
	if err == nil {
		httpapi.Respond(w, r, 200, v)
		return
	}
	switch {
	case errors.Is(err, ErrEvents):
		httpapi.Failure(w, r, 400, "invalid_events", "请选择有效的提醒类型")
	case errors.Is(err, ErrForbidden):
		httpapi.Failure(w, r, 403, "forbidden", "无权管理提醒")
	case errors.Is(err, ErrVersion):
		httpapi.Failure(w, r, 409, "version_conflict", "配置已更新，请刷新后重试")
	case errors.Is(err, ErrSender):
		httpapi.Failure(w, r, 409, "sender_unavailable", "请选择本人角色并授权发信")
	case errors.Is(err, pgx.ErrNoRows):
		httpapi.Failure(w, r, 409, "retry_unavailable", "此记录不可重试")
	default:
		httpapi.Failure(w, r, 503, "reminder_unavailable", "提醒服务暂不可用")
	}
}
func (h Handler) settings(w http.ResponseWriter, r *http.Request) {
	v, e := h.Service.Configuration(r.Context(), h.User(r))
	respond(w, r, v, e)
}
func (h Handler) save(w http.ResponseWriter, r *http.Request) {
	var in SaveInput
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	d.DisallowUnknownFields()
	if d.Decode(&in) != nil || d.Decode(&struct{}{}) != io.EOF {
		httpapi.Failure(w, r, 400, "invalid_request", "配置无效")
		return
	}
	v, e := h.Service.Save(r.Context(), h.User(r), in)
	respond(w, r, v, e)
}
func validID(s string) bool { var id pgtype.UUID; return s != "" && id.Scan(s) == nil && id.Valid }
func (h Handler) deliveries(w http.ResponseWriter, r *http.Request) {
	before := r.URL.Query().Get("before")
	if before != "" && !validID(before) {
		httpapi.Failure(w, r, 400, "invalid_request", "记录编号无效")
		return
	}
	v, e := h.Service.Deliveries(r.Context(), h.User(r), before)
	respond(w, r, v, e)
}
func (h Handler) retry(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !validID(id) {
		httpapi.Failure(w, r, 400, "invalid_request", "记录编号无效")
		return
	}
	e := h.Service.Retry(r.Context(), h.User(r), id)
	respond(w, r, map[string]bool{"queued": e == nil}, e)
}

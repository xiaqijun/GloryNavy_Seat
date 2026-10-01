package approval

import (
	"errors"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/module"
	"glorynavy.local/seat/internal/platform/reviewqueue"
	"net/http"
)

type Handler struct {
	Service *Service
	User    func(*http.Request) string
}

func (h Handler) Module() module.Definition {
	return module.Definition{Manifest: module.Manifest{ID: "approval", Version: "0.1.0", APIVersion: 1, Requires: []module.Dependency{{ID: "identity", APIVersion: 1}, {ID: "access", APIVersion: 1}}}, Permissions: []string{"approval.self"}, Routes: []module.Route{
		{Method: "GET", Path: "/context", Permission: "approval.self", Handler: http.HandlerFunc(h.context)},
		{Method: "GET", Path: "/items", Permission: "approval.self", Handler: http.HandlerFunc(h.list)},
		{Method: "GET", Path: "/items/{source}/{id}", Permission: "approval.self", Handler: http.HandlerFunc(h.detail)},
	}}
}
func respond(w http.ResponseWriter, r *http.Request, v any, e error) {
	if e == nil {
		httpapi.Respond(w, r, 200, v)
	} else if errors.Is(e, ErrInvalid) {
		httpapi.Failure(w, r, 400, "invalid_approval_query", "筛选或分页已失效，请重新查询")
	} else if errors.Is(e, pgx.ErrNoRows) {
		httpapi.Failure(w, r, 404, "approval_unavailable", "记录不存在或无权访问")
	} else {
		httpapi.Failure(w, r, 503, "approval_unavailable", "审批中心暂不可用，请重试")
	}
}
func (h Handler) context(w http.ResponseWriter, r *http.Request) {
	v, e := h.Service.Context(r.Context(), h.User(r))
	respond(w, r, v, e)
}
func (h Handler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	view := q.Get("view")
	if view == "" {
		view = "pending"
	}
	v, e := h.Service.List(r.Context(), h.User(r), reviewqueue.Filter{View: view, Sort: q.Get("sort"), Kind: q.Get("kind"), Corporation: q.Get("corporation"), Account: q.Get("account"), Search: q.Get("q"), Status: q.Get("status"), From: q.Get("from"), Until: q.Get("until"), Mine: q.Get("mine") == "true"}, q.Get("cursor"))
	respond(w, r, v, e)
}
func (h Handler) detail(w http.ResponseWriter, r *http.Request) {
	v, e := h.Service.Detail(r.Context(), h.User(r), chi.URLParam(r, "source"), chi.URLParam(r, "id"))
	respond(w, r, v, e)
}

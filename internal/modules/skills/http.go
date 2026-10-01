package skills

import (
	"encoding/json"
	"errors"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
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
	routes := []module.Route{}
	for _, r := range []struct {
		method, path string
		fn           http.HandlerFunc
	}{{"GET", "/context", h.context}, {"GET", "/catalog", h.catalog}, {"GET", "/characters/{id}", h.character}, {"GET", "/plans", h.plans}, {"POST", "/plans", h.save}, {"PUT", "/plans/{id}", h.save}, {"DELETE", "/plans/{id}", h.save}, {"GET", "/plans/{id}/check", h.check}} {
		routes = append(routes, module.Route{Method: r.method, Path: r.path, Permission: "skills.self", Handler: r.fn})
	}
	return module.Definition{Manifest: module.Manifest{ID: "skills", Version: "0.1.0", APIVersion: 1, Requires: []module.Dependency{{ID: "identity", APIVersion: 1}, {ID: "eve", APIVersion: 1}, {ID: "access", APIVersion: 1}}}, Permissions: []string{"skills.self"}, Routes: routes}
}
func respond(w http.ResponseWriter, r *http.Request, v any, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		httpapi.Failure(w, r, 404, "skills_unavailable", "记录不存在或无权访问")
	case errors.Is(err, ErrInvalid):
		httpapi.Failure(w, r, 400, "invalid_skills", "请检查技能、等级和方案名称")
	case errors.Is(err, ErrConflict):
		httpapi.Failure(w, r, 409, "skills_conflict", "方案已变更或已达数量上限，请刷新后重试")
	case err != nil:
		httpapi.Failure(w, r, 503, "skills_unavailable", "技能服务暂不可用")
	default:
		httpapi.Respond(w, r, 200, v)
	}
}
func (h Handler) context(w http.ResponseWriter, r *http.Request) {
	if member := r.URL.Query().Get("member"); member != "" && !validUUID(member) {
		respond(w, r, nil, ErrInvalid)
		return
	}
	chars, err := h.Service.Characters(r.Context(), h.User(r), r.URL.Query().Get("member"))
	if err != nil {
		respond(w, r, nil, err)
		return
	}
	corps, err := h.Service.Corporations(r.Context(), h.User(r))
	respond(w, r, map[string]any{"characters": chars, "corporations": corps}, err)
}
func (h Handler) catalog(w http.ResponseWriter, r *http.Request) {
	v, e := h.Service.Types(r.Context())
	respond(w, r, map[string]any{"build": catalogBuild, "items": v}, e)
}
func (h Handler) character(w http.ResponseWriter, r *http.Request) {
	id, e := number(chi.URLParam(r, "id"))
	if e != nil {
		respond(w, r, nil, e)
		return
	}
	v, e := h.Service.Read(r.Context(), h.User(r), id)
	respond(w, r, v, e)
}
func (h Handler) plans(w http.ResponseWriter, r *http.Request) {
	id, e := number(r.URL.Query().Get("corporation_id"))
	if e != nil {
		respond(w, r, nil, e)
		return
	}
	v, e := h.Service.Plans(r.Context(), h.User(r), id)
	respond(w, r, v, e)
}
func (h Handler) save(w http.ResponseWriter, r *http.Request) {
	var id int64
	var err error
	if r.Method != "POST" {
		id, err = number(chi.URLParam(r, "id"))
		if err != nil {
			respond(w, r, nil, err)
			return
		}
	}
	var c Edit
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	d.DisallowUnknownFields()
	if d.Decode(&c) != nil || d.Decode(new(any)) != io.EOF {
		respond(w, r, nil, ErrInvalid)
		return
	}
	v, e := h.Service.Save(r.Context(), h.User(r), id, c, r.Method == "DELETE")
	respond(w, r, v, e)
}
func (h Handler) check(w http.ResponseWriter, r *http.Request) {
	id, e := number(chi.URLParam(r, "id"))
	if e != nil {
		respond(w, r, nil, e)
		return
	}
	scope := r.URL.Query().Get("scope")
	member := r.URL.Query().Get("member")
	if member != "" && !validUUID(member) {
		respond(w, r, nil, ErrInvalid)
		return
	}
	var after int64
	if raw := r.URL.Query().Get("after"); raw != "" {
		after, e = number(raw)
		if e != nil {
			respond(w, r, nil, e)
			return
		}
	}
	if (scope != "" && scope != "corporation") || (scope == "corporation" && member != "") {
		respond(w, r, nil, ErrInvalid)
		return
	}
	v, e := h.Service.CheckPlan(r.Context(), h.User(r), id, member, scope == "corporation", after)
	respond(w, r, v, e)
}

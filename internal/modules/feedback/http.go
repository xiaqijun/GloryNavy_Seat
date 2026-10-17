package feedback

import (
	"encoding/json"
	"errors"
	"github.com/go-chi/chi/v5"
	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/module"
	"io"
	"net/http"
)

type Handler struct {
	Service *Service
	Actor   func(*http.Request) Actor
}

func (h Handler) Module() module.Definition {
	return module.Definition{Manifest: module.Manifest{ID: "feedback", Version: "0.1.0", APIVersion: 1, Requires: []module.Dependency{{ID: "identity", APIVersion: 1}, {ID: "access", APIVersion: 1}}}, Permissions: []string{"feedback.self"}, Routes: []module.Route{
		{Method: "GET", Path: "/tickets", Permission: "feedback.self", Handler: http.HandlerFunc(h.list)},
		{Method: "POST", Path: "/tickets", Permission: "feedback.self", Handler: http.HandlerFunc(h.create)},
		{Method: "GET", Path: "/tickets/{id}", Permission: "feedback.self", Handler: http.HandlerFunc(h.detail)},
		{Method: "POST", Path: "/tickets/{id}/messages", Permission: "feedback.self", Handler: http.HandlerFunc(h.reply)},
	}}
}
func respond(w http.ResponseWriter, r *http.Request, v any, err error) {
	if err == nil {
		httpapi.Respond(w, r, 200, v)
		return
	}
	var f *Fault
	if errors.As(err, &f) {
		httpapi.Failure(w, r, f.Status, f.Code, f.Message)
		return
	}
	httpapi.Failure(w, r, 503, "feedback_unavailable", "反馈暂不可用，请稍后重试")
}
func read(w http.ResponseWriter, r *http.Request, v any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32768))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil || d.Decode(&struct{}{}) != io.EOF {
		respond(w, r, nil, invalid)
		return false
	}
	return true
}
func (h Handler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	scope := q.Get("scope")
	if scope != "" && scope != "mine" && scope != "all" {
		respond(w, r, nil, invalid)
		return
	}
	var before int64
	if q.Get("before") != "" {
		var err error
		before, err = parseID(q.Get("before"))
		if err != nil {
			respond(w, r, nil, err)
			return
		}
	}
	v, err := h.Service.List(r.Context(), h.Actor(r).ID, scope == "all", q.Get("kind"), q.Get("status"), before)
	respond(w, r, v, err)
}
func (h Handler) detail(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		respond(w, r, nil, err)
		return
	}
	v, err := h.Service.Detail(r.Context(), h.Actor(r).ID, id)
	respond(w, r, v, err)
}
func (h Handler) create(w http.ResponseWriter, r *http.Request) {
	var in Submission
	if !read(w, r, &in) {
		return
	}
	v, err := h.Service.Create(r.Context(), h.Actor(r), in)
	respond(w, r, v, err)
}
func (h Handler) reply(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		respond(w, r, nil, err)
		return
	}
	var in Reply
	if !read(w, r, &in) {
		return
	}
	v, err := h.Service.Reply(r.Context(), h.Actor(r), id, in)
	respond(w, r, v, err)
}

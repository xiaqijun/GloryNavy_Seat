package fittings

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/module"
	"glorynavy.local/seat/internal/modules/eve"
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
	}{{"GET", "/library/context", h.libraryContext}, {"GET", "/library", h.libraryList}, {"POST", "/library", h.librarySave}, {"GET", "/library/{id}", h.libraryRead}, {"PUT", "/library/{id}", h.librarySave}, {"DELETE", "/library/{id}", h.librarySave}, {"GET", "/library/{id}/requirements", h.libraryRequirements}, {"POST", "/library/{id}/save-to-game", h.libraryGameSave}, {"GET", "/context", h.context}, {"GET", "/saved", h.saved}, {"GET", "/types", h.types}, {"GET", "/names", h.names}, {"GET", "/drafts", h.list}, {"POST", "/drafts", h.save}, {"GET", "/drafts/{id}", h.read}, {"PUT", "/drafts/{id}", h.save}, {"DELETE", "/drafts/{id}", h.remove}} {
		routes = append(routes, module.Route{Method: r.method, Path: r.path, Permission: "fittings.self", Handler: r.fn})
	}
	return module.Definition{Manifest: module.Manifest{ID: "fittings", Version: "0.1.0", APIVersion: 1, Requires: []module.Dependency{{ID: "identity", APIVersion: 1}, {ID: "eve", APIVersion: 1}, {ID: "access", APIVersion: 1}}}, Permissions: []string{"fittings.self"}, Routes: routes}
}
func reply(w http.ResponseWriter, r *http.Request, v any, err error) {
	var importErr ImportError
	switch {
	case errors.As(err, &importErr):
		httpapi.Failure(w, r, 400, "invalid_fitting", importErr.Message)
	case err == nil:
		httpapi.Respond(w, r, 200, v)
	case errors.Is(err, ErrInvalid):
		httpapi.Failure(w, r, 400, "invalid_fitting", "请检查舰船、槽位和配装内容")
	case errors.Is(err, ErrConflict):
		httpapi.Failure(w, r, 409, "fitting_conflict", "方案已变更，请刷新后重试")
	case errors.Is(err, pgx.ErrNoRows):
		httpapi.Failure(w, r, 404, "fitting_unavailable", "方案不存在或无权访问")
	default:
		httpapi.Failure(w, r, 503, "fitting_unavailable", "配装服务暂不可用")
	}
}
func body(w http.ResponseWriter, r *http.Request, v any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil || d.Decode(new(any)) != io.EOF {
		reply(w, r, nil, ErrInvalid)
		return false
	}
	return true
}
func number(v string) (int64, error) {
	n, e := strconv.ParseInt(v, 10, 64)
	if e != nil || n <= 0 || strconv.FormatInt(n, 10) != v {
		return 0, ErrInvalid
	}
	return n, nil
}
func (h Handler) context(w http.ResponseWriter, r *http.Request) {
	user := h.User(r)
	subject, err := h.Service.Subject(r.Context(), user, r.URL.Query().Get("member"))
	if err != nil {
		reply(w, r, nil, err)
		return
	}
	chars, err := h.Service.Own(r.Context(), subject)
	if chars == nil {
		chars = []Character{}
	}
	reply(w, r, map[string]any{"characters": chars, "can_edit": subject == user}, err)
}
func (h Handler) list(w http.ResponseWriter, r *http.Request) {
	var before int64
	var err error
	if raw := r.URL.Query().Get("before"); raw != "" {
		before, err = number(raw)
	}
	if err != nil {
		reply(w, r, nil, err)
		return
	}
	rows, next, err := h.Service.List(r.Context(), h.User(r), r.URL.Query().Get("member"), before)
	reply(w, r, map[string]any{"items": rows, "next_cursor": next}, err)
}
func (h Handler) read(w http.ResponseWriter, r *http.Request) {
	id, err := number(chi.URLParam(r, "id"))
	if err != nil {
		reply(w, r, nil, err)
		return
	}
	v, err := h.Service.Read(r.Context(), h.User(r), id)
	reply(w, r, v, err)
}
func (h Handler) save(w http.ResponseWriter, r *http.Request) {
	var id int64
	var err error
	if raw := chi.URLParam(r, "id"); raw != "" {
		id, err = number(raw)
	}
	if err != nil {
		reply(w, r, nil, err)
		return
	}
	var c Edit
	if !body(w, r, &c) {
		return
	}
	v, err := h.Service.Save(r.Context(), h.User(r), id, c)
	reply(w, r, v, err)
}
func (h Handler) remove(w http.ResponseWriter, r *http.Request) {
	id, err := number(chi.URLParam(r, "id"))
	if err != nil {
		reply(w, r, nil, err)
		return
	}
	var c struct {
		Version int64 `json:"version,string"`
	}
	if !body(w, r, &c) {
		return
	}
	err = h.Service.Delete(r.Context(), h.User(r), id, c.Version)
	reply(w, r, map[string]bool{"saved": err == nil}, err)
}

type namedType struct {
	ID   int64  `json:"id,string"`
	Name string `json:"name"`
}

func (h Handler) types(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(q)) < 2 || len([]rune(q)) > 80 {
		reply(w, r, nil, ErrInvalid)
		return
	}
	rows, err := h.Service.Search(r.Context(), q)
	out := []namedType{}
	for _, v := range rows {
		out = append(out, namedType{v.ID, v.Name})
	}
	reply(w, r, out, err)
}
func (h Handler) names(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Query().Get("ids"), ",")
	if len(parts) > 512 {
		reply(w, r, nil, ErrInvalid)
		return
	}
	ids := []int64{}
	for _, p := range parts {
		id, err := number(p)
		if err != nil {
			reply(w, r, nil, err)
			return
		}
		ids = append(ids, id)
	}
	rows, err := h.Service.Names.TypeNames(r.Context(), ids)
	out := []namedType{}
	for _, id := range ids {
		if n, ok := rows[id]; ok {
			out = append(out, namedType{id, n.Name})
		}
	}
	reply(w, r, out, err)
}

type savedItem struct {
	Flag     string `json:"flag"`
	Quantity int64  `json:"quantity"`
	TypeID   int64  `json:"type_id,string"`
}
type savedFit struct {
	ID          int64       `json:"id,string"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	ShipTypeID  int64       `json:"ship_type_id,string"`
	Items       []savedItem `json:"items"`
}

func (h Handler) saved(w http.ResponseWriter, r *http.Request) {
	id, err := number(r.URL.Query().Get("character_id"))
	if err != nil {
		reply(w, r, nil, err)
		return
	}
	ok, err := h.Service.CanReadCharacter(r.Context(), h.User(r), id)
	if err != nil {
		reply(w, r, nil, err)
		return
	}
	if !ok {
		reply(w, r, nil, pgx.ErrNoRows)
		return
	}
	resource := r.URL.Query().Get("resource")
	if resource == "" {
		resource = "fittings"
	}
	if resource != "fittings" && resource != "skills" {
		reply(w, r, nil, ErrInvalid)
		return
	}
	raw, at, err := h.Service.Snapshot(r.Context(), id, resource)
	if errors.Is(err, pgx.ErrNoRows) {
		reply(w, r, map[string]any{"status": "pending", "observed_at": nil, "fittings": []savedFit{}, "skills": map[string]int{}}, nil)
		return
	}
	if err != nil {
		reply(w, r, nil, err)
		return
	}
	fits := []savedFit{}
	skills := map[string]int{}
	if resource == "fittings" {
		var rows []eve.SavedFitting
		err = json.Unmarshal(raw, &rows)
		for _, f := range rows {
			items := []savedItem{}
			for _, i := range f.Items {
				items = append(items, savedItem{i.Flag, i.Quantity, i.TypeID})
			}
			fits = append(fits, savedFit{f.ID, f.Name, f.Description, f.ShipTypeID, items})
		}
	} else {
		var rows eve.TrainedSkills
		err = json.Unmarshal(raw, &rows)
		for _, s := range rows.Skills {
			skills[strconv.FormatInt(s.ID, 10)] = s.Active
		}
	}
	reply(w, r, map[string]any{"status": "ready", "observed_at": at, "fittings": fits, "skills": skills}, err)
}

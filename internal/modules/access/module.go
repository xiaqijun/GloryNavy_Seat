package access

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5/pgconn"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"

	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/module"
)

// The host supplies the verified principal; no caller-controlled user ID is accepted for self access.
type Handler struct {
	Service    *Service
	User       func(*http.Request) string
	Directory  func(context.Context, string, string) ([]Member, string, error)
	MemberData func(context.Context, string) (MemberData, error)
}

type Member struct {
	UserID         string `json:"user_id"`
	CharacterID    string `json:"character_id"`
	Name           string `json:"name"`
	CharacterCount int32  `json:"character_count"`
	Roles          []Role `json:"roles"`
	Administrator  bool   `json:"administrator"`
}

func (h Handler) Module() module.Definition {
	return module.Definition{Manifest: module.Manifest{ID: "access", Version: "0.1.0", APIVersion: 1, Requires: []module.Dependency{{ID: "identity", APIVersion: 1}, {ID: "eve", APIVersion: 1}}}, Permissions: []string{"access.self", "access.manage", "access.members.read"}, Routes: []module.Route{
		{Method: "GET", Path: "/members/{user}/data", Permission: "access.members.read", Handler: http.HandlerFunc(h.memberData)},
		{Method: "GET", Path: "/me", Permission: "access.self", Handler: http.HandlerFunc(h.current)},
		{Method: "GET", Path: "/catalog", Permission: "access.manage", Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { httpapi.Respond(w, r, 200, ManageableCatalog()) })},
		{Method: "GET", Path: "/roles", Permission: "access.manage", Handler: http.HandlerFunc(h.roles)},
		{Method: "GET", Path: "/members", Permission: "access.manage", Handler: http.HandlerFunc(h.members)},
		{Method: "GET", Path: "/audit", Permission: "access.manage", Handler: http.HandlerFunc(h.audit)},
		{Method: "PUT", Path: "/roles/{id}", Permission: "access.manage", Handler: http.HandlerFunc(h.save)},
		{Method: "DELETE", Path: "/roles/{id}", Permission: "access.manage", Handler: http.HandlerFunc(h.remove)},
		{Method: "PUT", Path: "/users/{user}/roles/{id}", Permission: "access.manage", Handler: http.HandlerFunc(h.assign)},
		{Method: "DELETE", Path: "/users/{user}/roles/{id}", Permission: "access.manage", Handler: http.HandlerFunc(h.assign)},
		{Method: "GET", Path: "/corporations/{id}/summary", Permission: "access.self", Handler: http.HandlerFunc(h.summary)},
	}}
}
func (h Handler) current(w http.ResponseWriter, r *http.Request) {
	a, err := h.Service.Account(r.Context(), h.User(r))
	if err != nil {
		httpapi.Failure(w, r, 503, "access_unavailable", "权限服务暂不可用")
		return
	}
	httpapi.Respond(w, r, 200, a)
}
func (h Handler) roles(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Service.Roles(r.Context())
	if err != nil {
		httpapi.Failure(w, r, 503, "access_unavailable", "权限服务暂不可用")
		return
	}
	httpapi.Respond(w, r, 200, rows)
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 65536)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(v) != nil || d.Decode(new(any)) != io.EOF {
		httpapi.Failure(w, r, 400, "invalid_request", "请检查提交内容")
		return false
	}
	return true
}
func outcome(w http.ResponseWriter, r *http.Request, err error) {
	if err != nil {
		if errors.Is(err, ErrConflict) {
			httpapi.Failure(w, r, 409, "role_conflict", "角色已变更，请读取最新内容后重试")
			return
		}
		var databaseError *pgconn.PgError
		if errors.Is(err, ErrInvalid) || (errors.As(err, &databaseError) && (databaseError.Code == "23505" || databaseError.Code == "23503" || databaseError.Code == "23514")) {
			httpapi.Failure(w, r, 400, "invalid_change", "保存失败，请检查角色、权限或账号是否有效")
		} else {
			httpapi.Failure(w, r, 503, "access_unavailable", "权限服务暂不可用，请稍后重试")
		}
		return
	}
	httpapi.Respond(w, r, 200, map[string]bool{"saved": true})
}
func (h Handler) save(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name    string  `json:"name"`
		Grants  []Grant `json:"grants"`
		Version *int64  `json:"version,string"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Grants == nil || body.Version == nil {
		httpapi.Failure(w, r, 400, "invalid_change", "请提供权限列表与角色版本")
		return
	}
	_, err := h.Service.SaveRole(r.Context(), h.User(r), Role{ID: r.PathValue("id"), Name: body.Name, Grants: body.Grants, Version: *body.Version})
	outcome(w, r, err)
}
func (h Handler) remove(w http.ResponseWriter, r *http.Request) {
	v, err := strconv.ParseInt(r.URL.Query().Get("version"), 10, 64)
	if err != nil || v <= 0 {
		outcome(w, r, ErrInvalid)
		return
	}
	outcome(w, r, h.Service.DeleteRole(r.Context(), h.User(r), r.PathValue("id"), v))
}

func (h Handler) members(w http.ResponseWriter, r *http.Request) {
	search := strings.TrimSpace(r.URL.Query().Get("q"))
	after := r.URL.Query().Get("after")
	if len([]rune(search)) > 200 {
		outcome(w, r, ErrInvalid)
		return
	}
	if after != "" {
		if _, err := uuid(after); err != nil {
			outcome(w, r, ErrInvalid)
			return
		}
	}
	if h.Directory == nil {
		outcome(w, r, errors.New("directory unavailable"))
		return
	}
	items, next, err := h.Directory(r.Context(), search, after)
	if err != nil {
		outcome(w, r, err)
		return
	}
	for i := range items {
		items[i].Roles, items[i].Administrator, err = h.Service.MemberRoles(r.Context(), items[i].UserID)
		if err != nil {
			outcome(w, r, err)
			return
		}
	}
	httpapi.Respond(w, r, 200, map[string]any{"items": items, "next": next})
}
func (h Handler) audit(w http.ResponseWriter, r *http.Request) {
	before := int64(math.MaxInt64)
	if raw := r.URL.Query().Get("before"); raw != "" {
		var err error
		before, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || before <= 0 {
			outcome(w, r, ErrInvalid)
			return
		}
	}
	items, next, err := h.Service.Audit(r.Context(), before)
	if err != nil {
		outcome(w, r, err)
		return
	}
	httpapi.Respond(w, r, 200, map[string]any{"items": items, "next": next})
}
func (h Handler) assign(w http.ResponseWriter, r *http.Request) {
	outcome(w, r, h.Service.Assign(r.Context(), h.User(r), r.PathValue("user"), r.PathValue("id"), r.Method == "PUT"))
}
func (h Handler) summary(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		httpapi.Failure(w, r, 400, "invalid_corporation", "军团 ID 无效")
		return
	}
	// Evaluate entity scope before returning even the public-data snapshot.
	target, err := h.Service.lookup(r.Context(), id)
	if err != nil {
		httpapi.Failure(w, r, 503, "snapshot_unavailable", "军团资料暂不可用")
		return
	}
	allowed, err := h.Service.Can(r.Context(), h.User(r), "corporation.summary", target)
	if err != nil {
		httpapi.Failure(w, r, 503, "access_unavailable", "权限服务暂不可用")
		return
	}
	if !allowed {
		httpapi.Failure(w, r, 403, "forbidden", "没有该军团的查看权限")
		return
	}
	httpapi.Respond(w, r, 200, target)
}

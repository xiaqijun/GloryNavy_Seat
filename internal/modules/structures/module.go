// Package structures exposes a read-only corporation building overview.
// Game-side ACL/profile changes remain in EVE's Structure Browser.
package structures

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/module"
	"glorynavy.local/seat/internal/modules/eve"
)

type Reader func(context.Context, string, int64) ([]eve.Structure, error)

type Handler struct {
	User func(*http.Request) string
	Read Reader
}

func (h Handler) Module() module.Definition {
	return module.Definition{
		Manifest:    module.Manifest{ID: "structures", Version: "0.1.0", APIVersion: 1, Requires: []module.Dependency{{ID: "identity", APIVersion: 1}, {ID: "eve", APIVersion: 1}, {ID: "access", APIVersion: 1}}},
		Permissions: []string{"structures.self"},
		Routes:      []module.Route{{Method: "GET", Path: "/structures", Permission: "structures.self", Handler: http.HandlerFunc(h.structures)}},
	}
}

func (h Handler) structures(w http.ResponseWriter, r *http.Request) {
	if h.Read == nil {
		failure(w, r, errors.New("structures unavailable"))
		return
	}
	corporationID := int64(0)
	if raw := r.URL.Query().Get("corporation_id"); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 || strconv.FormatInt(id, 10) != raw {
			httpapi.Failure(w, r, 400, "invalid_request", "建筑查询条件无效")
			return
		}
		corporationID = id
	}
	items, err := h.Read(r.Context(), h.User(r), corporationID)
	if err != nil {
		failure(w, r, err)
		return
	}
	if items == nil {
		items = []eve.Structure{}
	}
	httpapi.Respond(w, r, 200, map[string]any{"items": items})
}

func failure(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, pgx.ErrNoRows) {
		httpapi.Failure(w, r, 404, "structures_unavailable", "建筑不存在或没有查看权限")
		return
	}
	httpapi.Failure(w, r, 503, "structures_unavailable", "建筑暂时无法读取，请检查角色授权后重试")
}

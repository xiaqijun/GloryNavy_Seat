package identity

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/module"
	"io"
	"net/http"
	"strconv"
	"time"
)

type principalKey struct{}

func Principal(ctx context.Context) *Session { s, _ := ctx.Value(principalKey{}).(*Session); return s }

type Handler struct {
	Service     *Service
	Origin      string
	Secure      bool
	Check       func(context.Context, *Session, string, *http.Request) (bool, error)
	Cleanup     CharacterCleanup
	ProfileGate func(context.Context, string, string) (bool, error)
}

// Authorize authenticates first, enforces CSRF and delegates business permissions
// to the host. With no policy installed, unknown permissions remain denied.
func (h Handler) Authorize(permission string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		session, err := h.Service.Session(ctx, httpapi.CookieValue(r, httpapi.SessionCookie))
		if err != nil {
			httpapi.Failure(w, r, 503, "identity_unavailable", "登录服务暂不可用，请稍后重试")
			return
		}
		if session == nil {
			httpapi.Failure(w, r, 401, "unauthenticated", "请先登录")
			return
		}
		if h.ProfileGate != nil {
			complete, e := h.ProfileGate(ctx, session.UserID, permission)
			if e != nil {
				httpapi.Failure(w, r, 503, "community_unavailable", "社区资料暂不可用")
				return
			}
			if !complete {
				httpapi.Failure(w, r, 403, "profile_required", "请先补全 QQ 号与 KOOK 昵称")
				return
			}
		}
		if permission != "identity.session.read" && permission != "identity.session.logout" && permission != "identity.characters.read" && permission != "identity.characters.manage" {
			allowed := false
			if h.Check != nil {
				allowed, err = h.Check(ctx, session, permission, r)
			}
			if err != nil {
				httpapi.Failure(w, r, 503, "access_unavailable", "权限服务暂不可用")
				return
			}
			if !allowed {
				httpapi.Failure(w, r, 403, "forbidden", "没有访问权限")
				return
			}
		}
		if r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" {
			if !httpapi.SameOrigin(r, h.Origin) || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(session.CSRFToken)) != 1 {
				httpapi.Failure(w, r, 403, "csrf_failed", "操作已失效，请刷新后重试")
				return
			}
		}
		if permission != "identity.session.logout" && !h.renew(ctx, w, r, session, false) {
			return
		}
		// The short deadline bounds authentication only. Business handlers retain
		// the request deadline/cancellation and apply their own operation budgets.
		cancel()
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, session)))
	})
}

func (h Handler) current(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	session, err := h.Service.Session(ctx, httpapi.CookieValue(r, httpapi.SessionCookie))
	if err != nil {
		httpapi.Failure(w, r, 503, "identity_unavailable", "登录服务暂不可用，请稍后重试")
		return
	}
	if session == nil && httpapi.CookieValue(r, httpapi.SessionCookie) != "" {
		httpapi.SetCookie(w, httpapi.SessionCookie, "", -1, h.Secure)
	}
	if session != nil && !h.renew(ctx, w, r, session, true) {
		return
	}
	httpapi.Respond(w, r, 200, struct {
		Authenticated bool     `json:"authenticated"`
		Session       *Session `json:"session"`
	}{session != nil, session})
}

func (h Handler) renew(ctx context.Context, w http.ResponseWriter, r *http.Request, session *Session, syncCookie bool) bool {
	// Cross-site probes are not user activity. Same-origin GETs usually omit Origin.
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != h.Origin) {
		return true
	}
	token := httpapi.CookieValue(r, httpapi.SessionCookie)
	renewed, err := h.Service.RenewSession(ctx, token, session)
	if errors.Is(err, pgx.ErrNoRows) {
		httpapi.SetCookie(w, httpapi.SessionCookie, "", -1, h.Secure)
		httpapi.Failure(w, r, 401, "unauthenticated", "请先登录")
		return false
	}
	if err != nil {
		httpapi.Failure(w, r, 503, "identity_unavailable", "登录服务暂不可用，请稍后重试")
		return false
	}
	if renewed || syncCookie {
		httpapi.SetCookie(w, httpapi.SessionCookie, token, max(1, int(time.Until(session.ExpiresAt)/time.Second)), h.Secure)
	}
	return true
}
func (h Handler) logout(w http.ResponseWriter, r *http.Request) {
	if err := h.Service.Logout(r.Context(), httpapi.CookieValue(r, httpapi.SessionCookie)); err != nil {
		httpapi.Failure(w, r, 503, "identity_unavailable", "退出失败，请稍后重试")
		return
	}
	httpapi.SetCookie(w, httpapi.SessionCookie, "", -1, h.Secure)
	httpapi.SetCookie(w, httpapi.FlowCookie, "", -1, h.Secure)
	httpapi.Respond(w, r, 200, map[string]bool{"logged_out": true})
}
func (h Handler) Module() module.Definition {
	return module.Definition{
		Manifest:    module.Manifest{ID: "identity", Version: "0.1.0", APIVersion: 1},
		Permissions: []string{"identity.session.read", "identity.session.logout", "identity.characters.read", "identity.characters.manage"},
		Routes: []module.Route{
			{Method: "GET", Path: "/merges/{id}", Permission: "identity.characters.manage", Handler: http.HandlerFunc(h.merge)},
			{Method: "POST", Path: "/merges/{id}", Permission: "identity.characters.manage", Handler: http.HandlerFunc(h.merge)},
			{Method: "DELETE", Path: "/merges/{id}", Permission: "identity.characters.manage", Handler: http.HandlerFunc(h.merge)},
			{Method: "GET", Path: "/session", Public: true, Handler: http.HandlerFunc(h.current)},
			{Method: "POST", Path: "/logout", Permission: "identity.session.logout", Handler: http.HandlerFunc(h.logout)},
			{Method: "GET", Path: "/characters", Permission: "identity.characters.read", Handler: http.HandlerFunc(h.characters)},
			{Method: "POST", Path: "/characters/{id}/main", Permission: "identity.characters.manage", Handler: http.HandlerFunc(h.changeCharacter)},
			{Method: "DELETE", Path: "/characters/{id}", Permission: "identity.characters.manage", Handler: http.HandlerFunc(h.changeCharacter)},
		},
	}
}

func (h Handler) merge(w http.ResponseWriter, r *http.Request) {
	previous, id := httpapi.CookieValue(r, httpapi.SessionCookie), chi.URLParam(r, "id")
	if r.Method == "DELETE" {
		if err := h.Service.CancelMerge(r.Context(), previous, id); err != nil {
			httpapi.Failure(w, r, 409, "merge_changed", "合并验证已失效，请重新验证")
			return
		}
		httpapi.Respond(w, r, 200, map[string]bool{"cancelled": true})
		return
	}
	var body struct {
		Token string `json:"token"`
	}
	if r.Method == "POST" {
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&body) != nil || decoder.Decode(&struct{}{}) != io.EOF || len(body.Token) != 64 {
			httpapi.Failure(w, r, 400, "invalid_merge", "请先查看合并预览")
			return
		}
	}
	preview, err := h.Service.Merge(r.Context(), previous, id, body.Token, r.Method == "POST")
	if err != nil {
		var blocker interface{ MergeBlockReason() string }
		if errors.As(err, &blocker) {
			httpapi.Failure(w, r, 409, "merge_business_conflict", blocker.MergeBlockReason())
			return
		}
		if errors.Is(err, ErrSession) {
			httpapi.Failure(w, r, 401, "unauthenticated", "请重新登录")
		} else {
			httpapi.Failure(w, r, 409, "merge_changed", "账号资料已变化或验证已过期，请重新预览或验证")
		}
		return
	}
	httpapi.Respond(w, r, 200, preview)
}

func (h Handler) characters(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Service.Characters(r.Context(), Principal(r.Context()).UserID)
	if err != nil {
		httpapi.Failure(w, r, 503, "identity_unavailable", "角色信息暂不可用")
		return
	}
	httpapi.Respond(w, r, 200, map[string]any{"characters": rows})
}
func (h Handler) changeCharacter(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		httpapi.Failure(w, r, 400, "invalid_character", "角色无效")
		return
	}
	err = h.Service.ChangeCharacter(r.Context(), httpapi.CookieValue(r, httpapi.SessionCookie), id, r.Method == "DELETE", h.Cleanup)
	switch {
	case errors.Is(err, ErrCharacter):
		httpapi.Failure(w, r, 404, "character_unavailable", "角色不可用")
	case errors.Is(err, ErrMain):
		httpapi.Failure(w, r, 409, "main_character", "请先设置其他主角色")
	case errors.Is(err, ErrLast):
		httpapi.Failure(w, r, 409, "last_character", "请保留至少一个可登录角色")
	case errors.Is(err, ErrSession):
		httpapi.Failure(w, r, 401, "unauthenticated", "请重新登录")
	case err != nil:
		httpapi.Failure(w, r, 503, "identity_unavailable", "操作失败，请稍后重试")
	default:
		httpapi.Respond(w, r, 200, map[string]bool{"updated": true})
	}
}

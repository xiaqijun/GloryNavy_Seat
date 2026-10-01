package eve

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/module"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
	"glorynavy.local/seat/internal/modules/identity"
)

type SignIn func(context.Context, Character, string) (string, error)
type SSO interface {
	Configured() bool
	AuthorizationURL(context.Context, string, string) (string, error)
	Exchange(context.Context, string, string) (Character, error)
}
type Handler struct {
	pool              *pgxpool.Pool
	sso               SSO
	signIn            SignIn
	origin            string
	secure            bool
	logger            *slog.Logger
	mu                sync.Mutex
	starts            map[string]startWindow
	Complete          func(context.Context, Character, string, identity.LoginIntent) (string, error)
	Accounts          *identity.Service
	Sync              *SyncHTTP
	Contracts         *ContractHTTP
	PublicCorporation *PublicCorporationService
	PublicActivity    *PublicActivityService
}
type startWindow struct {
	Count int
	Until time.Time
}

func NewHandler(pool *pgxpool.Pool, sso SSO, signIn SignIn, origin string, secure bool, logger *slog.Logger) *Handler {
	return &Handler{pool: pool, sso: sso, signIn: signIn, origin: origin, secure: secure, logger: logger, starts: map[string]startWindow{}}
}
func (h *Handler) allowStart(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	for key, window := range h.starts {
		if now.After(window.Until) {
			delete(h.starts, key)
		}
	}
	window, exists := h.starts[host]
	if !exists {
		if len(h.starts) >= 4096 {
			return false
		}
		window.Until = now.Add(10 * time.Minute)
	}
	if window.Count >= 20 {
		return false
	}
	window.Count++
	h.starts[host] = window
	return true
}
func (h *Handler) result(w http.ResponseWriter, r *http.Request, code string) {
	http.Redirect(w, r, "/login?error="+code, http.StatusSeeOther)
}
func (h *Handler) unavailable(w http.ResponseWriter, r *http.Request) {
	h.logger.Warn("EVE login unavailable", "request_id", httpapi.RequestID(r))
	h.result(w, r, "unavailable")
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	h.start(w, r, identity.LoginIntent{Kind: "login"})
}
func (h *Handler) accountStart(w http.ResponseWriter, r *http.Request) {
	session := identity.Principal(r.Context())
	if session == nil || h.Accounts == nil || h.Complete == nil {
		httpapi.Failure(w, r, 503, "identity_unavailable", "角色服务暂不可用")
		return
	}
	intent := identity.LoginIntent{Kind: "link", UserID: session.UserID}
	if r.URL.Path == "/api/v1/eve/accounts/merge" {
		intent.Kind = "merge"
	}
	if raw := chi.URLParam(r, "id"); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			httpapi.Failure(w, r, 400, "invalid_character", "角色无效")
			return
		}
		owned, err := h.Accounts.ActiveCharacters(r.Context(), session.UserID)
		if err != nil {
			httpapi.Failure(w, r, 503, "identity_unavailable", "角色服务暂不可用")
			return
		}
		found := false
		for _, ch := range owned {
			if ch.ID == id {
				found = true
			}
		}
		if !found {
			httpapi.Failure(w, r, 404, "character_unavailable", "角色不可用")
			return
		}
		intent.Kind = "reauthorize"
		intent.ExpectedID = id
	}
	h.start(w, r, intent)
}
func (h *Handler) start(w http.ResponseWriter, r *http.Request, intent identity.LoginIntent) {
	unavailable := func() {
		if intent.Kind == "login" {
			h.unavailable(w, r)
		} else {
			httpapi.Failure(w, r, 503, "eve_unavailable", "EVE 授权暂不可用，请稍后重试")
		}
	}
	if !httpapi.SameOrigin(r, h.origin) {
		httpapi.Failure(w, r, 403, "csrf_failed", "请从本站登录页面重试")
		return
	}
	if !h.sso.Configured() {
		if intent.Kind != "login" {
			httpapi.Failure(w, r, 503, "not_configured", "EVE 登录尚未配置")
			return
		}
		h.result(w, r, "not_configured")
		return
	}
	if !h.allowStart(r) {
		w.Header().Set("Retry-After", "600")
		httpapi.Failure(w, r, 429, "rate_limited", "登录尝试过于频繁，请稍后重试")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	state, browser := rand.Text(), rand.Text()
	verifier := base64.RawURLEncoding.EncodeToString(randBytes())
	target, err := h.sso.AuthorizationURL(ctx, state, verifier)
	if err != nil {
		unavailable()
		return
	}
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		unavailable()
		return
	}
	defer tx.Rollback(context.Background())
	q := store.New(tx)
	if err = q.PruneFlows(ctx); err != nil {
		unavailable()
		return
	}
	required := []string{}
	if provider, ok := h.sso.(interface{ RequestedScopes() []string }); ok {
		required = provider.RequestedScopes()
	}
	err = q.CreateFlow(ctx, store.CreateFlowParams{StateHash: httpapi.Hash(state), BrowserHash: httpapi.Hash(browser), SessionHash: httpapi.Hash(httpapi.CookieValue(r, httpapi.SessionCookie)), Verifier: verifier, RequestedScopes: required})
	if err != nil {
		unavailable()
		return
	}
	if intent.Kind != "login" {
		var user pgtype.UUID
		if user.Scan(intent.UserID) != nil {
			unavailable()
			return
		}
		err = q.SetFlowIntent(ctx, store.SetFlowIntentParams{StateHash: httpapi.Hash(state), Intent: intent.Kind, TargetUserID: user, ExpectedCharacterID: pgtype.Int8{Int64: intent.ExpectedID, Valid: intent.ExpectedID > 0}})
		if err != nil {
			unavailable()
			return
		}
	}
	if err = tx.Commit(ctx); err != nil {
		unavailable()
		return
	}
	httpapi.SetCookie(w, httpapi.FlowCookie, browser, 600, h.secure)
	if intent.Kind != "login" {
		httpapi.Respond(w, r, 200, map[string]string{"url": target})
		return
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}
func randBytes() []byte { b := make([]byte, 32); _, _ = rand.Read(b); return b }

func (h *Handler) callback(w http.ResponseWriter, r *http.Request) {
	if !h.sso.Configured() {
		h.result(w, r, "not_configured")
		return
	}
	query := r.URL.Query()
	state, code := query.Get("state"), query.Get("code")
	browser := httpapi.CookieValue(r, httpapi.FlowCookie)
	if state == "" || len(state) > 128 || browser == "" || len(code) > 4096 || len(query["state"]) != 1 || len(query["code"]) > 1 || len(query["error"]) > 1 {
		h.result(w, r, "invalid_state")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	previous := httpapi.CookieValue(r, httpapi.SessionCookie)
	flow, err := store.New(h.pool).ConsumeFlow(ctx, store.ConsumeFlowParams{StateHash: httpapi.Hash(state), BrowserHash: httpapi.Hash(browser), SessionHash: httpapi.Hash(previous)})
	if errors.Is(err, pgx.ErrNoRows) {
		h.result(w, r, "invalid_state")
		return
	}
	if err != nil {
		h.unavailable(w, r)
		return
	}
	httpapi.SetCookie(w, httpapi.FlowCookie, "", -1, h.secure)
	result := func(code string) {
		if flow.Intent == "login" {
			h.result(w, r, code)
		} else {
			http.Redirect(w, r, "/account?error="+code, http.StatusSeeOther)
		}
	}
	if flow.Intent != "login" {
		if h.Accounts == nil || h.Complete == nil {
			result("unavailable")
			return
		}
		session, e := h.Accounts.Session(ctx, previous)
		if e != nil || session == nil || session.UserID != flow.TargetUserID.String() {
			result("session_expired")
			return
		}
	}
	if query.Get("error") != "" {
		result("cancelled")
		return
	}
	if code == "" {
		result("invalid_state")
		return
	}
	character, err := h.sso.Exchange(ctx, code, flow.Verifier)
	if err != nil {
		if errors.Is(err, ErrReauthorize) {
			result("authorization_required")
			return
		}
		result("unavailable")
		return
	}
	if len(flow.RequestedScopes) > 0 && (character.authorization == nil || !hasScopes(character.authorization.Scopes, flow.RequestedScopes)) {
		result("authorization_required")
		return
	}
	if flow.Intent == "reauthorize" && character.ID != flow.ExpectedCharacterID.Int64 {
		result("wrong_character")
		return
	}
	var token string
	if h.Complete != nil {
		token, err = h.Complete(ctx, character, previous, identity.LoginIntent{Kind: flow.Intent, UserID: flow.TargetUserID.String(), ExpectedID: flow.ExpectedCharacterID.Int64})
	} else {
		token, err = h.signIn(ctx, character, previous)
	}
	if err != nil {
		h.logger.Warn("EVE identity could not sign in", "request_id", httpapi.RequestID(r))
		code := "identity_rejected"
		if errors.Is(err, identity.ErrConflict) {
			code = "character_conflict"
		}
		if errors.Is(err, identity.ErrMerge) {
			code = "merge_rejected"
		}
		if errors.Is(err, identity.ErrSession) {
			code = "session_expired"
		}
		result(code)
		return
	}
	// Match the identity service's initial lifetime; authorized activity renews it.
	if flow.Intent == "login" {
		httpapi.SetCookie(w, httpapi.SessionCookie, token, int(identity.SessionLifetime/time.Second), h.secure)
	}
	if flow.Intent == "merge" {
		http.Redirect(w, r, "/account?merge="+token, http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/account", http.StatusSeeOther)
}

func (h *Handler) Module() module.Definition {
	if h.PublicActivity == nil {
		h.PublicActivity = NewPublicActivity()
	}
	if h.PublicCorporation == nil {
		h.PublicCorporation = NewPublicCorporation(nil)
	}
	d := module.Definition{
		Manifest:    module.Manifest{ID: "eve", Version: "0.1.0", APIVersion: 1, Requires: []module.Dependency{{ID: "identity", APIVersion: 1}}},
		Permissions: []string{"eve.characters.manage"},
		Routes: []module.Route{
			{Method: "GET", Path: "/public/activity", Public: true, Handler: h.PublicActivity},
			{Method: "GET", Path: "/public/corporation", Public: true, Handler: h.PublicCorporation},
			{Method: "POST", Path: "/accounts/merge", Permission: "eve.characters.manage", Handler: http.HandlerFunc(h.accountStart)},
			{Method: "GET", Path: "/login-status", Public: true, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				httpapi.Respond(w, r, 200, map[string]bool{"configured": h.sso.Configured()})
			})},
			{Method: "GET", Path: "/status", Permission: "eve.characters.manage", Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				roles := false
				if provider, ok := h.sso.(interface{ CorporationRolesEnabled() bool }); ok {
					roles = provider.CorporationRolesEnabled()
				}
				scopes := []string{}
				if provider, ok := h.sso.(interface{ RequestedScopes() []string }); ok {
					scopes = provider.RequestedScopes()
				}
				httpapi.Respond(w, r, 200, map[string]any{"configured": h.sso.Configured(), "corporation_roles": roles, "scopes": scopes})
			})},
			{Method: "POST", Path: "/login", Public: true, Handler: http.HandlerFunc(h.login)},
			{Method: "POST", Path: "/characters/link", Permission: "eve.characters.manage", Handler: http.HandlerFunc(h.accountStart)},
			{Method: "POST", Path: "/characters/{id}/reauthorize", Permission: "eve.characters.manage", Handler: http.HandlerFunc(h.accountStart)},
			{Method: "GET", Path: "/callback", Public: true, Handler: http.HandlerFunc(h.callback)},
		},
	}
	if h.Sync != nil {
		d.Permissions = append(d.Permissions, "eve.sync.self", "eve.sync.manage")
		d.Routes = append(d.Routes, h.Sync.Routes()...)
	}
	if h.Contracts != nil {
		d.Permissions = append(d.Permissions, "eve.contracts.read")
		d.Routes = append(d.Routes, h.Contracts.Routes()...)
	}
	return d
}

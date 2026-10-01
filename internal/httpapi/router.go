package httpapi

import (
	"context"
	"crypto/rand"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"glorynavy.local/seat/internal/module"
	"glorynavy.local/seat/internal/platform/locale"
)

func New(logger *slog.Logger, registry *module.Registry, ready http.Handler, guards ...module.Authorizer) http.Handler {
	r := chi.NewRouter()
	r.Use(locale.Middleware)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			id := rand.Text()
			w.Header().Set("X-Request-ID", id)
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Referrer-Policy", "no-referrer")
			req = req.WithContext(context.WithValue(req.Context(), requestKey{}, id))
			ww := middleware.NewWrapResponseWriter(w, req.ProtoMajor)
			start := time.Now()
			defer func() {
				if recover() != nil {
					logger.Error("request panic", "request_id", id)
					if ww.Status() == 0 {
						Failure(ww, req, 500, "internal_error", "服务暂时不可用")
					}
				}
				route := chi.RouteContext(req.Context()).RoutePattern()
				logger.Info("http request", "request_id", id, "method", req.Method, "route", route, "status", ww.Status(), "duration_ms", time.Since(start).Milliseconds())
			}()
			next.ServeHTTP(ww, req)
		})
	})
	r.Get("/health/live", func(w http.ResponseWriter, r *http.Request) { Respond(w, r, 200, map[string]string{"status": "alive"}) })
	r.Method(http.MethodGet, "/health/ready", ready)
	var catalog http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { Respond(w, req, http.StatusOK, registry.Manifests()) })
	if len(guards) > 0 && guards[0] != nil {
		catalog = guards[0]("identity.session.read", catalog)
	} else {
		catalog = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { Failure(w, r, 401, "unauthenticated", "请先登录") })
	}
	r.Method(http.MethodGet, "/api/v1/modules", catalog)
	for _, endpoint := range registry.Endpoints() {
		r.Method(endpoint.Method, endpoint.Pattern, endpoint.Handler)
	}

	r.NotFound(func(w http.ResponseWriter, r *http.Request) { Failure(w, r, 404, "not_found", "接口不存在") })
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		Failure(w, r, 405, "method_not_allowed", "不支持此请求方法")
	})
	return r
}

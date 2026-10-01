package system

import (
	"context"
	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/module"
	"log/slog"
	"net/http"
	"time"
)

type StatusService interface {
	Status(context.Context) (Status, error)
}

func StatusHandler(service StatusService, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		data, err := service.Status(ctx)
		if err != nil {
			logger.Warn("database readiness check failed", "request_id", httpapi.RequestID(r))
			httpapi.Failure(w, r, 503, "not_ready", "数据库暂未就绪，请稍后重试")
			return
		}
		httpapi.Respond(w, r, 200, data)
	})
}

func Module(status http.Handler) module.Definition {
	return module.Definition{Manifest: module.Manifest{ID: "system", Version: "0.1.0", APIVersion: 1}, Permissions: []string{"system.status.read"}, Routes: []module.Route{{Method: http.MethodGet, Path: "/status", Permission: "system.status.read", Handler: status}}}
}

// ReadyHandler returns no deployment, module or database metadata.
func ReadyHandler(service StatusService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if _, err := service.Status(ctx); err != nil {
			httpapi.Failure(w, r, 503, "not_ready", "服务暂未就绪")
			return
		}
		httpapi.Respond(w, r, 200, map[string]string{"status": "ready"})
	})
}

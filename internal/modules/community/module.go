package community

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/jackc/pgx/v5"

	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/module"
)

type Handler struct {
	Service *Service
	User    func(*http.Request) string
}

func (h Handler) get(w http.ResponseWriter, r *http.Request) {
	p, err := h.Service.Get(r.Context(), h.User(r))
	if err != nil {
		httpapi.Failure(w, r, 503, "community_unavailable", "社区资料暂不可用")
		return
	}
	httpapi.Respond(w, r, 200, p)
}
func (h Handler) update(w http.ResponseWriter, r *http.Request) {
	var body struct {
		QQ      string `json:"qq_number"`
		KOOK    string `json:"kook_name"`
		Version string `json:"version"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&body) != nil || decoder.Decode(new(any)) != io.EOF {
		httpapi.Failure(w, r, 400, "invalid_profile", "资料格式无效")
		return
	}
	p, err := h.Service.Update(r.Context(), h.User(r), body.QQ, body.KOOK, body.Version)
	switch {
	case errors.Is(err, ErrQQ):
		httpapi.Failure(w, r, 400, "invalid_qq", ErrQQ.Error())
	case errors.Is(err, ErrKOOK):
		httpapi.Failure(w, r, 400, "invalid_kook", ErrKOOK.Error())
	case errors.Is(err, ErrVersion):
		httpapi.Failure(w, r, 409, "profile_conflict", ErrVersion.Error())
	case err != nil:
		httpapi.Failure(w, r, 503, "community_unavailable", "保存失败，请稍后重试")
	default:
		httpapi.Respond(w, r, 200, p)
	}
}

func (h Handler) createQQGroupApplication(w http.ResponseWriter, r *http.Request) {
	var body struct {
		GroupOpenID string `json:"group_openid"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&body) != nil || decoder.Decode(new(any)) != io.EOF {
		httpapi.Failure(w, r, 400, "invalid_group_application", "入群申请格式无效")
		return
	}
	application, err := h.Service.CreateQQGroupApplication(r.Context(), h.User(r), body.GroupOpenID)
	switch {
	case errors.Is(err, ErrQQNotBound):
		httpapi.Failure(w, r, 409, "qq_profile_required", "请先填写 QQ 号")
	case errors.Is(err, ErrQQGroupNotConfigured):
		httpapi.Failure(w, r, 503, "qq_group_unavailable", "入群审批群组尚未配置")
	case errors.Is(err, ErrQQGroupApplicationExists):
		httpapi.Failure(w, r, 409, "group_application_exists", "该群已有待处理的入群申请")
	case err != nil:
		httpapi.Failure(w, r, 503, "community_unavailable", "入群申请暂不可用，请稍后重试")
	default:
		httpapi.Respond(w, r, http.StatusCreated, application)
	}
}

func (h Handler) getQQGroupApplication(w http.ResponseWriter, r *http.Request) {
	application, err := h.Service.QQGroupApplication(r.Context(), h.User(r))
	if errors.Is(err, pgx.ErrNoRows) {
		httpapi.Respond(w, r, http.StatusOK, nil)
		return
	}
	if err != nil {
		httpapi.Failure(w, r, 503, "community_unavailable", "入群申请暂不可用，请稍后重试")
		return
	}
	httpapi.Respond(w, r, http.StatusOK, application)
}

func (h Handler) syncQQGroupApplications(w http.ResponseWriter, r *http.Request) {
	if err := h.Service.SyncQQGroupApplications(r.Context()); err != nil {
		httpapi.Failure(w, r, 503, "qq_group_sync_failed", "入群申请同步失败，请稍后重试")
		return
	}
	httpapi.Respond(w, r, http.StatusOK, map[string]string{"status": "ok"})
}

func (h Handler) listQQGroupApplications(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	limit := int32(100)
	if raw := query.Get("limit"); raw != "" {
		if _, err := fmt.Sscan(raw, &limit); err != nil {
			httpapi.Failure(w, r, 400, "invalid_group_application_query", "入群申请查询参数无效")
			return
		}
	}
	items, err := h.Service.ListQQGroupApplications(r.Context(), query.Get("group_openid"), query.Get("status"), limit)
	if err != nil {
		httpapi.Failure(w, r, 503, "community_unavailable", "入群申请暂不可用，请稍后重试")
		return
	}
	httpapi.Respond(w, r, http.StatusOK, map[string]any{"items": items})
}

func (h Handler) getQQGroupSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.Service.ListQQGroupSettings(r.Context())
	if err != nil {
		httpapi.Failure(w, r, 503, "community_unavailable", "QQ 群配置暂不可用，请稍后重试")
		return
	}
	httpapi.Respond(w, r, http.StatusOK, settings)
}

func (h Handler) getQQGroupOptions(w http.ResponseWriter, r *http.Request) {
	options, err := h.Service.ListQQGroupOptions(r.Context())
	if err != nil {
		httpapi.Failure(w, r, 503, "community_unavailable", "QQ 群列表暂不可用，请稍后重试")
		return
	}
	httpapi.Respond(w, r, http.StatusOK, options)
}

func (h Handler) saveQQGroupSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Items []QQGroupSetting `json:"items"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&body) != nil || decoder.Decode(new(any)) != io.EOF {
		httpapi.Failure(w, r, 400, "invalid_qq_group_settings", "QQ 群配置格式无效")
		return
	}
	settings, err := h.Service.SaveQQGroupSettings(r.Context(), body.Items)
	if err != nil {
		httpapi.Failure(w, r, 400, "invalid_qq_group_settings", err.Error())
		return
	}
	httpapi.Respond(w, r, http.StatusOK, settings)
}

func (h Handler) getQQBotSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.Service.ListQQBotSettings(r.Context())
	if err != nil {
		httpapi.Failure(w, r, 503, "community_unavailable", "QQ 机器人配置暂不可用，请稍后重试")
		return
	}
	httpapi.Respond(w, r, http.StatusOK, settings)
}

func (h Handler) saveQQBotSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AppID   string `json:"app_id"`
		APIBase string `json:"api_base"`
		Secret  string `json:"secret"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&body) != nil || decoder.Decode(new(any)) != io.EOF {
		httpapi.Failure(w, r, 400, "invalid_qq_bot_settings", "QQ 机器人配置格式无效")
		return
	}
	settings, err := h.Service.SaveQQBotSettings(r.Context(), body.AppID, body.APIBase, body.Secret)
	if errors.Is(err, ErrQQBotSecretKey) {
		httpapi.Failure(w, r, 503, "qq_bot_unavailable", err.Error())
		return
	}
	if err != nil {
		httpapi.Failure(w, r, 400, "invalid_qq_bot_settings", err.Error())
		return
	}
	httpapi.Respond(w, r, http.StatusOK, settings)
}
func (h Handler) Module() module.Definition {
	return module.Definition{
		Manifest:    module.Manifest{ID: "community", Version: "0.1.0", APIVersion: 1, Requires: []module.Dependency{{ID: "identity", APIVersion: 1}}},
		Permissions: []string{"community.profile.read", "community.profile.update", "community.group.apply", "community.group.manage"},
		Routes: []module.Route{
			{Method: "GET", Path: "/profile", Permission: "community.profile.read", Handler: http.HandlerFunc(h.get)},
			{Method: "PUT", Path: "/profile", Permission: "community.profile.update", Handler: http.HandlerFunc(h.update)},
			{Method: "POST", Path: "/qq/challenge", Permission: "community.profile.update", Handler: http.HandlerFunc(h.createOfficialQQChallenge)},
			{Method: "POST", Path: "/qq/group/application", Permission: "community.group.apply", Handler: http.HandlerFunc(h.createQQGroupApplication)},
			{Method: "GET", Path: "/qq/group/application", Permission: "community.group.apply", Handler: http.HandlerFunc(h.getQQGroupApplication)},
			{Method: "GET", Path: "/qq/group/options", Permission: "community.group.apply", Handler: http.HandlerFunc(h.getQQGroupOptions)},
			{Method: "GET", Path: "/qq/group/applications", Permission: "community.group.manage", Handler: http.HandlerFunc(h.listQQGroupApplications)},
			{Method: "POST", Path: "/qq/group/sync", Permission: "community.group.manage", Handler: http.HandlerFunc(h.syncQQGroupApplications)},
			{Method: "GET", Path: "/qq/group/settings", Permission: "community.group.manage", Handler: http.HandlerFunc(h.getQQGroupSettings)},
			{Method: "PUT", Path: "/qq/group/settings", Permission: "community.group.manage", Handler: http.HandlerFunc(h.saveQQGroupSettings)},
			{Method: "GET", Path: "/qq/bot/settings", Permission: "community.group.manage", Handler: http.HandlerFunc(h.getQQBotSettings)},
			{Method: "PUT", Path: "/qq/bot/settings", Permission: "community.group.manage", Handler: http.HandlerFunc(h.saveQQBotSettings)},
			{Method: "POST", Path: "/qq/official/webhook", Public: true, Handler: http.HandlerFunc(h.officialQQWebhook)},
		},
	}
}

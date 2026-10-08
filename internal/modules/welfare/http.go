package welfare

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/module"
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
	"io"
	"net/http"
	"strconv"
	"strings"
)

type Handler struct {
	Service *Service
	User    func(*http.Request) string
}

func (h Handler) Module() module.Definition {
	routes := []module.Route{}
	routes = append(routes, module.Route{Method: "GET", Path: "/growth/check", Permission: "welfare.self", Handler: http.HandlerFunc(h.growthCheck)})
	routes = append(routes, module.Route{Method: "POST", Path: "/activity/projects", Permission: "welfare.self", Handler: http.HandlerFunc(h.activityProject)})
	routes = append(routes, module.Route{Method: "POST", Path: "/activity/applications", Permission: "welfare.self", Handler: http.HandlerFunc(h.activityApply)})
	routes = append(routes, module.Route{Method: "GET", Path: "/cases/{id}/images/{ordinal}", Permission: "welfare.self", Handler: http.HandlerFunc(h.activityImage)})
	routes = append(routes, module.Route{Method: "GET", Path: "/items", Permission: "welfare.self", Handler: http.HandlerFunc(h.items)})
	routes = append(routes, module.Route{Method: "GET", Path: "/cases/{id}/contracts", Permission: "welfare.self", Handler: http.HandlerFunc(h.deliveryCandidates)})
	routes = append(routes, module.Route{Method: "GET", Path: "/losses", Permission: "welfare.self", Handler: http.HandlerFunc(h.losses)})
	routes = append(routes, module.Route{Method: "GET", Path: "/losses/prices", Permission: "welfare.self", Handler: http.HandlerFunc(h.lossPrices)})
	routes = append(routes, module.Route{Method: "GET", Path: "/ships", Permission: "welfare.self", Handler: http.HandlerFunc(h.ships)})
	for _, v := range []struct {
		method, path string
		fn           http.HandlerFunc
	}{{"GET", "/context", h.context}, {"GET", "/cases", h.list}, {"GET", "/cases/{id}", h.detail}, {"GET", "/members", h.members}, {"GET", "/profile", h.profile}, {"POST", "/preview", h.preview}, {"POST", "/commands", h.command}, {"GET", "/settlements", h.settlementList}, {"GET", "/settlements/{id}", h.settlementDetail}, {"POST", "/settlements", h.settlementCreate}, {"POST", "/settlements/{id}/retry", h.settlementRetry}} {
		routes = append(routes, module.Route{Method: v.method, Path: v.path, Permission: "welfare.self", Handler: v.fn})
	}
	return module.Definition{Manifest: module.Manifest{ID: "welfare", Version: "0.1.0", APIVersion: 1, Requires: []module.Dependency{{ID: "identity", APIVersion: 1}, {ID: "eve", APIVersion: 1}, {ID: "access", APIVersion: 1}, {ID: "exchange", APIVersion: 1}}}, Permissions: []string{"welfare.self"}, Routes: routes}
}

func (h Handler) settlementList(w http.ResponseWriter, r *http.Request) {
	rows, e := h.Service.SettlementBatches(r.Context(), h.User(r))
	respond(w, r, map[string]any{"items": rows}, e)
}

func (h Handler) settlementDetail(w http.ResponseWriter, r *http.Request) {
	id := number(chi.URLParam(r, "id"))
	if id == 0 {
		respond(w, r, nil, ErrInvalid)
		return
	}
	v, e := h.Service.SettlementBatch(r.Context(), h.User(r), id)
	respond(w, r, v, e)
}

func (h Handler) settlementCreate(w http.ResponseWriter, r *http.Request) {
	var input SettlementInput
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	d.DisallowUnknownFields()
	if d.Decode(&input) != nil || d.Decode(new(any)) != io.EOF {
		respond(w, r, nil, ErrInvalid)
		return
	}
	v, e := h.Service.CreateSettlementBatch(r.Context(), h.User(r), input)
	respond(w, r, v, e)
}

func (h Handler) settlementRetry(w http.ResponseWriter, r *http.Request) {
	id := number(chi.URLParam(r, "id"))
	if id == 0 {
		respond(w, r, nil, ErrInvalid)
		return
	}
	e := h.Service.RetrySettlementBatch(r.Context(), h.User(r), id)
	respond(w, r, map[string]any{"ok": e == nil}, e)
}
func (h Handler) losses(w http.ResponseWriter, r *http.Request) {
	corp, char := number(r.URL.Query().Get("corporation_id")), number(r.URL.Query().Get("character_id"))
	if corp == 0 || char == 0 || h.Service.Losses == nil {
		respond(w, r, nil, ErrInvalid)
		return
	}
	v, e := h.Service.Losses(r.Context(), h.User(r), corp, char, number(r.URL.Query().Get("before")), number(r.URL.Query().Get("killmail_id")))
	if e != nil {
		respond(w, r, nil, e)
		return
	}
	next := ""
	if len(v) > 30 {
		next = strconv.FormatInt(v[29].ID, 10)
		v = v[:30]
	}
	items, e := h.Service.annotateLosses(r.Context(), h.User(r), corp, char, v)
	respond(w, r, map[string]any{"items": items, "next_cursor": next}, e)
}
func (h Handler) ships(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(q)) < 2 || len([]rune(q)) > 80 {
		respond(w, r, nil, ErrInvalid)
		return
	}
	if e := h.Service.allowed(r.Context(), h.User(r), number(r.URL.Query().Get("corporation_id")), false); e != nil {
		respond(w, r, nil, e)
		return
	}
	v, e := h.Service.SearchShips(r.Context(), q)
	respond(w, r, map[string]any{"items": v}, e)
}

func (h Handler) items(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := h.User(r)
	if e := h.Service.admin(ctx, user); e != nil {
		respond(w, r, nil, e)
		return
	}
	if e := h.Service.allowed(ctx, user, number(r.URL.Query().Get("corporation_id")), true); e != nil {
		respond(w, r, nil, e)
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(q)) < 2 || len([]rune(q)) > 80 || h.Service.SearchItems == nil {
		respond(w, r, nil, ErrInvalid)
		return
	}
	rows, e := h.Service.SearchItems(ctx, q)
	out := []Ship{}
	for _, v := range rows {
		out = append(out, Ship{ID: v.ID, Name: v.Name})
	}
	respond(w, r, map[string]any{"items": out}, e)
}
func respond(w http.ResponseWriter, r *http.Request, v any, e error) {
	switch {
	case e == nil:
		httpapi.Respond(w, r, 200, v)
	case errors.Is(e, pgx.ErrNoRows):
		httpapi.Failure(w, r, 404, "welfare_unavailable", "记录不存在或无权访问")
	case errors.Is(e, ErrInvalid):
		httpapi.Failure(w, r, 400, "invalid_welfare", "请检查必填项、金额与日期")
	case errors.Is(e, ErrValuation):
		httpapi.Failure(w, r, 409, "welfare_valuation_required", "请重新核价，或选择人工核价并填写原因")
	case errors.Is(e, ErrCapitalPurchase):
		httpapi.Failure(w, r, 409, "welfare_purchase_required", "请同步已完成的个人购舰合同，合同须包含一艘对应旗舰")
	case errors.Is(e, ErrAttendanceRequired):
		httpapi.Failure(w, r, 409, "welfare_attendance_required", "该损失尚未关联有效出勤，无法申请军团补损")
	case errors.Is(e, ErrLossDailyQuota):
		httpapi.Failure(w, r, 409, "welfare_loss_quota", "该成员今日补损额度不足，请核对额度设置")
	case errors.Is(e, ErrLossWeeklyQuota):
		httpapi.Failure(w, r, 409, "welfare_loss_quota", "该成员本周补损额度不足，请核对额度设置")
	case errors.Is(e, ErrLossMonthlyQuota):
		httpapi.Failure(w, r, 409, "welfare_loss_quota", "该成员本月补损额度不足，请核对额度设置")
	case errors.Is(e, ErrRule):
		httpapi.Failure(w, r, 409, "welfare_rule_required", "规则、历史资格或证明尚未核实，请补充后重试")
	case errors.Is(e, ErrGrowthUnmet):
		httpapi.Failure(w, r, 409, "growth_skills_required", "尚未满足成长福利技能要求，请同步技能后重试")
	case errors.Is(e, ErrGrowthClaimed):
		httpapi.Failure(w, r, 409, "growth_already_claimed", "该船型成长福利已申请或已领取，所有绑定角色共享一次资格")
	case errors.Is(e, ErrActivityClaimed):
		httpapi.Failure(w, r, 409, "activity_already_claimed", "该角色已达到活动福利领取次数上限")
	case errors.Is(e, ErrConflict):
		httpapi.Failure(w, r, 409, "welfare_conflict", "记录已更新或资格已占用，请刷新核对")
	case errors.Is(e, ErrCancellationPayment):
		httpapi.Failure(w, r, 409, "welfare_cancellation_payment", "合同仍可能发放或已交付，暂不能确认取消；请核对游戏合同并等待同步")
	case errors.Is(e, ErrDelivery):
		httpapi.Failure(w, r, 409, "welfare_delivery_contract", "合同不满足交付核验条件，请刷新后核对双方、物品与状态")
	case errors.Is(e, ErrSettlementGroup):
		httpapi.Failure(w, r, 409, "welfare_settlement_group", "批量结算只能包含同一账号组的记录，请分开提交")
	case errors.Is(e, ErrSettlementUnsupported):
		httpapi.Failure(w, r, 409, "welfare_settlement_unsupported", "所选记录包含暂不支持合并合同的奖励，请拆分结算")
	default:
		httpapi.Failure(w, r, 503, "welfare_unavailable", "福利服务暂不可用，请重试")
	}
}
func (h Handler) deliveryCandidates(w http.ResponseWriter, r *http.Request) {
	id := number(chi.URLParam(r, "id"))
	q := r.URL.Query().Get("contract_id")
	cid := number(q)
	if id == 0 || q != "" && cid == 0 {
		respond(w, r, nil, ErrInvalid)
		return
	}
	items, e := h.Service.DeliveryCandidates(r.Context(), h.User(r), id, cid)
	respond(w, r, map[string]any{"items": items}, e)
}
func number(v string) int64 {
	i, e := strconv.ParseInt(v, 10, 64)
	if e != nil || i <= 0 {
		return 0
	}
	return i
}
func (h Handler) context(w http.ResponseWriter, r *http.Request) {
	u := h.User(r)
	cs, e := h.Service.Corporations(r.Context(), u)
	if e != nil {
		respond(w, r, nil, e)
		return
	}
	chars, e := h.Service.Characters(r.Context(), u)
	if e != nil {
		respond(w, r, nil, e)
		return
	}
	a, e := h.Service.Administrator(r.Context(), u)
	if e != nil {
		respond(w, r, nil, e)
		return
	}
	corp := number(r.URL.Query().Get("corporation_id"))
	policies := []Policy{}
	quotas := []LossQuota{}
	if corp > 0 {
		if e = h.Service.allowed(r.Context(), u, corp, false); e == nil {
			policies, e = store.Policies(r.Context(), h.Service.Pool, corp)
			if e == nil {
				quotas, e = h.Service.LossQuotas(r.Context(), u, corp, policies)
			}
		} else if e = h.Service.allowedLoss(r.Context(), u, corp, true); e == nil {
			policies, e = store.Policies(r.Context(), h.Service.Pool, corp)
			if e == nil {
				quotas, e = h.Service.LossQuotas(r.Context(), u, corp, policies)
			}
		}
	}
	if e == nil {
		policies = h.Service.presentPolicies(r.Context(), policies)
	}
	respond(w, r, map[string]any{"corporations": cs, "characters": chars, "administrator": a, "policies": policies, "loss_quotas": quotas}, e)
}
func (h Handler) list(w http.ResponseWriter, r *http.Request) {
	v, e := h.Service.List(r.Context(), h.User(r), number(r.URL.Query().Get("corporation_id")), r.URL.Query().Get("scope") == "all", r.URL.Query().Get("kind"), number(r.URL.Query().Get("before")))
	next := ""
	if len(v) > 30 {
		next = strconv.FormatInt(v[29].ID, 10)
		v = v[:30]
	}
	if e == nil {
		v = h.Service.presentCases(r.Context(), v)
	}
	respond(w, r, map[string]any{"items": v, "next_cursor": next}, e)
}
func (h Handler) detail(w http.ResponseWriter, r *http.Request) {
	v, e := h.Service.Read(r.Context(), h.User(r), number(chi.URLParam(r, "id")))
	if e != nil {
		respond(w, r, nil, e)
		return
	}
	history, e := store.History(r.Context(), h.Service.Pool, v.ID)
	if e == nil {
		v = h.Service.presentCases(r.Context(), []Case{v})[0]
		v = h.Service.enrichCaseLoss(r.Context(), v)
	}
	respond(w, r, map[string]any{"item": v, "history": history}, e)
}
func (h Handler) members(w http.ResponseWriter, r *http.Request) {
	u := h.User(r)
	corp := number(r.URL.Query().Get("corporation_id"))
	if e := h.Service.allowed(r.Context(), u, corp, true); e != nil {
		respond(w, r, nil, e)
		return
	}
	v, e := h.Service.Members(r.Context(), u, corp)
	respond(w, r, map[string]any{"items": v}, e)
}
func (h Handler) profile(w http.ResponseWriter, r *http.Request) {
	u := h.User(r)
	owner := r.URL.Query().Get("member")
	if owner == "" {
		owner = u
	}
	if owner != u {
		if e := h.Service.admin(r.Context(), u); e != nil {
			respond(w, r, nil, e)
			return
		}
		if e := h.Service.member(r.Context(), u, number(r.URL.Query().Get("corporation_id")), owner); e != nil {
			respond(w, r, nil, e)
			return
		}
	}
	v, e := store.Profile(r.Context(), h.Service.Pool, owner)
	respond(w, r, v, e)
}
func body(w http.ResponseWriter, r *http.Request) (Command, error) {
	var c Command
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	d.DisallowUnknownFields()
	if d.Decode(&c) != nil || d.Decode(new(any)) != io.EOF {
		return c, ErrInvalid
	}
	return c, nil
}
func (h Handler) preview(w http.ResponseWriter, r *http.Request) {
	c, e := body(w, r)
	if e != nil {
		respond(w, r, nil, e)
		return
	}
	v, e := h.Service.Preview(r.Context(), h.User(r), c)
	respond(w, r, v, e)
}
func (h Handler) command(w http.ResponseWriter, r *http.Request) {
	c, e := body(w, r)
	if e != nil {
		respond(w, r, nil, e)
		return
	}
	v, e := h.Service.Execute(r.Context(), h.User(r), c)
	respond(w, r, v, e)
}
func (s *Service) MergeAccountTx(ctx context.Context, tx pgx.Tx, source, target string, apply bool) (json.RawMessage, error) {
	return store.Merge(ctx, tx, source, target, apply)
}

func (h Handler) growthCheck(w http.ResponseWriter, r *http.Request) {
	corp, char := number(r.URL.Query().Get("corporation_id")), number(r.URL.Query().Get("character_id"))
	if corp <= 0 || char <= 0 {
		respond(w, r, nil, ErrInvalid)
		return
	}
	v, e := h.Service.GrowthStatus(r.Context(), h.User(r), corp, char, r.URL.Query().Get("kind"))
	respond(w, r, v, e)
}

package loan

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/module"
	"glorynavy.local/seat/internal/modules/loan/internal/store"
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
	}{
		{"GET", "/context", h.context}, {"GET", "/credit", h.credit}, {"GET", "/cases", h.cases}, {"GET", "/cases/{id}", h.detail},
		{"POST", "/contributions", h.contributionCreate}, {"GET", "/contributions", h.contributions}, {"POST", "/contributions/{id}/deposit", h.contributionDeposit}, {"POST", "/contributions/{id}/auto-verify", h.contributionAutoVerify}, {"POST", "/contributions/{id}/cancel", h.contributionCancel}, {"POST", "/applications", h.applicationCreate}, {"POST", "/cases/{id}/review", h.review},
		{"POST", "/cases/{id}/payments", h.payment}, {"POST", "/cases/{id}/guarantees", h.guarantee}, {"POST", "/guarantees/{id}/decision", h.guaranteeDecision},
		{"GET", "/guarantees", h.guarantees},
		{"POST", "/cases/{id}/collateral", h.collateral}, {"POST", "/collateral/{id}/decision", h.collateralDecision},
	} {
		routes = append(routes, module.Route{Method: r.method, Path: r.path, Permission: "loan.self", Handler: r.fn})
	}
	return module.Definition{Manifest: module.Manifest{ID: "loan", Version: "0.1.0", APIVersion: 1, Requires: []module.Dependency{{ID: "identity", APIVersion: 1}, {ID: "eve", APIVersion: 1}, {ID: "access", APIVersion: 1}}}, Permissions: []string{"loan.self"}, Routes: routes}
}

func read(w http.ResponseWriter, r *http.Request, v any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 262144))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil || d.Decode(new(any)) != io.EOF {
		respond(w, r, nil, ErrInvalid)
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
func respond(w http.ResponseWriter, r *http.Request, v any, e error) {
	if e == nil {
		httpapi.Respond(w, r, 200, v)
		return
	}
	code, status, msg := "loan_unavailable", 503, "贷款服务暂不可用"
	switch {
	case errors.Is(e, ErrInvalid):
		code, status, msg = "invalid_loan", 400, "贷款参数无效"
	case errors.Is(e, ErrForbidden):
		code, status, msg = "loan_forbidden", 403, "没有该贷款记录或操作权限"
	case errors.Is(e, ErrRule):
		code, status, msg = "loan_rule_required", 409, "贷款规则或系统信用评估尚未完成"
	case errors.Is(e, ErrLimit):
		code, status, msg = "loan_limit_exceeded", 409, "超出贷款额度或资金池限制"
	case errors.Is(e, ErrCoverage):
		code, status, msg = "loan_coverage_required", 409, "请先完成担保或抵押覆盖"
	case errors.Is(e, ErrConflict):
		code, status, msg = "loan_conflict", 409, "贷款记录已变化，请刷新后重试"
	case errors.Is(e, ErrContract):
		code, status, msg = "loan_contract_invalid", 409, "合同尚未完成或金额、方向与贷款不符"
	case errors.Is(e, pgx.ErrNoRows):
		code, status, msg = "loan_unavailable", 404, "贷款记录不存在或无权访问"
	}
	httpapi.Failure(w, r, status, code, msg)
}

func (h Handler) context(w http.ResponseWriter, r *http.Request) {
	u := h.User(r)
	admin := false
	if h.Service.IsAdministrator != nil {
		admin, _ = h.Service.IsAdministrator(r.Context(), u)
	}
	p, e := h.Service.Pools(r.Context(), u, admin)
	if e != nil {
		respond(w, r, nil, e)
		return
	}
	c, e := h.Service.Credit(r.Context(), u)
	if e != nil {
		respond(w, r, nil, e)
		return
	}
	var summary any = map[string]any{"pending_minor": int64(0), "funded_minor": int64(0), "cash_minor": int64(0), "reserved_minor": int64(0), "available_minor": int64(0)}
	if len(p) > 0 {
		if v, er := store.Summary(r.Context(), h.Service.Pool, p[0].ID); er == nil {
			summary = v
		} else {
			respond(w, r, nil, er)
			return
		}
	}
	respond(w, r, map[string]any{"pools": p, "credit": c, "pool_summary": summary, "administrator": admin}, nil)
}
func (h Handler) credit(w http.ResponseWriter, r *http.Request) {
	c, e := h.Service.Credit(r.Context(), h.User(r))
	respond(w, r, c, e)
}
func (h Handler) cases(w http.ResponseWriter, r *http.Request) {
	admin := false
	if h.Service.IsAdministrator != nil {
		admin, _ = h.Service.IsAdministrator(r.Context(), h.User(r))
	}
	v, e := h.Service.List(r.Context(), h.User(r), admin)
	respond(w, r, map[string]any{"items": v}, e)
}
func (h Handler) detail(w http.ResponseWriter, r *http.Request) {
	id, e := number(chi.URLParam(r, "id"))
	if e != nil {
		respond(w, r, nil, e)
		return
	}
	v, e := h.Service.Detail(r.Context(), h.User(r), id)
	respond(w, r, v, e)
}
func (h Handler) contributions(w http.ResponseWriter, r *http.Request) {
	v, e := h.Service.Contributions(r.Context(), h.User(r))
	respond(w, r, map[string]any{"items": v}, e)
}
func (h Handler) contributionCreate(w http.ResponseWriter, r *http.Request) {
	var in ContributionInput
	if !read(w, r, &in) {
		return
	}
	v, e := h.Service.AddContribution(r.Context(), h.User(r), in)
	respond(w, r, v, e)
}
func (h Handler) contributionDeposit(w http.ResponseWriter, r *http.Request) {
	id, e := number(chi.URLParam(r, "id"))
	if e != nil {
		respond(w, r, nil, e)
		return
	}
	var in DepositInput
	if !read(w, r, &in) {
		return
	}
	v, e := h.Service.VerifyContribution(r.Context(), h.User(r), id, in)
	respond(w, r, v, e)
}
func (h Handler) contributionAutoVerify(w http.ResponseWriter, r *http.Request) {
	id, e := number(chi.URLParam(r, "id"))
	if e != nil {
		respond(w, r, nil, e)
		return
	}
	var in struct {
		Version int64 `json:"version"`
	}
	if !read(w, r, &in) {
		return
	}
	v, e := h.Service.AutoVerifyContribution(r.Context(), h.User(r), id, in.Version)
	respond(w, r, v, e)
}
func (h Handler) contributionCancel(w http.ResponseWriter, r *http.Request) {
	id, e := number(chi.URLParam(r, "id"))
	if e != nil {
		respond(w, r, nil, e)
		return
	}
	var in struct {
		Version int64 `json:"version"`
	}
	if !read(w, r, &in) {
		return
	}
	v, e := h.Service.CancelContribution(r.Context(), h.User(r), id, in.Version)
	respond(w, r, v, e)
}
func (h Handler) applicationCreate(w http.ResponseWriter, r *http.Request) {
	var in ApplicationInput
	if !read(w, r, &in) {
		return
	}
	v, e := h.Service.CreateApplication(r.Context(), h.User(r), in)
	respond(w, r, v, e)
}
func (h Handler) review(w http.ResponseWriter, r *http.Request) {
	id, e := number(chi.URLParam(r, "id"))
	if e != nil {
		respond(w, r, nil, e)
		return
	}
	var in ReviewInput
	if !read(w, r, &in) {
		return
	}
	v, e := h.Service.Review(r.Context(), h.User(r), id, in)
	respond(w, r, v, e)
}
func (h Handler) payment(w http.ResponseWriter, r *http.Request) {
	id, e := number(chi.URLParam(r, "id"))
	if e != nil {
		respond(w, r, nil, e)
		return
	}
	var in PaymentInput
	if !read(w, r, &in) {
		return
	}
	v, e := h.Service.createSharedPayment(r.Context(), h.User(r), id, in)
	respond(w, r, v, e)
}
func (h Handler) guarantee(w http.ResponseWriter, r *http.Request) {
	id, e := number(chi.URLParam(r, "id"))
	if e != nil {
		respond(w, r, nil, e)
		return
	}
	var in GuaranteeInput
	if !read(w, r, &in) {
		return
	}
	v, e := h.Service.AddGuarantee(r.Context(), h.User(r), id, in)
	respond(w, r, v, e)
}
func (h Handler) guaranteeDecision(w http.ResponseWriter, r *http.Request) {
	id, e := number(chi.URLParam(r, "id"))
	if e != nil {
		respond(w, r, nil, e)
		return
	}
	var in DecisionInput
	if !read(w, r, &in) {
		return
	}
	v, e := h.Service.DecideGuarantee(r.Context(), h.User(r), id, in)
	respond(w, r, v, e)
}
func (h Handler) guarantees(w http.ResponseWriter, r *http.Request) {
	v, e := h.Service.Guarantees(r.Context(), h.User(r))
	respond(w, r, map[string]any{"items": v}, e)
}
func (h Handler) collateral(w http.ResponseWriter, r *http.Request) {
	id, e := number(chi.URLParam(r, "id"))
	if e != nil {
		respond(w, r, nil, e)
		return
	}
	var in CollateralInput
	if !read(w, r, &in) {
		return
	}
	v, e := h.Service.AddCollateral(r.Context(), h.User(r), id, in)
	respond(w, r, v, e)
}
func (h Handler) collateralDecision(w http.ResponseWriter, r *http.Request) {
	id, e := number(chi.URLParam(r, "id"))
	if e != nil {
		respond(w, r, nil, e)
		return
	}
	var in DecisionInput
	if !read(w, r, &in) {
		return
	}
	v, e := h.Service.DecideCollateral(r.Context(), h.User(r), id, in)
	respond(w, r, v, e)
}

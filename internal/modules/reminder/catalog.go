package reminder

import (
	"context"
	"time"
)

// Sources return opaque references after checking the recipient's current access.
// They read local business facts only, never perform network calls or change state.
type Source func(context.Context, string, time.Time) ([]string, error)
type Event struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"-"`
}

var events = []Event{
	{"approval_pending", "待审批提醒", "/approvals"},
	{"feedback_update", "反馈回复与进度", "/feedback"},
	{"feedback_pending", "待处理反馈", "/feedback"},
	{"collateral_pending", "待交付抵押", "/loans"},
	{"contribution_result", "出借到账", "/loans"},
	{"welfare_result", "福利申请进度", "/welfare"},
	{"exchange_result", "兑换进度", "/exchange"},
	{"loan_result", "贷款进度", "/loans"},
	{"contract_pending", "待领取合同", "/contracts"},
	{"guarantee_pending", "待处理担保", "/loans"},
	{"loan_funding", "待放款提醒", "/approvals"},
	{"loan_due", "贷款即将到期", "/loans"},
	{"loan_overdue", "贷款逾期", "/loans"},
	{"pap_requirement", "PAP 未达标", "/attendance"},
	{"pap_change", "PAP 变动", "/attendance"},
	{"coin_change", "果壳币变动", "/exchange"},
	{"authorization_expired", "角色授权失效", "/account"},
	{"structure_alert", "建筑燃料与拆除", "/structures"},
	{"sync_anomaly", "数据同步异常", "/eve/sync"},
	{"sentry_alert", "预警服务异常", "/sentry"},
}

func (s *Service) source(id string) Source {
	if id == "approval_pending" && s.Pending != nil {
		return func(ctx context.Context, u string, _ time.Time) ([]string, error) { return s.Pending(ctx, u) }
	}
	return s.Sources[id]
}
func (s *Service) catalog() []Event {
	out := []Event{}
	for _, e := range events {
		if s.source(e.ID) != nil {
			out = append(out, e)
		}
	}
	return out
}
func eventByID(id string) (Event, bool) {
	for _, e := range events {
		if e.ID == id {
			return e, true
		}
	}
	return Event{}, false
}

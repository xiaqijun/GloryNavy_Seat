package store

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/platform/remindermessage"
	"strconv"
	"time"
)

type reminderDetail struct {
	Ref, State, Name, Account, Reason, Delivery                                      string
	Amount, Total, Principal, Interest, Sequence, Source, Recipient, Count, Contract int64
	Time                                                                             time.Time
	Due                                                                              *time.Time
}

// ReminderDetails accepts only references authorized by this module's service.
// The host rechecks live source eligibility after reading these projections.
func ReminderDetails(ctx context.Context, db *pgxpool.Pool, kind string, keys []string) ([]remindermessage.Item, error) {
	if len(keys) == 0 {
		return []remindermessage.Item{}, nil
	}
	var query string
	switch kind {
	case "loan_result":
		query = `SELECT c.id::text||':'||c.state key,jsonb_build_object('ref',c.public_id,'state',c.state,'name',p.name,'amount',c.principal_minor,'total',c.total_due_minor,'reason',c.review_note,'time',c.updated_at,'due',c.first_due_at) data FROM loan_cases c JOIN loan_pools p ON p.id=c.pool_id`
	case "guarantee_pending":
		query = `SELECT g.id::text||':'||g.state key,jsonb_build_object('ref',c.public_id,'state',g.state,'account',c.borrower_account_id,'amount',g.amount_minor,'reason',g.note,'time',g.created_at) data FROM loan_guarantees g JOIN loan_cases c ON c.id=g.case_id`
	case "loan_due":
		query = `SELECT i.id::text||':'||i.due_at::text key,jsonb_build_object('ref',c.public_id,'name',p.name,'sequence',i.sequence,'amount',i.principal_minor-i.paid_principal_minor+i.interest_minor-i.paid_interest_minor,'principal',i.principal_minor-i.paid_principal_minor,'interest',i.interest_minor-i.paid_interest_minor,'due',i.due_at) data FROM loan_installments i JOIN loan_cases c ON c.id=i.case_id JOIN loan_pools p ON p.id=c.pool_id`
	case "loan_overdue":
		query = `SELECT i.id::text||':'||i.due_at::text key,jsonb_build_object('ref',c.public_id,'name',p.name,'sequence',i.sequence,'amount',i.principal_minor-i.paid_principal_minor+i.interest_minor-i.paid_interest_minor,'principal',i.principal_minor-i.paid_principal_minor,'interest',i.interest_minor-i.paid_interest_minor,'due',i.due_at) data FROM loan_installments i JOIN loan_cases c ON c.id=i.case_id JOIN loan_pools p ON p.id=c.pool_id`
	case "collateral_pending":
		query = `SELECT r.id::text key,jsonb_build_object('ref',c.public_id,'delivery',r.public_id,'source',r.source_character_id,'recipient',r.recipient_character_id,'count',jsonb_array_length(r.items),'time',r.created_at) data FROM loan_collateral_requests r JOIN loan_cases c ON c.id=r.case_id`
	case "contribution_result":
		query = `SELECT c.id::text key,jsonb_build_object('ref',c.id::text,'name',p.name,'source',c.source_character_id,'contract',c.contract_id,'amount',c.amount_minor,'time',c.funded_at) data FROM loan_contributions c JOIN loan_pools p ON p.id=c.pool_id`
	case "loan_funding":
		query = `SELECT c.id::text key,jsonb_build_object('ref',c.public_id,'state',c.state,'account',c.borrower_account_id,'recipient',c.borrower_character_id,'name',p.name,'amount',c.principal_minor,'reason',c.review_note) data FROM loan_cases c JOIN loan_pools p ON p.id=c.pool_id WHERE c.state IN ('approved','funding')`
	default:
		return nil, errors.New("unsupported reminder detail")
	}
	rows, err := db.Query(ctx, "SELECT key, data FROM ("+query+") x WHERE key=ANY($1::text[]) ORDER BY key", keys)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []remindermessage.Item{}
	for rows.Next() {
		var key string
		var data []byte
		var r reminderDetail
		if err = rows.Scan(&key, &data); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(data, &r); err != nil {
			return nil, err
		}
		item := reminderDetailMessage(kind, key, r)
		out = append(out, item)
	}
	return out, rows.Err()
}
func reminderDetailMessage(kind, key string, r reminderDetail) remindermessage.Item {

	item := remindermessage.Item{Key: key, Fields: []remindermessage.Field{remindermessage.Text("贷款编号", r.Ref)}}
	switch kind {
	case "loan_result":
		state := remindermessage.Label(r.State, map[string]string{"awaiting_acceptance": "待确认条款", "approved": "已批准", "funding": "放款中", "active": "已放款", "settled": "已结清", "rejected": "已驳回", "cancelled": "已取消", "defaulted": "已违约", "disputed": "争议处理中"})
		item.Title = "贷款 · " + state
		item.Fields = append(item.Fields, remindermessage.Text("资金池", r.Name), remindermessage.Text("本金", remindermessage.Money(r.Amount, "ISK")), remindermessage.Text("本息合计", remindermessage.Money(r.Total, "ISK")), remindermessage.Text("处理意见", r.Reason), remindermessage.Text("更新时间", remindermessage.Date(r.Time)))
		item.Action = "请查看贷款详情与还款计划。"
		if r.State == "awaiting_acceptance" {
			item.Action = "请核对金额、利息和还款日期后确认条款。"
		}
		if r.Due != nil {
			item.Fields = append(item.Fields, remindermessage.Text("首期还款日", remindermessage.Date(*r.Due)))
		}
	case "loan_due", "loan_overdue":
		item.Title = "贷款即将到期"
		item.Action = "请按贷款详情中的还款方式偿还本期欠款。"
		if kind == "loan_overdue" {
			item.Title = "贷款已逾期"
			item.Action = "请尽快偿还本期欠款，并联系贷款办理人员。"
		}
		item.Fields = append(item.Fields, remindermessage.Text("资金池", r.Name), remindermessage.Text("期数", strconv.FormatInt(r.Sequence, 10)), remindermessage.Text("本期剩余应还", remindermessage.Money(r.Amount, "ISK")), remindermessage.Text("剩余本金", remindermessage.Money(r.Principal, "ISK")), remindermessage.Text("剩余利息", remindermessage.Money(r.Interest, "ISK")))
		if r.Due != nil {
			item.Fields = append(item.Fields, remindermessage.Text("到期时间", remindermessage.Date(*r.Due)))
		}
	case "guarantee_pending":
		item.Title = remindermessage.Label(r.State, map[string]string{"invited": "担保邀请", "called": "担保追偿", "disputed": "担保争议"})
		item.Fields = append(item.Fields, remindermessage.Person("借款人", r.Account), remindermessage.Text("担保金额", remindermessage.Money(r.Amount, "ISK")), remindermessage.Text("备注", r.Reason))
		item.Action = "请查看担保责任与当前状态后处理。"
	case "collateral_pending":
		item.Title = "待交付抵押物"
		item.Fields = append(item.Fields, remindermessage.Text("抵押交付编号", r.Delivery), remindermessage.Character("交付角色", r.Source), remindermessage.Character("接收角色", r.Recipient), remindermessage.Text("物品种类", strconv.FormatInt(r.Count, 10)))
		item.Action = "请按抵押详情创建游戏合同，交付已选物品；合同核验完成后才计入抵押。"
	case "contribution_result":
		item.Title = "出借资金已到账"
		item.Fields = []remindermessage.Field{remindermessage.Text("出借记录", r.Ref), remindermessage.Text("资金池", r.Name), remindermessage.Text("到账金额", remindermessage.Money(r.Amount, "ISK")), remindermessage.Character("出借角色", r.Source), remindermessage.Text("到账时间", remindermessage.Date(r.Time))}
		if r.Contract > 0 {
			item.Fields = append(item.Fields, remindermessage.Text("合同 ID", strconv.FormatInt(r.Contract, 10)))
		}
		item.Action = "请查看出借记录。"
	case "loan_funding":
		item.Title = "贷款待放款"
		item.Fields = append(item.Fields, remindermessage.Person("借款人", r.Account), remindermessage.Character("收款角色", r.Recipient), remindermessage.Text("资金池", r.Name), remindermessage.Text("贷款本金", remindermessage.Money(r.Amount, "ISK")), remindermessage.Text("处理意见", r.Reason))
		item.Action = "请核对已有付款和合同进度后办理放款。"
	}
	return item

}

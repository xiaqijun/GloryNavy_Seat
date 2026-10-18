package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/platform/remindermessage"
	"strings"
	"time"
)

type reminderDetail struct {
	Ref, Kind, State, Name, Recipient, Reason string
	Amount, Coins, Cash                       int64
	Time                                      time.Time
	Items, Fittings                           []reminderReward
}

type reminderReward struct {
	Name      string
	Quantity  int64
	TypeID    json.RawMessage `json:"type_id"`
	FittingID json.RawMessage `json:"fitting_id"`
}

// ReminderDetails accepts only references authorized by this module's service.
// The host rechecks live source eligibility after reading these projections.
func ReminderDetails(ctx context.Context, db *pgxpool.Pool, kind string, keys []string) ([]remindermessage.Item, error) {
	if len(keys) == 0 {
		return []remindermessage.Item{}, nil
	}
	var query string
	switch kind {
	case "welfare_result":
		query = `SELECT c.id::text||':'||c.state key,jsonb_build_object('ref',c.settlement_reference,'kind',c.kind,'state',c.state,'name',coalesce(nullif(c.detail#>>'{rule,project_name}',''),c.detail#>>'{loss_evidence,ship_name}'),'recipient',c.detail->>'character_name','amount',c.award_minor,'coins',c.detail#>'{rewards,coins_minor}','cash',c.detail#>'{rewards,isk_minor}','items',c.detail#>'{rewards,items}','fittings',c.detail#>'{rewards,fittings}','reason',(SELECT note FROM welfare_audit WHERE case_id=c.id AND action IN ('approve','reject','information','approve_cancel','reject_cancel','cancel','void') ORDER BY created_at DESC,id DESC LIMIT 1),'time',c.updated_at) data FROM welfare_cases c`
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

	label := remindermessage.Label(r.State, map[string]string{"information": "待补充资料", "approved": "已批准", "completed": "已发放", "rejected": "已驳回", "cancelled": "已取消", "reversed": "已冲正"})
	category := remindermessage.Label(r.Kind, map[string]string{"srp": "军团补损", "alliance": "联盟补损", "solo": "PVP 补损", "capital": "旗舰补贴", "supercarrier": "超级航母补贴", "titan": "泰坦补贴", "grant": "福利发放"})
	if strings.HasPrefix(r.Kind, "growth_") {
		category = "成长福利"
	}
	if strings.HasPrefix(r.Kind, "activity_") {
		category = "活动福利"
	}
	item := remindermessage.Item{Key: key, Title: category + " · " + label, Fields: []remindermessage.Field{remindermessage.Text("单号", r.Ref), remindermessage.Text("项目", r.Name), remindermessage.Text("领取角色", r.Recipient), remindermessage.Text("处理意见", r.Reason), remindermessage.Text("更新时间", remindermessage.Date(r.Time))}}
	if r.Amount > 0 && !strings.HasPrefix(r.Kind, "growth_") && !strings.HasPrefix(r.Kind, "activity_") && r.Kind != "grant" {
		item.Fields = append(item.Fields, remindermessage.Text("核准金额", remindermessage.Money(r.Amount, "ISK")))
	}
	if r.Coins > 0 {
		item.Fields = append(item.Fields, remindermessage.Text("果壳币", remindermessage.Money(r.Coins, "果壳币")))
	}
	if r.Kind == "grant" && r.Amount > 0 {
		item.Fields = append(item.Fields, remindermessage.Text("果壳币", remindermessage.Money(r.Amount, "果壳币")))
	}
	if r.Cash > 0 {
		item.Fields = append(item.Fields, remindermessage.Text("奖励 ISK", remindermessage.Money(r.Cash, "ISK")))
	}
	if summary := welfareReminderRewards(r.Items, r.Fittings); summary != "" {
		item.Fields = append(item.Fields, remindermessage.Text("实物奖励", summary))
	}
	switch r.State {
	case "information":
		item.Action = "请打开申请，补充资料后重新提交。"
	case "approved":
		item.Action = "请查看发放进度；涉及物品或 ISK 时，以实际交付结果为准。"
	case "rejected":
		item.Action = "请查看处理意见。"
	case "completed":
		item.Action = "请查看发放记录。"
	default:
		item.Action = "请查看申请记录。"
	}
	return item

}

func welfareReminderRewards(items, fittings []reminderReward) string {
	out := []string{}
	for _, item := range append(append([]reminderReward(nil), items...), fittings...) {
		name := item.Name
		if name == "" && len(item.TypeID) > 0 {
			name = "物品 #" + strings.Trim(string(item.TypeID), "\"")
		}
		if name == "" && len(item.FittingID) > 0 {
			name = "配装 #" + strings.Trim(string(item.FittingID), "\"")
		}
		if name != "" {
			out = append(out, fmt.Sprintf("%s × %d", name, item.Quantity))
		}
	}
	return strings.Join(out, "、")
}

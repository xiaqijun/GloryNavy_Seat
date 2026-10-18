package store

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/platform/remindermessage"
	"time"
)

type reminderDetail struct {
	Ref, State, Name, Recipient, Reason string
	Amount                              int64
	Time                                time.Time
}

// ReminderDetails accepts only references authorized by this module's service.
// The host rechecks live source eligibility after reading these projections.
func ReminderDetails(ctx context.Context, db *pgxpool.Pool, kind string, keys []string) ([]remindermessage.Item, error) {
	if len(keys) == 0 {
		return []remindermessage.Item{}, nil
	}
	var query string
	switch kind {
	case "exchange_result":
		query = `SELECT id::text||':'||state key,jsonb_build_object('ref',settlement_reference,'state',state,'name',reward_name,'recipient',recipient_name,'amount',coins_minor,'reason',note,'time',decided_at) data FROM exchange_redemptions`
	case "coin_change":
		query = `SELECT id::text key,jsonb_build_object('ref',reference,'state',kind,'amount',delta,'reason',reason,'time',created_at) data FROM exchange_coin_ledger`
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

	if kind == "coin_change" {
		return remindermessage.Item{Key: key, Title: "果壳币变动", Fields: []remindermessage.Field{remindermessage.Text("变动", remindermessage.Signed(r.Amount, "果壳币")), remindermessage.Text("原因", r.Reason), remindermessage.Text("发生时间", remindermessage.Date(r.Time))}, Action: "请查看果壳币流水。"}
	}
	state := remindermessage.Label(r.State, map[string]string{"fulfilled": "已完成", "cancelled": "已取消"})
	amountLabel := "兑换花费"
	action := "请查看兑换与交付记录。"
	if r.State == "cancelled" {
		amountLabel = "退回果壳币"
		action = "订单已取消，请查看退款流水。"
	}
	return remindermessage.Item{Key: key, Title: "兑换" + state, Fields: []remindermessage.Field{remindermessage.Text("单号", r.Ref), remindermessage.Text("奖励", r.Name), remindermessage.Text("领取角色", r.Recipient), remindermessage.Text(amountLabel, remindermessage.Money(r.Amount, "果壳币")), remindermessage.Text("处理意见", r.Reason), remindermessage.Text("处理时间", remindermessage.Date(r.Time))}, Action: action}

}

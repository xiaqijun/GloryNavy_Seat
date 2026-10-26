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
	Ref, Name, Kind, State, EventState, Author, Reason string
	Time                                               time.Time
}

// ReminderDetails accepts only references authorized by this module's service.
// The host rechecks live source eligibility after reading these projections.
func ReminderDetails(ctx context.Context, db *pgxpool.Pool, kind string, keys []string) ([]remindermessage.Item, error) {
	if len(keys) == 0 {
		return []remindermessage.Item{}, nil
	}
	var query string
	switch kind {
	case "feedback_update":
		query = `SELECT m.id::text key,jsonb_build_object('ref',t.id::text,'name',t.title,'kind',t.kind,'state',t.status,'eventstate',m.status,'author',m.author_name,'reason',m.body,'time',m.created_at) data FROM feedback_messages m JOIN feedback_tickets t ON t.id=m.ticket_id`
	case "feedback_pending":
		query = `SELECT CASE WHEN m.previous_status IN ('awaiting_confirmation','resolved','closed','withdrawn') THEN 'returned:'||m.id::text ELSE t.id::text END key,
 jsonb_build_object('ref',t.id::text,'name',t.title,'kind',t.kind,'state',t.status,
 'author',CASE WHEN m.previous_status IN ('awaiting_confirmation','resolved','closed','withdrawn') THEN m.author_name ELSE t.author_name END,
 'reason',CASE WHEN m.previous_status IN ('awaiting_confirmation','resolved','closed','withdrawn') THEN m.body ELSE t.body END,
 'time',CASE WHEN m.previous_status IN ('awaiting_confirmation','resolved','closed','withdrawn') THEN m.created_at ELSE t.created_at END) data
 FROM feedback_tickets t LEFT JOIN LATERAL (
 SELECT id,author_name,body,created_at,lag(status) OVER(ORDER BY id) previous_status FROM feedback_messages WHERE ticket_id=t.id AND status<>'' ORDER BY id DESC LIMIT 1
 ) m ON true WHERE t.status IN ('submitted','in_progress')`
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

	category := remindermessage.Label(r.Kind, map[string]string{"bug": "BUG", "feature": "功能需求"})
	state := remindermessage.Label(r.State, map[string]string{"submitted": "待处理", "in_progress": "处理中", "accepted": "已采纳", "awaiting_confirmation": "待确认", "resolved": "已完成", "closed": "已关闭", "withdrawn": "已撤销"})
	item := remindermessage.Item{Key: key, Title: category + " #" + r.Ref + " · " + r.Name, Fields: []remindermessage.Field{remindermessage.Text("当前状态", state)}}
	if kind == "feedback_update" {
		item.Fields = append(item.Fields, remindermessage.Text("回复人", r.Author), remindermessage.Text("回复", r.Reason), remindermessage.Text("回复时间", remindermessage.Date(r.Time)))
		if r.EventState != "" {
			item.Fields = append(item.Fields, remindermessage.Text("本次变更", remindermessage.Label(r.EventState, map[string]string{"submitted": "待处理", "in_progress": "处理中", "accepted": "已采纳", "awaiting_confirmation": "待确认", "resolved": "已完成", "closed": "已关闭", "withdrawn": "已撤销"})))
		}
		item.Action = "请查看完整回复与处理进度。"
	} else {
		item.Fields = append(item.Fields, remindermessage.Text("提交人", r.Author), remindermessage.Text("内容", r.Reason), remindermessage.Text("提交时间", remindermessage.Date(r.Time)))
		item.Action = "请查看反馈并回复或更新处理状态。"
	}
	return item

}

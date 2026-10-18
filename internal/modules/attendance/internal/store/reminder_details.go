package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/platform/remindermessage"
	"time"
)

type reminderDetail struct {
	Name, Recipient, Reason string
	Delta                   int64
	Time                    time.Time
}

// ReminderDetails accepts only references authorized by this module's service.
// The host rechecks live source eligibility after reading these projections.
func ReminderDetails(ctx context.Context, db *pgxpool.Pool, kind string, keys []string) ([]remindermessage.Item, error) {
	if len(keys) == 0 {
		return []remindermessage.Item{}, nil
	}
	var query string
	switch kind {
	case "pap_change":
		query = `SELECT l.id::text key,jsonb_build_object('name',e.title,'recipient',a.character_name,'delta',l.delta,'reason',l.reason,'time',l.created_at) data FROM attendance_pap_ledger l JOIN attendance_events e ON e.id=l.event_id JOIN attendance_pap_awards a ON a.event_id=l.event_id AND a.character_id=l.character_id`
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

	return remindermessage.Item{Key: key, Title: "PAP 变动", Fields: []remindermessage.Field{remindermessage.Text("活动", r.Name), remindermessage.Text("参战角色", r.Recipient), remindermessage.Text("变动", fmt.Sprintf("%+d PAP", r.Delta)), remindermessage.Text("原因", r.Reason), remindermessage.Text("发生时间", remindermessage.Date(r.Time))}, Action: "请查看军团 PAP 流水。"}

}

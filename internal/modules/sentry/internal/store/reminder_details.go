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
	Name, Reason string
	Time         time.Time
}

// ReminderDetails accepts only references authorized by this module's service.
// The host rechecks live source eligibility after reading these projections.
func ReminderDetails(ctx context.Context, db *pgxpool.Pool, kind string, keys []string) ([]remindermessage.Item, error) {
	if len(keys) == 0 {
		return []remindermessage.Item{}, nil
	}
	var query string
	switch kind {
	case "sentry_alert":
		query = `SELECT id::text||':'||updated_at::text key,jsonb_build_object('name',name,'reason',last_error,'time',updated_at) data FROM sentry_keys WHERE status='sync_error' `
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
	reason := "预警服务状态同步失败"
	switch r.Reason {
	case "预警平台未确认创建结果", "预警平台未确认密钥更新结果", "预警平台已更新，本地记录待核对", "预警平台未确认吊销结果":
		reason = r.Reason
	}
	return remindermessage.Item{Key: key, Title: "预警服务同步异常", Fields: []remindermessage.Field{remindermessage.Text("密钥名称", r.Name), remindermessage.Text("异常开始时间", remindermessage.Date(r.Time)), remindermessage.Text("原因", reason)}, Action: "请打开预警服务，查看状态并重新同步。"}

}

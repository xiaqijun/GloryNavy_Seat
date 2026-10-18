package store

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/platform/remindermessage"
)

type reminderDetail struct {
	Name, Title, Kind, Resource, Reason     string
	ID, Failures, Price, Reward, Collateral int64
	Due, Success, Updated                   *time.Time
}

// Explicit safe columns only: no credentials, payload dumps or raw upstream errors.
func ReminderDetails(ctx context.Context, db *pgxpool.Pool, kind string, keys []string, characterID int64) ([]remindermessage.Item, error) {
	if len(keys) == 0 {
		return []remindermessage.Item{}, nil
	}
	var query string
	switch kind {
	case "authorization_expired":
		query = `SELECT c.character_id::text||':'||c.grant_generation::text key,jsonb_build_object('id',c.character_id,'name',p.name) data FROM eve_credentials c LEFT JOIN eve_character_profiles p ON p.character_id=c.character_id WHERE c.character_id=$2`
	case "contract_pending":
		query = `SELECT x.contract_id::text key,jsonb_build_object('id',x.contract_id,'title',x.payload->>'title','kind',x.contract_type,'due',(x.payload->>'date_expired')::timestamptz,'price',(coalesce((x.payload->>'price')::numeric,0)*100)::bigint,'reward',(coalesce((x.payload->>'reward')::numeric,0)*100)::bigint,'collateral',(coalesce((x.payload->>'collateral')::numeric,0)*100)::bigint) data FROM eve_contracts x WHERE x.owner_kind='character' AND x.owner_id=$2`
	case "sync_anomaly":
		query = `SELECT t.id::text||':'||t.generation::text||':'||coalesce(t.last_success_at::text,'initial') key,jsonb_build_object('id',t.character_id,'name',p.name,'resource',t.resource,'reason',t.reason,'failures',t.failures,'success',t.last_success_at,'updated',t.updated_at) data FROM eve_sync_targets t LEFT JOIN eve_character_profiles p ON p.character_id=t.character_id WHERE $2::bigint=0`
	default:
		return nil, errors.New("unsupported reminder detail")
	}
	rows, err := db.Query(ctx, "SELECT key,data FROM ("+query+") x WHERE key=ANY($1::text[]) ORDER BY key", keys, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []remindermessage.Item{}
	for rows.Next() {
		var key string
		var raw []byte
		var r reminderDetail
		if err = rows.Scan(&key, &raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &r); err != nil {
			return nil, err
		}
		out = append(out, eveReminderMessage(kind, key, r))
	}
	return out, rows.Err()
}

func eveReminderMessage(kind, key string, r reminderDetail) remindermessage.Item {
	item := remindermessage.Item{Key: key}
	if kind == "contract_pending" {
		item.Title = "待领取合同"
		item.Fields = []remindermessage.Field{remindermessage.Text("合同 ID", strconv.FormatInt(r.ID, 10)), remindermessage.Text("标题", r.Title), remindermessage.Text("类型", remindermessage.Label(r.Kind, map[string]string{"item_exchange": "物品交换", "courier": "快递", "auction": "拍卖"})), remindermessage.Text("价格", remindermessage.Money(r.Price, "ISK")), remindermessage.Text("报酬", remindermessage.Money(r.Reward, "ISK"))}
		if r.Collateral > 0 {
			item.Fields = append(item.Fields, remindermessage.Text("抵押金", remindermessage.Money(r.Collateral, "ISK")))
		}
		if r.Due != nil {
			item.Fields = append(item.Fields, remindermessage.Text("到期时间", remindermessage.Date(*r.Due)))
		}
		item.Action = "请在游戏内核对物品、金额和合同条件后接受合同。"
		return item
	}
	name := r.Name
	if name == "" {
		name = "角色 #" + strconv.FormatInt(r.ID, 10)
	}
	item.Fields = []remindermessage.Field{remindermessage.Text("角色", name)}
	if kind == "authorization_expired" {
		item.Title = "角色需要重新授权"
		item.Fields = append(item.Fields, remindermessage.Text("状态", "授权已失效，相关数据无法继续同步"))
		item.Action = "请进入个人账户，选择该角色并重新完成 EVE 授权。"
		return item
	}
	item.Title = "数据同步持续失败"
	resource := remindermessage.Label(r.Resource, map[string]string{"character_contracts": "个人合同", "corporation_contracts": "军团合同", "character_wallet_journal": "个人钱包日志", "character_wallet_transactions": "个人钱包交易", "corporation_wallet_journal": "军团钱包日志", "corporation_wallet_transactions": "军团钱包交易", "skills": "技能", "skillqueue": "技能队列", "assets": "个人资产", "corporation_assets": "军团资产", "online": "在线状态", "roles": "军团职务", "profile": "角色资料", "killmails": "舰船损失", "structures": "军团建筑", "fittings": "配装"})
	if resource == "待查看" {
		resource = r.Resource
	}
	reason := remindermessage.Label(r.Reason, map[string]string{"upstream_unavailable": "ESI 服务暂不可用", "rate_limited": "ESI 请求限流", "forbidden": "权限不足", "scope_missing": "缺少授权权限", "authorization_pending": "等待角色授权", "corporation_changed": "角色军团已变更"})
	if reason == "待查看" {
		reason = "同步未成功，请查看同步管理记录"
	}
	item.Fields = append(item.Fields, remindermessage.Text("同步项目", resource), remindermessage.Text("连续失败次数", strconv.FormatInt(r.Failures, 10)), remindermessage.Text("原因", reason))
	if r.Success != nil {
		item.Fields = append(item.Fields, remindermessage.Text("上次成功", remindermessage.Date(*r.Success)))
	} else {
		item.Fields = append(item.Fields, remindermessage.Text("上次成功", "尚无成功记录"))
	}
	item.Action = "请进入 EVE 同步管理，查看失败原因并处理。"
	return item
}

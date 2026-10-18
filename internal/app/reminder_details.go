package app

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"time"

	"glorynavy.local/seat/internal/modules/access"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/modules/reminder"
	"glorynavy.local/seat/internal/modules/structures"
	"glorynavy.local/seat/internal/platform/remindermessage"
)

func wireEveReminderDetails(s *reminder.Service, accounts *identity.Service, policy *access.Service, auth *eve.AuthorizationService, contracts *eve.ContractHTTP, buildings structures.Handler, enabled []string) {
	if s.Details == nil {
		s.Details = map[string]reminder.DetailSource{}
	}
	for _, kind := range []string{"authorization_expired", "contract_pending"} {
		s.Details[kind] = func(ctx context.Context, user string, _ time.Time) ([]remindermessage.Item, error) {
			chars, err := accounts.ActiveCharacters(ctx, user)
			if err != nil {
				return nil, err
			}
			out := []remindermessage.Item{}
			for _, ch := range chars {
				var items []remindermessage.Item
				if kind == "authorization_expired" {
					items, err = auth.AuthorizationReminderDetails(ctx, ch.ID, ch.OwnerHash)
				} else {
					items, err = contracts.ContractReminderDetails(ctx, user, ch.ID, ch.OwnerHash)
				}
				if err != nil {
					return nil, err
				}
				for i := range items {
					if kind == "contract_pending" {
						items[i].Fields = append(items[i].Fields, remindermessage.Text("领取角色", ch.Name))
					} else {
						items[i].Fields[0] = remindermessage.Text("角色", ch.Name)
					}
				}
				out = append(out, items...)
			}
			return out, nil
		}
	}
	s.Details["sync_anomaly"] = func(ctx context.Context, user string, _ time.Time) ([]remindermessage.Item, error) {
		ok, err := policy.IsAdministrator(ctx, user)
		if err != nil {
			return nil, err
		}
		if !ok {
			return []remindermessage.Item{}, nil
		}
		return auth.SyncReminderDetails(ctx)
	}
	if slices.Contains(enabled, "structures") {
		s.Details["structure_alert"] = func(ctx context.Context, user string, _ time.Time) ([]remindermessage.Item, error) {
			rows, err := buildings.Read(ctx, user, 0)
			if err != nil {
				return nil, err
			}
			return structureReminderDetails(rows, time.Now().UTC()), nil
		}
	}
}

func structureReminderDetails(rows []eve.Structure, now time.Time) []remindermessage.Item {
	out := []remindermessage.Item{}
	for _, row := range rows {
		name := row.Name
		if name == "" {
			name = row.TypeName
		}
		if name == "" {
			name = "建筑 #" + strconv.FormatInt(row.ID, 10)
		}
		corp := row.CorporationName
		if corp == "" {
			corp = strconv.FormatInt(row.CorporationID, 10)
		}
		system := row.SolarSystemName
		if system == "" {
			system = strconv.FormatInt(row.SolarSystemID, 10)
		}
		fields := []remindermessage.Field{remindermessage.Text("建筑", name), remindermessage.Text("建筑 ID", strconv.FormatInt(row.ID, 10)), remindermessage.Text("军团", corp), remindermessage.Text("星系", system), remindermessage.Text("数据时间", remindermessage.Date(row.ObservedAt))}
		if row.FuelExpires != nil {
			stage, title := "", ""
			left := row.FuelExpires.Sub(now)
			switch {
			case left <= 0:
				stage, title = "empty", "建筑燃料已到预计耗尽时间"
			case left <= 24*time.Hour:
				stage, title = "24h", "建筑燃料不足 24 小时"
			case left <= 72*time.Hour:
				stage, title = "72h", "建筑燃料不足 72 小时"
			}
			if stage != "" {
				f := append([]remindermessage.Field(nil), fields...)
				f = append(f, remindermessage.Text("预计耗尽时间", remindermessage.Date(*row.FuelExpires)))
				out = append(out, remindermessage.Item{Key: fmt.Sprintf("%d:fuel:%s:%s", row.ID, row.FuelExpires.UTC().Format("2006-01-02"), stage), Title: title, Fields: f, Action: "请核对游戏内燃料状态并及时补充。"})
			}
		}
		if row.UnanchorsAt != nil && row.UnanchorsAt.After(now) {
			f := append([]remindermessage.Field(nil), fields...)
			f = append(f, remindermessage.Text("拆除时间", remindermessage.Date(*row.UnanchorsAt)))
			out = append(out, remindermessage.Item{Key: fmt.Sprintf("%d:unanchor:%s", row.ID, row.UnanchorsAt.UTC().Format(time.RFC3339)), Title: "建筑正在拆除", Fields: f, Action: "请确认拆除安排并及时处理建筑内资产。"})
		}
	}
	return out
}

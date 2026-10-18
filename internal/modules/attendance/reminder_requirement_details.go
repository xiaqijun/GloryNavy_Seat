package attendance

import (
	"context"
	"fmt"
	"glorynavy.local/seat/internal/platform/remindermessage"
	"time"
)

func (s *Service) RequirementReminderDetails(ctx context.Context, account string, _ time.Time) ([]remindermessage.Item, error) {
	now := time.Now().UTC()
	last := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	if last.Sub(now) > 7*24*time.Hour {
		return []remindermessage.Item{}, nil
	}
	rule, err := s.PAPRequirement(ctx, account)
	if err != nil {
		return nil, err
	}
	if rule.MonthlyPoints <= 0 {
		return []remindermessage.Item{}, nil
	}
	report, err := s.AlliancePAPReport(ctx, account)
	if err != nil {
		return nil, err
	}
	return requirementReminderDetails(report, rule.MonthlyPoints, now)
}

func requirementReminderDetails(report AlliancePAPReport, target int32, now time.Time) ([]remindermessage.Item, error) {
	last := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	keys, err := requirementReminderKeys(report, target, now)
	if err != nil {
		return nil, err
	}
	out := []remindermessage.Item{}
	for _, key := range keys {
		out = append(out, remindermessage.Item{Key: key, Title: "本月 PAP 尚未达标", Fields: []remindermessage.Field{remindermessage.Text("月份", report.Month), remindermessage.Text("当前 PAP", fmt.Sprintf("%g", report.Points)), remindermessage.Text("目标 PAP", fmt.Sprintf("%d", target)), remindermessage.Text("还差 PAP", fmt.Sprintf("%g", float64(target)-report.Points)), remindermessage.Text("截止时间", remindermessage.Date(last)), remindermessage.Text("数据时间", remindermessage.Date(*report.LastSyncedAt))}, Action: "请在本月结束前完成 PAP 要求。"})
	}
	return out, nil
}

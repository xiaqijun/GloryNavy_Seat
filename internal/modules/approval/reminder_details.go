package approval

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/platform/remindermessage"
	"glorynavy.local/seat/internal/platform/reviewqueue"
)

// PendingNotificationDetails uses live source authorization, never index/cache
// access or stored approval payloads from a previous recipient.
func (s *Service) PendingNotificationDetails(ctx context.Context, user string, _ time.Time) ([]remindermessage.Item, error) {
	live := &Service{Sources: s.Sources, Names: s.Names}
	out := []remindermessage.Item{}
	token := ""
	for {
		page, err := live.listLegacy(ctx, user, reviewqueue.Filter{View: "pending", Sort: "id_asc"}, token)
		if errors.Is(err, pgx.ErrNoRows) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		if len(page.Unavailable) > 0 {
			return nil, errors.New("approval notification source unavailable")
		}
		for _, row := range page.Items {
			if row.Account == user || !slices.ContainsFunc(row.Actions, func(a string) bool { return a == "approve" || a == "approve_cancel" || a == "cancelled" }) {
				continue
			}
			out = append(out, approvalReminderMessage(row))
		}
		if page.Next == "" {
			break
		}
		if page.Next == token {
			return nil, errors.New("approval notification cursor did not advance")
		}
		token = page.Next
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func approvalReminderMessage(row reviewqueue.Item) remindermessage.Item {
	kind := remindermessage.Label(row.Kind, map[string]string{"loan": "贷款申请", "srp": "军团补损", "solo": "PVP 补损", "supercarrier": "超级航母补贴", "titan": "泰坦补贴"})
	if row.Source == "exchange" {
		kind = "兑换取消申请"
	}
	if strings.HasPrefix(row.Kind, "growth_") {
		kind = "成长福利"
	}
	if strings.HasPrefix(row.Kind, "activity_") {
		kind = "活动福利"
	}
	phase := "待审批"
	if slices.Contains(row.Actions, "approve_cancel") {
		phase = "待审核取消"
	}
	ref := row.Reference
	if ref == "" {
		ref = fmt.Sprintf("#%d", row.ID)
	}
	item := remindermessage.Item{
		Key:    fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s/%d/%s", row.Source, row.ID, row.Status)))),
		Title:  kind + " · " + phase,
		Fields: []remindermessage.Field{remindermessage.Text("单号", ref), remindermessage.Person("申请人", row.Account), remindermessage.Text("事项", row.Title), remindermessage.Text("领取角色", row.Recipient), remindermessage.Text("申请时间", remindermessage.Date(row.Time))},
		Action: "请进入审批中心核对申请并处理。",
	}
	if row.Amount > 0 {
		switch strings.ToLower(row.Unit) {
		case "isk":
			item.Fields = append(item.Fields, remindermessage.Text("金额", remindermessage.Money(row.Amount, "ISK")))
		case "coin", "coins":
			item.Fields = append(item.Fields, remindermessage.Text("果壳币", remindermessage.Money(row.Amount, "果壳币")))
		}
	}
	return item
}

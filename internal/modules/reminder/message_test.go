package reminder

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"glorynavy.local/seat/internal/platform/remindermessage"
)

func TestMailEscapingLengthAndExactValues(t *testing.T) {
	item := remindermessage.Item{Title: "贷款 <test>", Fields: []remindermessage.Field{remindermessage.Text("单号", "LN-20261018-000001"), remindermessage.Text("应还", remindermessage.Money(123456789001, "ISK")), remindermessage.Text("意见", "<a href=\"https://evil.invalid\">fake</a>\n说明")}, Action: "请查看详情。"}
	body, err := renderMessage("贷款到期", "https://example.com/loans", []remindermessage.Item{item})
	if err != nil || !strings.Contains(body, "1,234,567,890.01 ISK") || !strings.Contains(body, "LN-20261018-000001") || strings.Contains(body, "<a href=\"https://evil") || strings.Count(body, "<a href=") != 1 {
		t.Fatal(body, err)
	}
	items := make([]remindermessage.Item, mailBatchSize)
	for i := range items {
		items[i] = remindermessage.Item{Title: strings.Repeat("<>&\"中", 100), Action: strings.Repeat("<>&", 200)}
		for j := 0; j < 12; j++ {
			items[i].Fields = append(items[i].Fields, remindermessage.Text(strings.Repeat("<>&", 100), strings.Repeat("<>&\"中", 2000)))
		}
	}
	body, err = renderMessage("测试", "https://example.com/loans", items)
	if err != nil || utf8.RuneCountInString(body) > 9500 || !strings.HasSuffix(body, "查看详情</a>") {
		t.Fatal(utf8.RuneCountInString(body), err)
	}
}

func TestMailNamesUseSelectedReferencesAndFallbackWithoutUUIDs(t *testing.T) {
	s := &Service{Origin: "https://example.com", Details: map[string]DetailSource{}}
	s.Details["loan_funding"] = func(context.Context, string, time.Time) ([]remindermessage.Item, error) {
		return []remindermessage.Item{
			{Key: "a", Title: "待放款", Fields: []remindermessage.Field{remindermessage.Person("申请人", "internal-account"), remindermessage.Character("收款角色", 123), remindermessage.Character("交付角色", 456)}},
			{Key: "not-due", Title: "not-due", Fields: []remindermessage.Field{remindermessage.Character("不应解析", 999)}},
		}, nil
	}
	s.Names = func(_ context.Context, ids []string) (map[string]string, error) {
		if len(ids) != 1 || ids[0] != "internal-account" {
			t.Fatal(ids)
		}
		return map[string]string{"internal-account": "成员名"}, nil
	}
	s.CharacterNames = func(_ context.Context, ids []int64) map[int64]string {
		if len(ids) != 2 || ids[0] != 123 || ids[1] != 456 {
			t.Fatal(ids)
		}
		return map[int64]string{123: "收款小号"}
	}
	event, _ := eventByID("loan_funding")
	body, err := s.message(context.Background(), "recipient", event, time.Now(), []string{detailKey("loan_funding", "a")})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"成员名", "收款小号", "角色 #456", "https://example.com/loans"} {
		if !strings.Contains(body, want) {
			t.Fatal(want, body)
		}
	}
	if strings.Contains(body, "internal-account") || strings.Contains(body, "not-due") {
		t.Fatal(body)
	}
}

func TestDetailBatchesOnlyReceiptListedItems(t *testing.T) {
	s, admin, u := fixture(t)
	ctx := context.Background()
	keys := []string{}
	for i := 0; i < 7; i++ {
		keys = append(keys, fmt.Sprintf("item-%d", i))
	}
	s.Pending = func(context.Context, string) ([]string, error) { return keys, nil }
	s.Names = func(context.Context, []string) (map[string]string, error) {
		return map[string]string{"private-account": "收件人"}, nil
	}
	s.Details["approval_pending"] = func(context.Context, string, time.Time) ([]remindermessage.Item, error) {
		out := []remindermessage.Item{}
		for _, key := range keys {
			out = append(out, remindermessage.Item{Key: key, Title: key, Fields: []remindermessage.Field{remindermessage.Person("申请人", "private-account")}})
		}
		// An authorized but already-notified or unrelated projection is not rendered.
		out = append(out, remindermessage.Item{Key: "not-due", Title: "SHOULD-NOT-BE-SENT"})
		return out, nil
	}
	bodies := []string{}
	s.Send = func(_ context.Context, _ Character, _ int64, _ string, body string) Result {
		bodies = append(bodies, body)
		return Result{State: "sent", ID: int64(len(bodies))}
	}
	if err := s.deliver(ctx, u); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE reminder_deliveries SET created_at=now()-interval '6 minutes'`); err != nil {
		t.Fatal(err)
	}
	if err := s.deliver(ctx, u); err != nil {
		t.Fatal(err)
	}
	if len(bodies) != 2 {
		t.Fatal(len(bodies))
	}
	for _, key := range keys {
		if strings.Count(strings.Join(bodies, ""), key) != 1 {
			t.Fatal("missing or duplicate item", key, bodies)
		}
	}
	for _, body := range bodies {
		if strings.Contains(body, "private-account") || strings.Contains(body, "SHOULD-NOT-BE-SENT") || !strings.Contains(body, "收件人") {
			t.Fatal(body)
		}
	}
	rows, err := s.Deliveries(ctx, admin, "")
	if err != nil || len(rows) != 2 || rows[0].Count+rows[1].Count != 7 {
		t.Fatal(rows, err)
	}
}

func TestDetailsFailureAndRevokedPermissionNeverSend(t *testing.T) {
	for _, mode := range []string{"missing", "duplicate", "revoked", "recipient-changed"} {
		t.Run(mode, func(t *testing.T) {
			s, _, u := fixture(t)
			ctx := context.Background()
			sent := false
			s.Pending = func(context.Context, string) ([]string, error) { return []string{"one"}, nil }
			s.Details["approval_pending"] = func(context.Context, string, time.Time) ([]remindermessage.Item, error) {
				items := []remindermessage.Item{{Key: "one", Title: "private detail"}}
				switch mode {
				case "missing":
					return nil, nil
				case "duplicate":
					return append(items, items[0]), nil
				case "revoked":
					s.Pending = func(context.Context, string) ([]string, error) { return nil, nil }
				case "recipient-changed":
					s.Main = func(context.Context, string) (Character, error) { return Character{ID: 303, Owner: []byte("new")}, nil }
				}
				return items, nil
			}
			s.Send = func(context.Context, Character, int64, string, string) Result {
				sent = true
				return Result{State: "sent", ID: 1}
			}
			if err := s.deliver(ctx, u); err != nil || sent {
				t.Fatal(err, sent)
			}
			var n int
			if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM reminder_deliveries WHERE state='sent'`).Scan(&n); err != nil || n != 0 {
				t.Fatal(n, err)
			}
		})
	}
}

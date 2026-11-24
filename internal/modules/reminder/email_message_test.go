package reminder

import (
	"strings"
	"testing"

	"glorynavy.local/seat/internal/platform/remindermessage"
)

func TestEmailMessageRendersEscapedDetails(t *testing.T) {
	items := []remindermessage.Item{
		{Title: "贷款 <备注>", Fields: []remindermessage.Field{remindermessage.Text("应还", remindermessage.Money(123456789001, "ISK"))}},
		{Title: "第二项", Fields: []remindermessage.Field{remindermessage.Text("未填写", "")}, Action: "请查看合同。"},
	}
	body, err := renderEmailMessage("贷款到期", "https://example.com/loans?a=1&b=2", items)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"荣耀海军 SEAT", "共 2 项", "贷款 &lt;备注&gt;", "1,234,567,890.01 ISK", "第二项", "请查看合同。", `href="https://example.com/loans?a=1&amp;b=2"`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(body, "<备注>") || strings.Contains(body, "未填写") || strings.Count(body, "<h2 ") != 2 {
		t.Fatalf("unexpected markup or rows: %s", body)
	}
}

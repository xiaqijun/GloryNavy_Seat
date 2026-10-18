package approval

import (
	"fmt"
	"strings"
	"testing"

	"glorynavy.local/seat/internal/platform/reviewqueue"
)

func TestApprovalReminderKindsAndMoneyUnits(t *testing.T) {
	for kind, want := range map[string]string{"alliance": "联盟补损", "capital": "旗舰补贴", "supercarrier": "超级航母补贴", "titan": "泰坦补贴", "growth_fitting_1": "成长福利", "activity_1": "活动福利", "loan": "贷款申请"} {
		row := reviewqueue.Item{Source: "welfare", Kind: kind, ID: 1, State: "submitted", Status: "submitted", Actions: []string{"approve"}, Amount: 123, Unit: "ISK"}
		item := approvalReminderMessage(row)
		if !strings.Contains(item.Title, want) || !strings.Contains(fmt.Sprint(item.Fields), "1.23 ISK") {
			t.Fatal(kind, item)
		}
	}
	row := reviewqueue.Item{Source: "exchange", Kind: "exchange", ID: 2, Amount: 123, Unit: "coin", Actions: []string{"cancelled"}}
	item := approvalReminderMessage(row)
	if !strings.Contains(item.Title, "兑换取消申请") || !strings.Contains(fmt.Sprint(item.Fields), "1.23 果壳币") {
		t.Fatal(item)
	}
	row.Unit = "reward"
	row.Amount = 0
	item = approvalReminderMessage(row)
	if strings.Contains(fmt.Sprint(item.Fields), "0.00") {
		t.Fatal("unknown/physical reward became zero cash", item)
	}
}

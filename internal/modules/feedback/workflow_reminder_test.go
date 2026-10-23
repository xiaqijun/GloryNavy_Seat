package feedback_test

import (
	"context"
	"fmt"
	"glorynavy.local/seat/internal/modules/feedback"
	"strings"
	"testing"
	"time"
)

func TestFeedbackWorkflowReturnReminderIsStable(t *testing.T) {
	s, _, a := fixture(t)
	ctx := context.Background()
	since := time.Now().Add(-time.Hour)
	v, err := s.Create(ctx, a[0], input(701))
	if err != nil {
		t.Fatal(err)
	}
	var id int64
	fmt.Sscan(v.ID, &id)
	keys, err := s.ReminderKeys(ctx, a[2].ID, "feedback_pending", since)
	if err != nil || len(keys) != 1 || keys[0] != v.ID {
		t.Fatal(keys, err)
	}
	for i, status := range []string{"accepted", "in_progress", "awaiting_confirmation", "in_progress"} {
		actor := a[2]
		body := "已完成"
		if i == 3 {
			actor = a[0]
			body = "退回原因"
		}
		v, err = s.Reply(ctx, actor, id, feedback.Reply{RequestKey: input(710 + i).RequestKey, Revision: v.Revision, Status: status, Body: body})
		if err != nil {
			t.Fatal(status, err)
		}
	}
	keys, err = s.ReminderKeys(ctx, a[2].ID, "feedback_pending", since)
	if err != nil || len(keys) != 1 || !strings.HasPrefix(keys[0], "returned:") {
		t.Fatal(keys, err)
	}
	before := keys[0]
	items, err := s.ReminderDetails(ctx, a[2].ID, "feedback_pending", since)
	if err != nil || len(items) != 1 {
		t.Fatal(items, err)
	}
	_, err = s.Reply(ctx, a[0], id, feedback.Reply{RequestKey: input(720).RequestKey, Revision: v.Revision, Body: "另补一条说明"})
	if err != nil {
		t.Fatal(err)
	}
	keys, err = s.ReminderKeys(ctx, a[2].ID, "feedback_pending", since)
	if err != nil || len(keys) != 1 || keys[0] != before {
		t.Fatal(keys, err)
	}
	keys, err = s.ReminderKeys(ctx, a[1].ID, "feedback_pending", since)
	if err != nil || len(keys) != 0 {
		t.Fatal(keys, err)
	}
}

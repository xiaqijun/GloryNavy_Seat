package feedback_test

import (
	"context"
	"fmt"
	"glorynavy.local/seat/internal/modules/feedback"
	"strings"
	"testing"
	"time"
)

func TestFeedbackWorkflowResubmitAndReopenReminders(t *testing.T) {
	s, _, a := fixture(t)
	ctx := context.Background()
	since := time.Now().Add(-time.Hour)
	v, err := s.Create(ctx, a[0], input(801))
	if err != nil {
		t.Fatal(err)
	}
	var id int64
	fmt.Sscan(v.ID, &id)
	var first string
	for i, status := range []string{"closed", "submitted", "accepted", "in_progress", "awaiting_confirmation", "resolved", "in_progress"} {
		actor := a[2]
		if status == "submitted" || status == "resolved" || i == 6 {
			actor = a[0]
		}
		v, err = s.Reply(ctx, actor, id, feedback.Reply{RequestKey: input(810 + i).RequestKey, Revision: v.Revision, Status: status, Body: "处理说明"})
		if err != nil {
			t.Fatal(status, err)
		}
		if i == 1 || i == 6 {
			keys, e := s.ReminderKeys(ctx, a[2].ID, "feedback_pending", since)
			if e != nil || len(keys) != 1 || !strings.HasPrefix(keys[0], "returned:") {
				t.Fatal(keys, e)
			}
			if i == 1 {
				first = keys[0]
			} else if keys[0] == first {
				t.Fatal("new action reused old reminder identity")
			}
		}
	}
}

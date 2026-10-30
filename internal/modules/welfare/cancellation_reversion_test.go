package welfare

import (
	"context"
	"encoding/json"
	"testing"
)

func TestRevertRejectedCancellationKeepsOriginalApproval(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	setQuota(t, s, Config{})
	c := quotaPending(t, s, 32201)
	raw, err := s.Execute(ctx, adminID, quotaApprove(c, 12000, 32201))
	if err != nil {
		t.Fatal(err)
	}
	var approved Case
	if err = json.Unmarshal(raw, &approved); err != nil {
		t.Fatal(err)
	}
	raw, err = s.Execute(ctx, userID, Command{Action: "request_cancel", ID: approved.ID, Version: approved.Version, RequestKey: key(32202), Note: "cancel"})
	if err != nil {
		t.Fatal(err)
	}
	var requested Case
	if err = json.Unmarshal(raw, &requested); err != nil {
		t.Fatal(err)
	}
	raw, err = s.Execute(ctx, adminID, Command{Action: "reject_cancel", ID: approved.ID, Version: requested.Version, RequestKey: key(32203), Note: "no"})
	if err != nil {
		t.Fatal(err)
	}
	var rejected Case
	if err = json.Unmarshal(raw, &rejected); err != nil {
		t.Fatal(err)
	}
	raw, err = s.Execute(ctx, adminID, Command{Action: "revert_cancel_review", ID: approved.ID, Version: rejected.Version, RequestKey: key(32204), Note: "mistake"})
	if err != nil {
		t.Fatal(err)
	}
	var restored Case
	if err = json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	var d Detail
	if err = json.Unmarshal(restored.Detail, &d); err != nil {
		t.Fatal(err)
	}
	if restored.State != "cancel_requested" || restored.Award != approved.Award || d.Cancellation == nil || d.Cancellation.Reason != "cancel" || d.Cancellation.Decision != "" {
		t.Fatalf("restored: %+v %+v", restored, d.Cancellation)
	}
}

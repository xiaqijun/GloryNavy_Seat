package feedback

// Transitions are validated on the locked, current ticket. Reply-only messages
// do not change workflow state; old completed records are never backfilled.
func validateTransition(t Ticket, actor string, admin bool, in Reply) error {
	if in.Status == "" {
		return nil
	}
	owner := actor == t.AccountID
	staff := admin && !owner
	if (in.Status == "accepted" || in.Status == "closed" || in.Status == "awaiting_confirmation") && !staff {
		return forbidden
	}
	if in.Status == "resolved" && !owner {
		return forbidden
	}
	if in.Status == "submitted" && !owner {
		return forbidden
	}
	allowed, reason := false, false
	switch t.Status {
	case "submitted":
		allowed = staff && (in.Status == "accepted" || in.Status == "closed")
		reason = in.Status == "closed"
	case "accepted":
		allowed = staff && (in.Status == "in_progress" || in.Status == "closed")
		reason = in.Status == "closed"
	case "in_progress":
		allowed = staff && (in.Status == "awaiting_confirmation" || in.Status == "closed" || in.Status == "accepted" && t.AcceptedAt == nil)
		reason = in.Status != "accepted"
	case "awaiting_confirmation":
		allowed = owner && (in.Status == "resolved" || in.Status == "in_progress")
		reason = in.Status == "in_progress"
	case "resolved":
		allowed = owner && in.Status == "in_progress"
		reason = true
	case "closed":
		allowed = owner && in.Status == "submitted"
		reason = true
	}
	if !allowed {
		return &Fault{409, "feedback_transition", "当前状态不能执行此操作，请刷新反馈"}
	}
	if reason && in.Body == "" {
		return &Fault{400, "feedback_reason_required", "请填写处理结果或原因"}
	}
	return nil
}

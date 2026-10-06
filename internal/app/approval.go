package app

import (
	"glorynavy.local/seat/internal/modules/approval"
	"glorynavy.local/seat/internal/modules/exchange"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/modules/loan"
	"glorynavy.local/seat/internal/modules/welfare"
	"glorynavy.local/seat/internal/platform/reviewqueue"
	"net/http"
	"slices"
)

func approvalHandler(enabled []string, accounts *identity.Service, w *welfare.Service, e *exchange.Service, l *loan.Service) approval.Handler {
	s := &approval.Service{Names: accounts.MainCharacterNames}
	if slices.Contains(enabled, "welfare") {
		s.Sources = append(s.Sources, reviewqueue.Source{ID: "welfare", Access: w.ApprovalAccess, Query: w.ApprovalQueue, People: w.ApprovalPeople})
	}
	if slices.Contains(enabled, "exchange") {
		s.Sources = append(s.Sources, reviewqueue.Source{ID: "exchange", Access: e.ApprovalAccess, Query: e.ApprovalQueue, People: e.ApprovalPeople})
	}
	if slices.Contains(enabled, "loan") {
		s.Sources = append(s.Sources, reviewqueue.Source{ID: "loan", Access: l.ApprovalAccess, Query: l.ApprovalQueue})
	}
	return approval.Handler{Service: s, User: func(r *http.Request) string { return identity.Principal(r.Context()).UserID }}
}

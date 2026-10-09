package app

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/modules/approval"
	"glorynavy.local/seat/internal/modules/exchange"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/modules/loan"
	"glorynavy.local/seat/internal/modules/welfare"
	"glorynavy.local/seat/internal/platform/reviewqueue"
	"net/http"
	"slices"
)

func approvalHandler(pool *pgxpool.Pool, dualRead bool, enabled []string, accounts *identity.Service, w *welfare.Service, e *exchange.Service, l *loan.Service) (approval.Handler, *approval.Projection) {
	s := approval.NewService(accounts.MainCharacterNames, pool)
	s.DualRead = dualRead
	if slices.Contains(enabled, "welfare") {
		s.Sources = append(s.Sources, reviewqueue.Source{ID: "welfare", Access: w.ApprovalAccess, Query: w.ApprovalQueue, QueryAuthorized: w.ApprovalQueueAuthorized, People: w.ApprovalPeople, PeopleAuthorized: w.ApprovalPeopleAuthorized, Snapshot: w.ApprovalSnapshot, Decorate: w.ApprovalDecorate})
	}
	if slices.Contains(enabled, "exchange") {
		s.Sources = append(s.Sources, reviewqueue.Source{ID: "exchange", Access: e.ApprovalAccess, IndexAccess: e.ApprovalIndexAccess, Query: e.ApprovalQueue, QueryAuthorized: e.ApprovalQueueAuthorized, People: e.ApprovalPeople, PeopleAuthorized: e.ApprovalPeopleAuthorized, Snapshot: e.ApprovalSnapshot, Decorate: e.ApprovalDecorate})
	}
	if slices.Contains(enabled, "loan") {
		s.Sources = append(s.Sources, reviewqueue.Source{ID: "loan", Access: l.ApprovalAccess, Query: l.ApprovalQueue, QueryAuthorized: l.ApprovalQueueAuthorized, Snapshot: l.ApprovalSnapshot, Decorate: l.ApprovalDecorate})
	}
	projection := approval.NewProjection(pool)
	return approval.Handler{Service: s, User: func(r *http.Request) string { return identity.Principal(r.Context()).UserID }}, projection
}

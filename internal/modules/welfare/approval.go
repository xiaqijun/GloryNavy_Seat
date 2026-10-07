package welfare

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
	"glorynavy.local/seat/internal/platform/reviewqueue"
	"strconv"
)

func (s *Service) ApprovalAccess(ctx context.Context, user string) (reviewqueue.Access, error) {
	out := reviewqueue.Access{Corporations: []reviewqueue.Option{}}
	corps, e := s.Corporations(ctx, user)
	if e != nil {
		return out, e
	}
	for _, c := range corps {
		if c.Manage {
			out.Allowed = true
			out.Corporations = append(out.Corporations, reviewqueue.Option{ID: strconv.FormatInt(c.ID, 10), Name: c.Name})
		}
	}
	return out, nil
}
func (s *Service) ApprovalQueue(ctx context.Context, user string, f reviewqueue.Filter, p reviewqueue.Position, limit int) (reviewqueue.Page, error) {
	access, e := s.ApprovalAccess(ctx, user)
	if e != nil {
		return reviewqueue.Page{}, e
	}
	return s.approvalQueue(ctx, user, f, p, limit, access)
}

// ApprovalQueueAuthorized reuses the source authorization collected by the
// central approval queue. The access result is still checked before any data
// is read, while avoiding a second full corporation scope lookup.
func (s *Service) ApprovalQueueAuthorized(ctx context.Context, user string, f reviewqueue.Filter, p reviewqueue.Position, limit int, access reviewqueue.Access) (reviewqueue.Page, error) {
	return s.approvalQueue(ctx, user, f, p, limit, access)
}

func (s *Service) approvalQueue(ctx context.Context, user string, f reviewqueue.Filter, p reviewqueue.Position, limit int, access reviewqueue.Access) (reviewqueue.Page, error) {
	if !access.Allowed {
		return reviewqueue.Page{}, pgx.ErrNoRows
	}
	scope := []map[string]string{}
	for _, corp := range access.Corporations {
		if f.Corporation != "" && f.Corporation != corp.ID {
			continue
		}
		id, _ := strconv.ParseInt(corp.ID, 10, 64)
		members, e := s.Members(ctx, user, id)
		if e != nil {
			return reviewqueue.Page{}, e
		}
		seen := map[string]bool{}
		for _, m := range members {
			if !seen[m.AccountID] {
				seen[m.AccountID] = true
				scope = append(scope, map[string]string{"corporation": corp.ID, "account": m.AccountID})
			}
		}
	}
	raw, _ := json.Marshal(scope)
	page, e := store.Approval(ctx, s.Pool, raw, user, f, p, limit)
	if e != nil {
		return page, e
	}
	cases := make([]Case, len(page.Items))
	for i := range page.Items {
		if e = json.Unmarshal(page.Items[i].Payload, &cases[i]); e != nil {
			return page, e
		}
	}
	cases = s.presentCases(ctx, cases)
	for i := range page.Items {
		v := &page.Items[i]
		c := cases[i]
		v.Payload = rawJSON(c)
		var d Detail
		if e = json.Unmarshal(c.Detail, &d); e != nil {
			return page, e
		}
		if isGrowth(c.Kind) || isActivity(c.Kind) {
			v.Unit = "reward"
			v.Amount = 0
		}
		if d.LossEvidence != nil {
			v.Title = d.LossEvidence.ShipName
		}
		if c.AccountID != user {
			switch c.State {
			case "submitted", "information", "external":
				v.Actions = []string{"approve", "reject", "information"}
			case "cancel_requested":
				v.Actions = []string{"approve_cancel", "reject_cancel"}
			case "approved", "executing":
				if coinOnly(d) {
					v.Actions = []string{"release_coins"}
				}
			}
		}
	}
	return page, nil
}
func rawJSON(v any) json.RawMessage { b, _ := json.Marshal(v); return b }

func (s *Service) ApprovalPeople(ctx context.Context, user string) ([]string, error) {
	a, e := s.ApprovalAccess(ctx, user)
	if e != nil {
		return nil, e
	}
	return s.approvalPeople(ctx, user, a)
}

// ApprovalPeopleAuthorized reuses the source authorization collected by the
// central approval context.
func (s *Service) ApprovalPeopleAuthorized(ctx context.Context, user string, a reviewqueue.Access) ([]string, error) {
	return s.approvalPeople(ctx, user, a)
}

func (s *Service) approvalPeople(ctx context.Context, user string, a reviewqueue.Access) ([]string, error) {
	if !a.Allowed {
		return nil, pgx.ErrNoRows
	}
	seen := map[string]bool{}
	ids := []string{}
	for _, c := range a.Corporations {
		id, _ := strconv.ParseInt(c.ID, 10, 64)
		members, e := s.Members(ctx, user, id)
		if e != nil {
			return nil, e
		}
		for _, m := range members {
			if !seen[m.AccountID] {
				seen[m.AccountID] = true
				ids = append(ids, m.AccountID)
			}
		}
	}
	return ids, nil
}

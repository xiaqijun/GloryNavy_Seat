package welfare

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
	"glorynavy.local/seat/internal/platform/reviewqueue"
	"strconv"
)

// ApprovalSnapshot is the source-owned compact projection consumed by the
// central approval index. It contains no actor-specific actions or names.
func (s *Service) ApprovalSnapshot(ctx context.Context) ([]reviewqueue.Item, error) {
	rows, err := store.Snapshot(ctx, s.Pool)
	if err != nil {
		return nil, err
	}
	out := make([]reviewqueue.Item, 0, len(rows))
	for _, row := range rows {
		c := row.Case
		payload := rawJSON(c)
		var d Detail
		_ = json.Unmarshal(c.Detail, &d)
		when := row.OccurredAt
		if when.IsZero() {
			when = c.CreatedAt
		}
		status := d.PaymentStatus
		if status == "" {
			status = c.State
		}
		title := d.Rule.ProjectName
		if title == "" && d.LossEvidence != nil {
			title = d.LossEvidence.ShipName
		}
		out = append(out, reviewqueue.Item{Source: "welfare", ID: c.ID, Version: c.Version, Account: c.AccountID, Corporation: strconv.FormatInt(c.CorporationID, 10), Kind: c.Kind, State: c.State, Status: status, Title: title, Reference: c.Reference, Amount: c.Award, Unit: "ISK", Time: when, Payload: payload})
	}
	return out, nil
}

// ApprovalDecorate restores only actor-specific presentation and actions
// after an item has been read from the shared index.
func (s *Service) ApprovalDecorate(ctx context.Context, user string, item *reviewqueue.Item) error {
	if item == nil {
		return nil
	}
	var c Case
	if err := json.Unmarshal(item.Payload, &c); err != nil {
		return err
	}
	var d Detail
	if err := json.Unmarshal(c.Detail, &d); err != nil {
		return err
	}
	if d.LossEvidence != nil {
		item.Title = d.LossEvidence.ShipName
	}
	if item.Account != user {
		switch c.State {
		case "submitted", "information", "external":
			item.Actions = []string{"approve", "reject", "information"}
		case "cancel_requested":
			item.Actions = []string{"approve_cancel", "reject_cancel"}
		case "approved", "executing":
			if coinOnly(d) {
				item.Actions = []string{"release_coins"}
			}
		}
	}
	return nil
}

func (s *Service) ApprovalAccess(ctx context.Context, user string) (reviewqueue.Access, error) {
	out := reviewqueue.Access{Corporations: []reviewqueue.Option{}}
	corps, e := s.Corporations(ctx, user)
	if e != nil {
		return out, e
	}
	for _, c := range corps {
		if c.Manage || c.Compensate {
			out.Allowed = true
			id := strconv.FormatInt(c.ID, 10)
			out.Corporations = append(out.Corporations, reviewqueue.Option{ID: id, Name: c.Name})
			if s.Members != nil {
				if out.AccountsByCorporation == nil {
					out.AccountsByCorporation = map[string][]string{}
				}
				members, err := s.Members(ctx, user, c.ID)
				if err != nil {
					return out, err
				}
				accounts := make([]string, 0, len(members))
				for _, member := range members {
					accounts = append(accounts, member.AccountID)
				}
				out.AccountsByCorporation[id] = accounts
			}
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
		full, e := s.Scope(ctx, user, id, true)
		if e != nil {
			return reviewqueue.Page{}, e
		}
		lossOnly := false
		if !full && s.CompensationScope != nil {
			lossOnly, e = s.CompensationScope(ctx, user, id)
			if e != nil {
				return reviewqueue.Page{}, e
			}
		}
		members, e := s.Members(ctx, user, id)
		if e != nil {
			return reviewqueue.Page{}, e
		}
		seen := map[string]bool{}
		for _, m := range members {
			if !seen[m.AccountID] {
				seen[m.AccountID] = true
				row := map[string]string{"corporation": corp.ID, "account": m.AccountID}
				if lossOnly {
					row["loss_only"] = "true"
				}
				scope = append(scope, row)
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
	if a.AccountsByCorporation != nil {
		for _, accounts := range a.AccountsByCorporation {
			for _, id := range accounts {
				if !seen[id] {
					seen[id] = true
					ids = append(ids, id)
				}
			}
		}
		return ids, nil
	}
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

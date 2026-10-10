package exchange

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/exchange/internal/store"
	"glorynavy.local/seat/internal/platform/reviewqueue"
	"strconv"
)

// ApprovalSnapshot converts source-owned redemption rows into the compact
// summary persisted by the central approval index.
func (s *Service) ApprovalSnapshot(ctx context.Context) ([]reviewqueue.Item, error) {
	rows, err := store.SnapshotRows(ctx, s.Pool)
	if err != nil {
		return nil, err
	}
	out := make([]reviewqueue.Item, 0, len(rows))
	for _, r := range rows {
		when := r.OccurredAt
		if when.IsZero() {
			when = r.CreatedAt.Time
		}
		order := RewardOrder{ID: r.ID, Version: r.Version, TypeID: r.TypeID, Name: r.RewardName, Quantity: r.Quantity, RecipientID: r.RecipientID, RecipientName: r.RecipientName, CoinsMinor: r.CoinsMinor, Rate: r.IskPerCoin, Value: r.IskValue, State: r.State, Note: r.Note, CreatedAt: when, Reference: r.SettlementReference}
		if len(r.RewardContent) > 0 {
			var content PhysicalReward
			if json.Unmarshal(r.RewardContent, &content) == nil {
				for i := range content.Fittings {
					content.Fittings[i].Fit = nil
				}
				order.Content = &content
			}
		}
		payload, _ := json.Marshal(order)
		status := r.DeliveryStatus
		if status == "" {
			status = "waiting_contract"
		}
		out = append(out, reviewqueue.Item{Source: "exchange", ID: r.ID, Version: r.Version, Account: r.AccountID.String(), ProcessedBy: r.ProcessedBy, History: r.History, Action: r.Action, Kind: "exchange", State: r.State, Status: status, Recipient: strconv.FormatInt(r.RecipientID, 10), Title: r.RewardName, Reference: r.SettlementReference, Amount: r.CoinsMinor, Unit: "coin", Time: when, Payload: payload})
	}
	return out, nil
}

func (s *Service) ApprovalDecorate(_ context.Context, user string, item *reviewqueue.Item) error {
	if item == nil {
		return nil
	}
	var order RewardOrder
	if err := json.Unmarshal(item.Payload, &order); err != nil {
		return err
	}
	// The central index already contains the frozen display summary. Keep this
	// hook actor-specific: resolving SDE names here would turn every indexed
	// list row back into a source fan-out and make optional presentation data
	// capable of delaying the approval page.
	if item.Account != user && item.State == "cancel_requested" {
		item.Actions = []string{"cancelled", "pending"}
	}
	return nil
}

func (s *Service) ApprovalAccess(ctx context.Context, user string) (reviewqueue.Access, error) {
	out := reviewqueue.Access{Corporations: []reviewqueue.Option{}}
	if s.Administrator == nil {
		return out, nil
	}
	ok, e := s.Administrator(ctx, user)
	out.Allowed = ok
	if e != nil || !ok {
		return out, e
	}
	return out, nil
}

// ApprovalIndexAccess adds the recipient/account bindings needed to authorize
// central index rows. It is intentionally separate from ApprovalAccess so the
// initial approval context does not query every historical recipient.
func (s *Service) ApprovalIndexAccess(ctx context.Context, user string) (reviewqueue.Access, error) {
	out, e := s.ApprovalAccess(ctx, user)
	if e != nil || !out.Allowed {
		return out, e
	}
	bindings, e := s.approvalBindings(ctx)
	if e != nil {
		return out, e
	}
	for _, binding := range bindings {
		out.Bindings = append(out.Bindings, reviewqueue.Binding{Account: binding["account"], Recipient: binding["recipient"]})
	}
	out.RestrictBindings = true
	return out, nil
}
func (s *Service) ApprovalQueue(ctx context.Context, user string, f reviewqueue.Filter, p reviewqueue.Position, limit int) (reviewqueue.Page, error) {
	a, e := s.ApprovalAccess(ctx, user)
	if e != nil {
		return reviewqueue.Page{}, e
	}
	return s.approvalQueue(ctx, user, f, p, limit, a)
}

// ApprovalQueueAuthorized reuses the source authorization collected by the
// central approval queue.
func (s *Service) ApprovalQueueAuthorized(ctx context.Context, user string, f reviewqueue.Filter, p reviewqueue.Position, limit int, a reviewqueue.Access) (reviewqueue.Page, error) {
	return s.approvalQueue(ctx, user, f, p, limit, a)
}

func (s *Service) approvalQueue(ctx context.Context, user string, f reviewqueue.Filter, p reviewqueue.Position, limit int, a reviewqueue.Access) (reviewqueue.Page, error) {
	if !a.Allowed {
		return reviewqueue.Page{}, pgx.ErrNoRows
	}
	scope, e := s.approvalBindings(ctx)
	if e != nil {
		return reviewqueue.Page{}, e
	}
	raw, _ := json.Marshal(scope)
	page, e := store.Approval(ctx, s.Pool, raw, user, f, p, limit)
	if e != nil {
		return page, e
	}
	for i := range page.Items {
		v := &page.Items[i]
		var order RewardOrder
		if e = json.Unmarshal(v.Payload, &order); e != nil {
			return page, e
		}
		if order.Content != nil {
			// The frozen reward snapshot already contains IDs and quantities.
			// Resolving display names is presentation-only; a temporary SDE
			// problem must not make the entire exchange source unavailable or
			// turn a valid count into an incomplete one.
			s.presentPhysicalForApproval(ctx, order.Content)
		}
		if order.Name == "" && s.Names != nil {
			if names, err := s.Names.TypeNames(ctx, []int64{order.TypeID}); err == nil {
				order.Name = names[order.TypeID].Name
			}
		}
		v.Title = order.Name
		v.Payload, _ = json.Marshal(order)
		if f.ID > 0 {
			history, err := store.ApprovalHistory(ctx, s.Pool, v.ID)
			if err != nil {
				return page, err
			}
			var payload map[string]json.RawMessage
			if err = json.Unmarshal(v.Payload, &payload); err != nil {
				return page, err
			}
			payload["history"], _ = json.Marshal(history)
			v.Payload, _ = json.Marshal(payload)
		}
		if v.Account != user && v.State == "cancel_requested" {
			v.Actions = []string{"cancelled", "pending"}
		}
	}
	return page, nil
}

// presentPhysicalForApproval enriches an immutable reward snapshot when the
// local SDE is available. Approval reads must remain usable when that optional
// presentation lookup is temporarily unavailable.
func (s *Service) presentPhysicalForApproval(ctx context.Context, c *PhysicalReward) {
	if len(c.Items) == 0 || s.Names == nil {
		return
	}
	ids := make([]int64, 0, len(c.Items))
	for _, item := range c.Items {
		ids = append(ids, item.ID)
	}
	names, err := s.Names.TypeNames(ctx, ids)
	if err != nil {
		return
	}
	for i := range c.Items {
		if name := names[c.Items[i].ID].Name; name != "" {
			c.Items[i].Name = name
		}
	}
}

func (s *Service) ApprovalPeople(ctx context.Context, user string) ([]string, error) {
	a, e := s.ApprovalAccess(ctx, user)
	if e != nil {
		return nil, e
	}
	return s.approvalPeople(ctx, a)
}

// ApprovalPeopleAuthorized reuses the source authorization collected by the
// central approval context.
func (s *Service) ApprovalPeopleAuthorized(ctx context.Context, _ string, a reviewqueue.Access) ([]string, error) {
	return s.approvalPeople(ctx, a)
}

func (s *Service) approvalPeople(ctx context.Context, a reviewqueue.Access) ([]string, error) {
	if !a.Allowed {
		return nil, pgx.ErrNoRows
	}
	rows, e := s.approvalBindings(ctx)
	if e != nil {
		return nil, e
	}
	ids := []string{}
	seen := map[string]bool{}
	for _, r := range rows {
		if !seen[r["account"]] {
			seen[r["account"]] = true
			ids = append(ids, r["account"])
		}
	}
	return ids, nil
}

func (s *Service) approvalBindings(ctx context.Context) ([]map[string]string, error) {
	if s.Bindings == nil {
		return nil, ErrUnavailable
	}
	ids, e := store.ApprovalRecipients(ctx, s.Pool)
	if e != nil {
		return nil, e
	}
	out := []map[string]string{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, e := s.Bindings(ctx, nil, ids)
	if e != nil {
		return nil, e
	}
	for _, r := range rows {
		out = append(out, map[string]string{"account": r.UserID, "recipient": strconv.FormatInt(r.ID, 10)})
	}
	return out, nil
}

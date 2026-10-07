package loan

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/loan/internal/store"
	"glorynavy.local/seat/internal/platform/reviewqueue"
)

// ApprovalAccess exposes only corporation pools to the central, read-only
// approval queue. Personal lender decisions remain in the loan module.
func (s *Service) ApprovalAccess(ctx context.Context, user string) (reviewqueue.Access, error) {
	out := reviewqueue.Access{Corporations: []reviewqueue.Option{}}
	pools, err := store.ListAllPools(ctx, s.db())
	if err != nil {
		return out, err
	}
	seen := map[string]bool{}
	for _, p := range pools {
		if p.LenderKind != "corporation" || p.CorporationID == nil {
			continue
		}
		ok, err := s.canManage(ctx, user, p)
		if err != nil {
			return out, err
		}
		if !ok {
			continue
		}
		id := strconv.FormatInt(*p.CorporationID, 10)
		if seen[id] {
			continue
		}
		seen[id] = true
		out.Allowed = true
		out.Corporations = append(out.Corporations, reviewqueue.Option{ID: id, Name: fmt.Sprintf("%s（%s）", id, p.Name)})
	}
	return out, nil
}

func (s *Service) ApprovalQueue(ctx context.Context, user string, f reviewqueue.Filter, p reviewqueue.Position, limit int) (reviewqueue.Page, error) {
	a, err := s.ApprovalAccess(ctx, user)
	if err != nil {
		return reviewqueue.Page{}, err
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
	scope := make([]map[string]string, 0, len(a.Corporations))
	for _, c := range a.Corporations {
		if f.Corporation != "" && f.Corporation != c.ID {
			continue
		}
		scope = append(scope, map[string]string{"corporation": c.ID})
	}
	raw, _ := json.Marshal(scope)
	return reviewqueue.Read(ctx, s.Pool, `
 SELECT c.id,
 CASE WHEN $11='history' THEN coalesce(a.created_at,c.updated_at,c.created_at) ELSE c.created_at END moment,
 CASE WHEN c.state='submitted' THEN 'pending' ELSE 'history' END bucket,
 (c.state<>'submitted') history,
 coalesce(a.actor_id::text,'') processed_by,
 jsonb_build_object(
   'id',c.id::text,'version',c.version::text,'account_id',c.borrower_account_id::text,
   'corporation_id',p.corporation_id::text,'kind','loan','state',c.state,'status',c.state,
   'recipient',c.borrower_character_id::text,'title',p.name,'reference',c.public_id,
   'amount_minor',c.principal_minor,'unit','isk','action',coalesce(a.action,''),
   'actions',CASE WHEN c.state='submitted' AND c.borrower_account_id<>$2::uuid THEN '["approve","reject"]'::jsonb ELSE '[]'::jsonb END,
   'payload',jsonb_build_object('id',c.id::text,'public_id',c.public_id,'version',c.version::text,
     'pool_id',c.pool_id::text,'pool_name',p.name,'corporation_id',p.corporation_id::text,
     'borrower_account_id',c.borrower_account_id::text,'borrower_character_id',c.borrower_character_id::text,
     'principal_minor',c.principal_minor,'interest_minor',c.interest_minor,'total_due_minor',c.total_due_minor,
     'installment_count',c.installment_count,'interval_days',c.interval_days,'first_due_at',c.first_due_at,
     'state',c.state,'review_note',c.review_note,'created_at',c.created_at)) item
 FROM loan_cases c
 JOIN loan_pools p ON p.id=c.pool_id
 LEFT JOIN LATERAL (
   SELECT actor_id, action, created_at FROM loan_audit
   WHERE case_id=c.id AND action='review' ORDER BY id DESC LIMIT 1
 ) a ON true
 WHERE p.lender_kind='corporation'
 AND EXISTS(SELECT 1 FROM jsonb_array_elements($1::jsonb) scope WHERE scope->>'corporation'=p.corporation_id::text)
 `, raw, user, f, p, limit, "loan")
}

package approval

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/modules/approval/internal/store"
	"glorynavy.local/seat/internal/platform/reviewqueue"
)

// Projection owns only the central list read model. Source modules continue
// to own details and decisions and expose snapshots through the host-registered
// reviewqueue.Source contract.
type Projection struct {
	Index    store.Index
	Interval time.Duration
}

func NewProjection(pool *pgxpool.Pool) *Projection {
	return &Projection{Index: store.Index{Pool: pool}, Interval: 2 * time.Minute}
}

func (p *Projection) Run(ctx context.Context, sources []reviewqueue.Source) {
	if p == nil || p.Index.Pool == nil {
		return
	}
	if err := p.Reconcile(ctx, sources); err != nil {
		slog.Warn("approval projection reconcile failed", "error", err)
	}
	ticker := time.NewTicker(p.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.Reconcile(ctx, sources); err != nil {
				slog.Warn("approval projection reconcile failed", "error", err)
			}
		}
	}
}

func (p *Projection) Reconcile(ctx context.Context, sources []reviewqueue.Source) error {
	var failures []error
	for _, source := range sources {
		if source.Snapshot == nil {
			continue
		}
		if err := p.Index.MarkSource(ctx, source.ID, "running", ""); err != nil {
			failures = append(failures, err)
			continue
		}
		items, err := source.Snapshot(ctx)
		if err != nil {
			_ = p.Index.MarkSource(ctx, source.ID, "error", err.Error())
			failures = append(failures, fmt.Errorf("%s snapshot: %w", source.ID, err))
			continue
		}
		if err = p.Index.ReplaceSource(ctx, source.ID, items); err != nil {
			_ = p.Index.MarkSource(ctx, source.ID, "error", err.Error())
			failures = append(failures, fmt.Errorf("%s replace: %w", source.ID, err))
		}
	}
	return errors.Join(failures...)
}

func scopes(access map[string]reviewqueue.Access) []store.Scope {
	out := make([]store.Scope, 0, len(access))
	for source, a := range access {
		if !a.Allowed {
			continue
		}
		bindings := make([]store.Binding, 0, len(a.Bindings))
		for _, binding := range a.Bindings {
			bindings = append(bindings, store.Binding{Account: binding.Account, Recipient: binding.Recipient})
		}
		if len(a.Corporations) == 0 {
			out = append(out, store.Scope{Source: source, All: !a.RestrictBindings, Bindings: bindings, RestrictBindings: a.RestrictBindings})
			continue
		}
		for _, corp := range a.Corporations {
			accounts := a.Accounts
			if a.AccountsByCorporation != nil {
				accounts = a.AccountsByCorporation[corp.ID]
			}
			out = append(out, store.Scope{Source: source, Corporation: corp.ID, Accounts: accounts, RestrictAccounts: a.AccountsByCorporation != nil, Bindings: bindings, RestrictBindings: a.RestrictBindings})
		}
	}
	return out
}

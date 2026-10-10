// Package testfixture provides deterministic in-memory approval sources for
// adapter contract tests. It deliberately has no database or network access.
package testfixture

import (
	"context"
	"sync/atomic"

	"glorynavy.local/seat/internal/platform/reviewqueue"
)

type Source struct {
	id            string
	items         []reviewqueue.Item
	queryCalls    atomic.Int64
	snapshotCalls atomic.Int64
}

func New(id string, items []reviewqueue.Item) *Source {
	return &Source{id: id, items: append([]reviewqueue.Item(nil), items...)}
}

func (s *Source) Adapter() reviewqueue.Source {
	return reviewqueue.Source{
		ID: s.id,
		Capabilities: reviewqueue.Capabilities{
			ApplicantFilter: true,
			Amount:          true,
			DetailKind:      s.id + "-detail",
		},
		Access: func(context.Context, string) (reviewqueue.Access, error) {
			return reviewqueue.Access{Allowed: true}, nil
		},
		Query: func(context.Context, string, reviewqueue.Filter, reviewqueue.Position, int) (reviewqueue.Page, error) {
			s.queryCalls.Add(1)
			return reviewqueue.Page{Items: append([]reviewqueue.Item(nil), s.items...), Counts: map[string]int64{"pending": int64(len(s.items))}}, nil
		},
		Snapshot: func(context.Context) ([]reviewqueue.Item, error) {
			s.snapshotCalls.Add(1)
			return append([]reviewqueue.Item(nil), s.items...), nil
		},
	}
}

func (s *Source) IndexOnlyAdapter() reviewqueue.Source {
	adapter := s.Adapter()
	adapter.Query = nil
	adapter.IndexOnly = true
	return adapter
}

func (s *Source) QueryCalls() int64    { return s.queryCalls.Load() }
func (s *Source) SnapshotCalls() int64 { return s.snapshotCalls.Load() }

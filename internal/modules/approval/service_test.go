package approval

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/platform/reviewqueue"
	"testing"
	"time"
)

func TestMergePaginationPermissionAndPartialFailure(t *testing.T) {
	ctx := context.Background()
	s := &Service{}
	now := time.Now().UTC().Truncate(time.Second)
	for _, name := range []string{"welfare", "exchange"} {
		name := name
		s.Sources = append(s.Sources, reviewqueue.Source{ID: name, Access: func(_ context.Context, u string) (reviewqueue.Access, error) {
			return reviewqueue.Access{Allowed: u == "manager"}, nil
		}, Query: func(_ context.Context, _ string, f reviewqueue.Filter, p reviewqueue.Position, n int) (reviewqueue.Page, error) {
			out := reviewqueue.Page{Items: []reviewqueue.Item{}, Counts: map[string]int64{"pending": 40}}
			for i := int64(1); i <= 40; i++ {
				v := reviewqueue.Item{Source: name, ID: i, Time: now}
				if !p.Time.IsZero() && (name < p.Source || name == p.Source && i <= p.ID) {
					continue
				}
				out.Items = append(out.Items, v)
				if len(out.Items) == n {
					break
				}
			}
			return out, nil
		}})
	}
	f := reviewqueue.Filter{View: "pending"}
	token := ""
	seen := map[string]bool{}
	for {
		p, e := s.List(ctx, "manager", f, token)
		if e != nil {
			t.Fatal(e)
		}
		if p.Counts["pending"] != 80 {
			t.Fatal(p.Counts)
		}
		for _, i := range p.Items {
			k := fmt.Sprint(i.Source, i.ID)
			if seen[k] {
				t.Fatal("duplicate", k)
			}
			seen[k] = true
		}
		token = p.Next
		if token == "" {
			break
		}
		if len(seen) > 80 {
			t.Fatal("loop")
		}
	}
	if len(seen) != 80 {
		t.Fatal("missing", len(seen))
	}
	if _, e := s.List(ctx, "member", f, ""); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatal("permission", e)
	}
	if _, e := s.List(ctx, "manager", reviewqueue.Filter{View: "pending", Sort: "amount_desc"}, ""); !errors.Is(e, ErrInvalid) {
		t.Fatal("sort validation", e)
	}
	p, _ := s.List(ctx, "manager", f, "")
	f.Kind = "solo"
	if _, e := s.List(ctx, "manager", f, p.Next); !errors.Is(e, ErrInvalid) {
		t.Fatal("cursor filters", e)
	}
	s.Sources[0].Query = func(context.Context, string, reviewqueue.Filter, reviewqueue.Position, int) (reviewqueue.Page, error) {
		return reviewqueue.Page{}, errors.New("down")
	}
	p, e := s.List(ctx, "manager", reviewqueue.Filter{View: "pending"}, "")
	if e != nil || len(p.Unavailable) != 1 || p.Next != "" || len(p.Items) == 0 {
		t.Fatalf("partial page %+v %v", p, e)
	}
	if _, e = s.Detail(ctx, "manager", "unknown", "1"); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatal(e)
	}
}

func TestListSortsByID(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	s := &Service{Sources: []reviewqueue.Source{{
		ID: "welfare",
		Access: func(context.Context, string) (reviewqueue.Access, error) {
			return reviewqueue.Access{Allowed: true}, nil
		},
		Query: func(context.Context, string, reviewqueue.Filter, reviewqueue.Position, int) (reviewqueue.Page, error) {
			items := make([]reviewqueue.Item, 0, 31)
			for i := int64(1); i <= 31; i++ {
				items = append(items, reviewqueue.Item{Source: "welfare", ID: i, Time: now})
			}
			return reviewqueue.Page{Items: items, Counts: map[string]int64{}}, nil
		},
	}}}
	p, err := s.List(context.Background(), "manager", reviewqueue.Filter{View: "pending", Sort: "id_asc"}, "")
	if err != nil || len(p.Items) != 30 || p.Items[0].ID != 1 || p.Items[29].ID != 30 || p.Next == "" {
		t.Fatalf("ascending ID sort: %+v %v", p.Items, err)
	}
	if _, err = s.List(context.Background(), "manager", reviewqueue.Filter{View: "pending", Sort: "id_asc"}, p.Next); err != nil {
		t.Fatalf("ascending ID cursor: %v", err)
	}
	p, err = s.List(context.Background(), "manager", reviewqueue.Filter{View: "pending", Sort: "id_desc"}, "")
	if err != nil || len(p.Items) != 30 || p.Items[0].ID != 31 || p.Items[29].ID != 2 {
		t.Fatalf("descending ID sort: %+v %v", p.Items, err)
	}
}

func TestListReusesSourceAccessForAuthorizedQuery(t *testing.T) {
	accessCalls, queryCalls, authorizedCalls := 0, 0, 0
	s := &Service{Sources: []reviewqueue.Source{{
		ID: "welfare",
		Access: func(context.Context, string) (reviewqueue.Access, error) {
			accessCalls++
			return reviewqueue.Access{Allowed: true, Corporations: []reviewqueue.Option{{ID: "10"}}}, nil
		},
		Query: func(context.Context, string, reviewqueue.Filter, reviewqueue.Position, int) (reviewqueue.Page, error) {
			queryCalls++
			return reviewqueue.Page{}, nil
		},
		QueryAuthorized: func(_ context.Context, _ string, _ reviewqueue.Filter, _ reviewqueue.Position, _ int, access reviewqueue.Access) (reviewqueue.Page, error) {
			authorizedCalls++
			if !access.Allowed || len(access.Corporations) != 1 {
				return reviewqueue.Page{}, errors.New("missing access result")
			}
			return reviewqueue.Page{Counts: map[string]int64{"pending": 1}}, nil
		},
	}}}

	result, err := s.List(context.Background(), "manager", reviewqueue.Filter{View: "pending"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if accessCalls != 1 || queryCalls != 0 || authorizedCalls != 1 || result.Counts["pending"] != 1 {
		t.Fatalf("access=%d query=%d authorized=%d result=%+v", accessCalls, queryCalls, authorizedCalls, result)
	}
}

package approval

import (
	"context"
	"errors"
	"fmt"
	"glorynavy.local/seat/internal/platform/reviewqueue"
	"testing"
	"time"
)

func TestPendingNotificationKeysLivePermissionsPaginationAndActions(t *testing.T) {
	allowed := true
	failing := false
	accessCalls := 0
	source := reviewqueue.Source{ID: "welfare", Access: func(context.Context, string) (reviewqueue.Access, error) {
		accessCalls++
		return reviewqueue.Access{Allowed: allowed}, nil
	}}
	source.Query = func(_ context.Context, user string, f reviewqueue.Filter, p reviewqueue.Position, limit int) (reviewqueue.Page, error) {
		if failing {
			return reviewqueue.Page{}, errors.New("offline")
		}
		if f.View != "pending" || f.Sort != "id_asc" {
			t.Fatal(f)
		}
		out := reviewqueue.Page{}
		for id := p.ID + 1; id <= 36 && len(out.Items) < limit; id++ {
			item := reviewqueue.Item{Source: "welfare", ID: id, Account: "other", State: "submitted", Status: "submitted", Actions: []string{"approve"}, Time: time.Unix(id, 0)}
			if id == 1 {
				item.Account = user
			}
			if id == 2 {
				item.Actions = []string{}
			}
			if id == 3 {
				item.State = "cancel_requested"
				item.Status = "cancel_requested"
				item.Actions = []string{"approve_cancel"}
			}
			if id == 4 {
				item.Source = "exchange"
				item.State = "cancel_requested"
				item.Status = "cancel_requested"
				item.Actions = []string{"cancelled", "pending"}
			}
			if id == 5 {
				item.Actions = []string{"release_coins"}
			}
			out.Items = append(out.Items, item)
		}
		return out, nil
	}
	s := &Service{Sources: []reviewqueue.Source{source}, UseIndex: true, AccessCacheTTL: time.Hour}
	keys, err := s.PendingNotificationKeys(context.Background(), "reviewer")
	if err != nil || len(keys) != 33 || accessCalls < 2 {
		t.Fatal(len(keys), accessCalls, err)
	}
	for _, key := range keys {
		if len(key) != 64 {
			t.Fatal("reference leaks business fields", key)
		}
	}
	allowed = false
	if keys, err = s.PendingNotificationKeys(context.Background(), "reviewer"); err != nil || len(keys) != 0 {
		t.Fatal("cached permission", keys, err)
	}
	allowed = true
	failing = true
	if keys, err = s.PendingNotificationKeys(context.Background(), "reviewer"); err == nil || keys != nil {
		t.Fatal("partial source read treated as complete", fmt.Sprint(keys), err)
	}
}

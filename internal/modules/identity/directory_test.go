package identity

import (
	"context"
	"fmt"
	"glorynavy.local/seat/internal/testutil"
	"testing"
)

func TestMemberDirectorySearchAndPagination(t *testing.T) {
	pool := testutil.Database(t)
	ctx := context.Background()
	s := New(pool)
	for i := int64(1); i <= 27; i++ {
		if _, err := s.SignIn(ctx, i, fmt.Sprintf("舰长 %d", i), "owner", ""); err != nil {
			t.Fatal(err)
		}
	}
	first, next, err := s.SearchMembers(ctx, "", "")
	if err != nil || len(first) != 25 || next == "" {
		t.Fatalf("first page %d %q %v", len(first), next, err)
	}
	second, end, err := s.SearchMembers(ctx, "", next)
	if err != nil || len(second) != 2 || end != "" {
		t.Fatalf("second page %d %q %v", len(second), end, err)
	}
	for _, a := range first {
		for _, b := range second {
			if a.UserID == b.UserID {
				t.Fatal("duplicate page member")
			}
		}
	}
	rows, _, err := s.SearchMembers(ctx, "27", "")
	if err != nil || len(rows) != 1 || rows[0].Main.ID != "27" {
		t.Fatal("ID search", err)
	}
	rows, _, err = s.SearchMembers(ctx, "%", "")
	if err != nil || len(rows) != 0 {
		t.Fatal("search wildcard should be literal", err)
	}
	rows, _, err = s.SearchMembers(ctx, first[0].UserID, "")
	if err != nil || len(rows) != 1 {
		t.Fatal("user search", err)
	}
}

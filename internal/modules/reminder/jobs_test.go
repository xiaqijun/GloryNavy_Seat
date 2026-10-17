package reminder

import (
	"context"
	"github.com/riverqueue/river"
	"testing"
)

func TestScannerSkipsUnconfiguredAuthorizationWithoutQueueingAccounts(t *testing.T) {
	s, _, _ := fixture(t)
	s.Users = func(context.Context) ([]string, error) { t.Fatal("unready sender scanned recipients"); return nil, nil }
	s.Own = func(context.Context, string) ([]Character, error) { return []Character{}, nil }
	w := &scanWorker{runtime: &Jobs{Service: s}}
	if err := w.Work(context.Background(), &river.Job[scanArgs]{}); err != nil {
		t.Fatal(err)
	}
}

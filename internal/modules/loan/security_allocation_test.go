package loan

import (
	"errors"
	"testing"
)

func TestSecurityAllocationPreservesSnapshotAndSupportsPartial(t *testing.T) {
	s := SecuritySummary{CreditReady: true, UnallocatedMinor: 200}
	for _, tc := range []struct {
		expected, allocation, want int64
		err                        error
	}{
		{200, 0, 200, nil}, {200, 100, 100, nil}, {0, 100, 100, nil},
		{199, 100, 0, ErrConflict}, {200, 201, 0, ErrConflict}, {200, -1, 0, ErrConflict},
	} {
		got, err := securityAllocation(s, tc.expected, tc.allocation)
		if got != tc.want || !errors.Is(err, tc.err) {
			t.Fatalf("expected=%d allocation=%d got=%d err=%v", tc.expected, tc.allocation, got, err)
		}
	}
}

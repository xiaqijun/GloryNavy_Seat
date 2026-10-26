package feedback

import (
	"testing"
	"time"
)

func TestFeedbackWithdrawalPolicy(t *testing.T) {
	accepted := time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, state, actor string
		admin, adopted     bool
		want               int
	}{
		{"owner pending", "submitted", "owner", false, false, 0},
		{"owner administrator", "submitted", "owner", true, false, 0},
		{"other administrator", "submitted", "staff", true, false, 403},
		{"other member", "submitted", "other", false, false, 403},
		{"previous adoption", "submitted", "owner", false, true, 409},
		{"accepted", "accepted", "owner", false, true, 409},
		{"working", "in_progress", "owner", false, false, 409},
		{"confirmation", "awaiting_confirmation", "owner", false, true, 409},
		{"resolved", "resolved", "owner", false, true, 409},
		{"closed", "closed", "owner", false, false, 409},
		{"withdrawn", "withdrawn", "owner", false, false, 409},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ticket := Ticket{AccountID: "owner", Status: tc.state}
			if tc.adopted {
				ticket.AcceptedAt = &accepted
			}
			err := validateTransition(ticket, tc.actor, tc.admin, Reply{Status: "withdrawn"})
			if tc.want == 0 {
				if err != nil {
					t.Fatal(err)
				}
			} else if fault, ok := err.(*Fault); !ok || fault.Status != tc.want {
				t.Fatalf("want %d, got %v", tc.want, err)
			}
		})
	}
}

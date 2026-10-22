package welfare

import (
	"encoding/json"
	"glorynavy.local/seat/internal/modules/eve"
	"testing"
)

func TestApprovalCapitalEstimatedAmount(t *testing.T) {
	rate := int64(1250)
	invalid := int64(10001)
	for _, tc := range []struct {
		name, kind, state string
		rate              *int64
		purchase          bool
		base, award, want int64
	}{
		{"custom snapshot rounded down", "supercarrier", "submitted", &rate, true, 12345678901, 0, 1543209862},
		{"legacy supercarrier", "supercarrier", "information", nil, true, 100000000000, 0, 10000000000},
		{"legacy titan", "titan", "external", nil, true, 100000000000, 0, 5000000000},
		{"no purchase", "supercarrier", "submitted", nil, false, 100000000000, 0, 0},
		{"invalid rate", "supercarrier", "submitted", &invalid, true, 100000000000, 0, 0},
		{"approved frozen award", "supercarrier", "approved", &rate, true, 12345678901, 222222222, 0},
		{"completed frozen award", "titan", "completed", nil, true, 100000000000, 222222222, 0},
		{"cancelled", "supercarrier", "cancelled", nil, true, 100000000000, 0, 0},
		{"other welfare", "solo", "submitted", nil, true, 100000000000, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := Detail{BaseMinor: tc.base, Rule: Config{SubsidyRateBPS: tc.rate}}
			if tc.purchase {
				d.Purchase = &eve.DeliveryContract{ID: 901}
			}
			c := Case{ID: 125, Kind: tc.kind, State: tc.state, Award: tc.award, Detail: rawJSON(d)}
			var got struct {
				Case
				EstimatedAward int64 `json:"estimated_award_minor"`
			}
			if err := json.Unmarshal(approvalSummary(c, d), &got); err != nil {
				t.Fatal(err)
			}
			if got.EstimatedAward != tc.want || got.Award != tc.award {
				t.Fatalf("estimate=%d award=%d, want %d / %d", got.EstimatedAward, got.Award, tc.want, tc.award)
			}
			var compact Detail
			if err := json.Unmarshal(got.Detail, &compact); err != nil {
				t.Fatal(err)
			}
			if compact.Purchase != nil {
				t.Fatal("full purchase evidence leaked into summary")
			}
		})
	}
}

package contractamount

import "testing"

func TestWholeISKAndHistoricalExactPayment(t *testing.T) {
	for _, tc := range []struct {
		minor  int64
		reward string
		want   bool
	}{
		{5352666651, "53526666", true},
		{5352666651, "53526666.51", true},
		{5352666651, "53526667", false},
		{5352666651, "53526666.50", false},
		{12345, "123", true},
		{12345, "123.45", true},
		{12345, "124", false},
		{99, "0", false},
		{0, "0", false},
		{12345, "NaN", false},
	} {
		if got := Matches(tc.minor, tc.reward); got != tc.want {
			t.Fatalf("Matches(%d, %q) = %v, want %v", tc.minor, tc.reward, got, tc.want)
		}
	}
	if got := WholeISK(5352666651); got != 53526666 {
		t.Fatal(got)
	}
}

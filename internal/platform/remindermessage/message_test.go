package remindermessage

import (
	"math"
	"testing"
	"time"
)

func TestExactMoneyAndUTC(t *testing.T) {
	for _, c := range []struct {
		n    int64
		want string
	}{{0, "0.00 ISK"}, {1, "0.01 ISK"}, {-1, "-0.01 ISK"}, {123456789001, "1,234,567,890.01 ISK"}, {math.MinInt64, "-92,233,720,368,547,758.08 ISK"}, {math.MaxInt64, "92,233,720,368,547,758.07 ISK"}} {
		if got := Money(c.n, "ISK"); got != c.want {
			t.Fatalf("%d: %s != %s", c.n, got, c.want)
		}
	}
	if Signed(120, "果壳币") != "+1.20 果壳币" {
		t.Fatal("positive ledger sign lost")
	}
	date := time.Date(2026, 10, 20, 8, 5, 0, 0, time.FixedZone("UTC+8", 8*3600))
	if Date(date) != "2026-10-20 00:05 UTC" {
		t.Fatal(Date(date))
	}
}

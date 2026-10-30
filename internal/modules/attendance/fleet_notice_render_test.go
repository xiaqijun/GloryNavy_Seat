package attendance

import (
	"strings"
	"testing"
	"time"
)

func TestFleetNoticeMOTDEscapesUserTextAndIncludesStatus(t *testing.T) {
	event := Event{Title: "<舰队>", StartsAt: time.Date(2026, 10, 29, 14, 0, 0, 0, time.UTC), PAPIssued: true, PAPPoints: 1}
	notice := FleetNotice{SolarSystem: "Jita", Doctrine: "<script>bad</script>", Details: "飞船 & 后勤"}
	got := fleetNoticeMOTD(event, notice)
	for _, want := range []string{"&lt;舰队&gt;", "2026-10-29 22:00", "&lt;script&gt;bad&lt;/script&gt;", "飞船 &amp; 后勤", "PAP：已发放（每角色 1）"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
	if strings.Contains(got, "<script>") {
		t.Fatal("unescaped HTML")
	}
}

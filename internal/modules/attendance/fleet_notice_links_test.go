package attendance

import (
	"strings"
	"testing"
	"time"
)

func TestFleetNoticeLinksRenderAndEscape(t *testing.T) {
	event := Event{Title: "舰队 <测试>", StartsAt: time.Now()}
	notice := FleetNotice{Doctrine: "船 & 配置", Commander: "指挥 <甲>", Links: FleetNoticeLinks{
		CommanderID: "90000001", FireAnchor: "火力", FireAnchorID: "90000002", LogisticsAnchor: "后勤", LogisticsAnchorID: "90000003", FittingDNA: "587:34;2:35;1::",
	}}
	if !validFleetNoticeLinks(notice.Links, notice.Commander) {
		t.Fatal("valid links rejected")
	}
	got := fleetNoticeHTML(event, notice)
	for _, want := range []string{`href="fitting:587:34;2:35;1::"`, `href="showinfo:1377//90000001"`, `href="showinfo:1377//90000002"`, `href="showinfo:1377//90000003"`, "船 &amp; 配置", "指挥 &lt;甲&gt;"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
	notice.Links.CommanderID = `1" onclick="bad`
	notice.Links.FittingDNA = `587:34;2::" onclick="bad`
	if validFleetNoticeLinks(notice.Links, notice.Commander) {
		t.Fatal("invalid links accepted")
	}
	got = fleetNoticeHTML(event, notice)
	if strings.Contains(got, `onclick=`) || strings.Contains(got, `href="fitting:`) || strings.Contains(got, `href="showinfo:1377//1`) {
		t.Fatalf("unsafe markup: %s", got)
	}
	text := fleetNoticeText(event, notice)
	if !strings.Contains(text, "火力走位：火力") || !strings.Contains(text, "后勤走位：后勤") || strings.Contains(text, "href=") {
		t.Fatalf("plain text: %s", text)
	}
}

func TestSelectedNoticeCharacterMatchesDirectory(t *testing.T) {
	candidates := []Binding{{ID: 12, Name: "Pilot"}}
	if !selectedNoticeCharacter("12", "Pilot", candidates, "", "") {
		t.Fatal("directory entry rejected")
	}
	if selectedNoticeCharacter("12", "Impostor", candidates, "", "") || selectedNoticeCharacter("13", "Pilot", candidates, "", "") {
		t.Fatal("mismatched character accepted")
	}
	if !selectedNoticeCharacter("99", "Former pilot", candidates, "99", "Former pilot") {
		t.Fatal("unchanged historical selection rejected")
	}
}

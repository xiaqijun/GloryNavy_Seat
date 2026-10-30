package attendance

import (
	"fmt"
	"html"
	"strings"
	"time"
)

// fleetNoticeText uses one frozen template for all delivery channels.
func fleetNoticeText(event Event, n FleetNotice) string {
	zone := time.FixedZone("Asia/Shanghai", 8*60*60)
	lines := []string{
		event.Title,
		"时间：" + event.StartsAt.In(zone).Format("2006-01-02 15:04") + "（北京时间）",
		"星系：" + n.SolarSystem,
		"建制：" + n.Doctrine,
	}
	if n.MeetingPoint != "" {
		lines = append(lines, "集结地点："+n.MeetingPoint)
	}
	if n.Commander != "" {
		lines = append(lines, "指挥："+n.Commander)
	}
	if n.Details != "" {
		lines = append(lines, n.Details)
	}
	return strings.Join(lines, "\n")
}

func fleetNoticeMOTD(event Event, n FleetNotice) string {
	text := html.EscapeString(fleetNoticeText(event, n))
	status := "PAP：未发放"
	if event.PAPIssued {
		status = fmt.Sprintf("PAP：已发放（每角色 %d）", event.PAPPoints)
	}
	return strings.ReplaceAll(text, "\n", "<br>") + "<br>" + html.EscapeString(status)
}

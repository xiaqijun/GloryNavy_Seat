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
	if n.Links.FireAnchor != "" {
		lines = append(lines, "火力走位："+n.Links.FireAnchor)
	}
	if n.Links.LogisticsAnchor != "" {
		lines = append(lines, "后勤走位："+n.Links.LogisticsAnchor)
	}
	if n.Details != "" {
		lines = append(lines, n.Details)
	}
	return strings.Join(lines, "\n")
}

func fleetNoticeMOTD(event Event, n FleetNotice) string {
	text := fleetNoticeHTML(event, n)
	status := "PAP：未发放"
	if event.PAPIssued {
		status = fmt.Sprintf("PAP：已发放（每角色 %d）", event.PAPPoints)
	}
	return text + "<br>" + html.EscapeString(status)
}

// Only server-built, strictly validated EVE links are emitted as markup.
func fleetNoticeHTML(event Event, n FleetNotice) string {
	zone := time.FixedZone("Asia/Shanghai", 8*60*60)
	lines := []string{html.EscapeString(event.Title), "时间：" + event.StartsAt.In(zone).Format("2006-01-02 15:04") + "（北京时间）", "星系：" + html.EscapeString(n.SolarSystem)}
	fit := html.EscapeString(n.Doctrine)
	if n.Links.FittingDNA != "" && len(n.Links.FittingDNA) <= 6000 && fleetDNA.MatchString(n.Links.FittingDNA) {
		fit = `<a href="fitting:` + n.Links.FittingDNA + `">` + fit + `</a>`
	}
	lines = append(lines, "建制："+fit)
	if n.MeetingPoint != "" {
		lines = append(lines, "集结地点："+html.EscapeString(n.MeetingPoint))
	}
	character := func(label, name, id string) {
		if name == "" {
			return
		}
		text := html.EscapeString(name)
		if id != "" && validNoticeID(id) {
			text = `<a href="showinfo:1377//` + id + `">` + text + `</a>`
		}
		lines = append(lines, label+"："+text)
	}
	character("指挥", n.Commander, n.Links.CommanderID)
	character("火力走位", n.Links.FireAnchor, n.Links.FireAnchorID)
	character("后勤走位", n.Links.LogisticsAnchor, n.Links.LogisticsAnchorID)
	if n.Details != "" {
		lines = append(lines, html.EscapeString(n.Details))
	}
	return strings.ReplaceAll(strings.Join(lines, "<br>"), "\n", "<br>")
}

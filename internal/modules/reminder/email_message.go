package reminder

import (
	"fmt"
	"html"
	"strings"

	"glorynavy.local/seat/internal/platform/remindermessage"
)

func renderEmailMessage(title, link string, items []remindermessage.Item) (string, error) {
	if len(items) == 0 || len(items) > mailBatchSize {
		return "", ErrEvents
	}

	var body strings.Builder
	body.WriteString(`<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"></head>
<body style="margin:0;background:#f1f5f9;color:#0f172a;font-family:Arial,'Microsoft YaHei',sans-serif;font-size:15px;line-height:1.6">
<table role="presentation" width="100%" cellspacing="0" cellpadding="0"><tr><td align="center" style="padding:20px 12px">
<table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="max-width:600px;border-collapse:separate;border-spacing:0">
<tr><td bgcolor="#1e3a8a" style="padding:20px 24px;color:#fff;border-radius:12px 12px 0 0">
<p style="margin:0 0 4px;font-size:14px">荣耀海军 SEAT</p>
`)
	fmt.Fprintf(&body, `<h1 style="margin:0;font-size:24px;line-height:1.3">%s</h1><p style="margin:6px 0 0">共 %d 项</p>`, mailText(title, 40), len(items))
	body.WriteString(`</td></tr><tr><td bgcolor="#ffffff" style="padding:4px 24px 24px;border-radius:0 0 12px 12px">`)
	for _, item := range items {
		if len(item.Fields) > 12 {
			return "", ErrEvents
		}
		fmt.Fprintf(&body, `<h2 style="margin:0;padding-top:16px;font-size:17px;line-height:1.4">%s</h2>`, mailText(item.Title, 100))
		for _, field := range item.Fields {
			if field.Value != "" {
				fmt.Fprintf(&body, `<p style="margin:4px 0 0;overflow-wrap:anywhere"><strong>%s</strong>：%s</p>`, mailText(field.Label, 24), mailText(field.Value, 100))
			}
		}
		if item.Action != "" {
			fmt.Fprintf(&body, `<p style="margin:8px 0 0;color:#334155">%s</p>`, mailText(item.Action, 160))
		}
		body.WriteString(`<hr style="margin:16px 0 0;border:0;border-top:1px solid #e2e8f0">`)
	}
	fmt.Fprintf(&body, `<p style="margin:16px 0 0"><a href="%s" style="color:#1d4ed8;font-weight:bold;text-decoration:underline">查看详情</a></p>`, html.EscapeString(link))
	body.WriteString(`</td></tr></table></td></tr></table></body></html>`)
	return body.String(), nil
}

package community

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Dynamic game data must not become markup or a QQ mention.
func escapeStructureMarkdown(value string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\\", "\\\\", "`", "\\`", "*", "\\*", "_", "\\_", "[", "\\[", "]", "\\]", "(", "\\(", ")", "\\)", "#", "\\#", "~", "\\~", "|", "\\|").Replace(value)
}

func formatStructureMarkdown(lines []string) string {
	parts := []string{"## SEAT 建筑提醒"}
	for _, body := range lines[1:] {
		fields := strings.Split(body, "\n")
		for i, field := range fields {
			label, value, ok := strings.Cut(field, "：")
			if ok {
				fields[i] = "**" + escapeStructureMarkdown(label) + "：**" + escapeStructureMarkdown(value)
			} else {
				fields[i] = escapeStructureMarkdown(field)
			}
		}
		parts = append(parts, strings.Join(fields, "\n\n"))
	}
	return strings.Join(parts, "\n\n---\n\n")
}

// Only a definite rejection for unsupported native Markdown permits one
// plain-text attempt. Network and ambiguous server failures are never retried.
func sendQQStructureMarkdown(ctx context.Context, bot *OfficialQQBot, group string, lines []string) qqGroupResult {
	if bot == nil || !validQQGroupOpenID(group) || len(lines) < 2 {
		return qqGroupResult{State: "rejected"}
	}
	token, err := bot.accessToken(ctx)
	if err != nil {
		return qqGroupResult{State: "rejected"}
	}
	body, err := json.Marshal(map[string]any{"msg_type": 2, "markdown": map[string]string{"content": formatStructureMarkdown(lines)}})
	if err != nil {
		return qqGroupResult{State: "rejected"}
	}
	_, _, base := bot.configSnapshot()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v2/groups/"+url.PathEscape(group)+"/messages", bytes.NewReader(body))
	if err != nil {
		return qqGroupResult{State: "rejected"}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "QQBot "+token)
	resp, err := bot.HTTP.Do(req)
	if err != nil {
		return qqGroupResult{State: "unknown"}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16384))
	if err != nil {
		return qqGroupResult{State: "unknown"}
	}
	var result struct {
		ID      string `json:"id"`
		Code    int    `json:"code"`
		ErrCode int    `json:"err_code"`
	}
	if json.Unmarshal(data, &result) != nil {
		return qqGroupResult{State: "unknown"}
	}
	code := result.Code
	if code == 0 {
		code = result.ErrCode
	}
	if code == 50056 && resp.StatusCode >= 400 && resp.StatusCode < 500 {
		return sendQQGroupNotification(ctx, bot, group, strings.Join(lines, "\n"))
	}
	if code != 0 && resp.StatusCode < 500 {
		return qqGroupResult{State: "rejected", Code: code}
	}
	if code != 0 || resp.StatusCode/100 != 2 || result.ID == "" {
		return qqGroupResult{State: "unknown"}
	}
	return qqGroupResult{State: "sent"}
}

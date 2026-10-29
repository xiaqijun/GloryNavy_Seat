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

// qqGroupResult distinguishes a confirmed rejection from a response that may
// have been delivered. Callers must never automatically retry unknown results.
type qqGroupResult struct {
	State string
	Code  int
}

func sendQQGroupNotification(ctx context.Context, bot *OfficialQQBot, group, content string) qqGroupResult {
	if bot == nil || !validQQGroupOpenID(group) || strings.TrimSpace(content) == "" {
		return qqGroupResult{State: "rejected"}
	}
	token, err := bot.accessToken(ctx)
	if err != nil {
		return qqGroupResult{State: "rejected"} // no message request was made
	}
	body, err := json.Marshal(map[string]any{"content": content, "msg_type": 0})
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
	if err = json.Unmarshal(data, &result); err != nil {
		return qqGroupResult{State: "unknown"}
	}
	code := result.Code
	if code == 0 {
		code = result.ErrCode
	}
	if code != 0 && resp.StatusCode < 500 {
		return qqGroupResult{State: "rejected", Code: code}
	}
	if code != 0 || resp.StatusCode/100 != 2 || result.ID == "" {
		return qqGroupResult{State: "unknown"}
	}
	return qqGroupResult{State: "sent"}
}

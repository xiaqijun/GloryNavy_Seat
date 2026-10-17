package community

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

var ErrOneBotUnavailable = errors.New("NapCat 未连接或配置已变更")

type oneBotClient struct{ base, token string }
type oneBotSegment struct {
	Type string            `json:"type"`
	Data map[string]string `json:"data"`
}
type oneBotLogin struct {
	UserID   json.Number `json:"user_id"`
	Nickname string      `json:"nickname"`
}
type oneBotGroup struct {
	GroupID json.Number `json:"group_id"`
	Name    string      `json:"group_name"`
}

// OneBot has no idempotency key for sends. Never retry a timed out send.
func (c oneBotClient) call(ctx context.Context, action string, input, output any) error {
	body, err := json.Marshal(input)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/"+action, bytes.NewReader(body))
	if err != nil {
		return ErrOneBotUnavailable
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	client := &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	if err != nil {
		return ErrOneBotUnavailable
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return ErrOneBotUnavailable
	}
	var result struct {
		Status  string          `json:"status"`
		Retcode int             `json:"retcode"`
		Data    json.RawMessage `json:"data"`
	}
	if response.StatusCode != 200 || json.Unmarshal(raw, &result) != nil || result.Status != "ok" || result.Retcode != 0 {
		return ErrOneBotUnavailable
	}
	if output != nil && (len(result.Data) == 0 || string(result.Data) == "null" || json.Unmarshal(result.Data, output) != nil) {
		return ErrOneBotUnavailable
	}
	return nil
}
func (c oneBotClient) login(ctx context.Context, expected string) (oneBotLogin, error) {
	var login oneBotLogin
	if err := c.call(ctx, "get_login_info", map[string]any{}, &login); err != nil {
		return login, err
	}
	if login.UserID.String() != expected {
		return oneBotLogin{}, errors.New("机器人 QQ 号与 NapCat 登录账号不一致")
	}
	var status struct {
		Online bool `json:"online"`
		Good   bool `json:"good"`
	}
	if err := c.call(ctx, "get_status", map[string]any{}, &status); err != nil {
		return oneBotLogin{}, err
	}
	if !status.Online || !status.Good {
		return oneBotLogin{}, ErrOneBotUnavailable
	}
	return login, nil
}
func oneBotMention(qq, text string) []oneBotSegment {
	return []oneBotSegment{{Type: "at", Data: map[string]string{"qq": qq}}, {Type: "text", Data: map[string]string{"text": " " + text}}}
}
func (c oneBotClient) send(ctx context.Context, group, qq, text string) error {
	id, err := strconv.ParseInt(group, 10, 64)
	if err != nil {
		return ErrQQGroupNotConfigured
	}
	var result struct {
		MessageID json.Number `json:"message_id"`
	}
	if err = c.call(ctx, "send_group_msg", map[string]any{"group_id": id, "message": oneBotMention(qq, text)}, &result); err != nil {
		return err
	}
	if result.MessageID.String() == "" || result.MessageID.String() == "0" {
		return fmt.Errorf("NapCat 未返回消息编号")
	}
	return nil
}

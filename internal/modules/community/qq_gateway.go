package community

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coder/websocket"
)

// qqGatewayURL rejects redirects and unrelated hosts before exposing a token.
func qqGatewayURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "wss" || u.User != nil || u.Fragment != "" || u.Hostname() == "" || !(u.Hostname() == "qq.com" || strings.HasSuffix(u.Hostname(), ".qq.com")) {
		return "", errors.New("invalid QQ gateway URL")
	}
	return u.String(), nil
}

func (b *OfficialQQBot) gatewayURL(ctx context.Context, token string) (string, error) {
	_, _, base := b.configSnapshot()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/gateway", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "QQBot "+token)
	resp, err := b.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gateway status %d", resp.StatusCode)
	}
	var result struct {
		URL string `json:"url"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 16384)).Decode(&result); err != nil {
		return "", err
	}
	return qqGatewayURL(result.URL)
}

type qqGatewayFrame struct {
	Op       int             `json:"op"`
	Data     json.RawMessage `json:"d"`
	Sequence *int64          `json:"s"`
	Type     string          `json:"t"`
	ID       string          `json:"id"`
}

type qqGatewayResume struct {
	SessionID string
	Sequence  *int64
}

// qqGatewaySession is a single bounded session. Callers authenticate and
// dispatch events; it never logs messages, tokens, or credentials.
func qqGatewaySession(ctx context.Context, endpoint, token string, intents uint64, resume *qqGatewayResume, dispatch func(qqGatewayFrame, []byte) error) error {
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	dialCtx, cancelDial := context.WithTimeout(ctx, 15*time.Second)
	c, _, err := websocket.Dial(dialCtx, endpoint, &websocket.DialOptions{HTTPClient: client})
	cancelDial()
	if err != nil {
		return err
	}
	defer c.CloseNow()
	c.SetReadLimit(1 << 20)
	helloCtx, cancelHello := context.WithTimeout(ctx, 10*time.Second)
	_, raw, err := c.Read(helloCtx)
	cancelHello()
	if err != nil {
		return err
	}
	var hello qqGatewayFrame
	if json.Unmarshal(raw, &hello) != nil || hello.Op != 10 {
		return errors.New("invalid gateway hello")
	}
	var settings struct {
		Interval int64 `json:"heartbeat_interval"`
	}
	if json.Unmarshal(hello.Data, &settings) != nil || settings.Interval < 1000 || settings.Interval > 300000 {
		return errors.New("invalid heartbeat interval")
	}
	write := func(value any) error {
		payload, e := json.Marshal(value)
		if e != nil {
			return e
		}
		writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		return c.Write(writeCtx, websocket.MessageText, payload)
	}
	auth := map[string]any{"op": 2, "d": map[string]any{"token": "QQBot " + token, "intents": intents, "shard": []int{0, 1}}}
	if resume.SessionID != "" && resume.Sequence != nil {
		auth = map[string]any{"op": 6, "d": map[string]any{"token": "QQBot " + token, "session_id": resume.SessionID, "seq": *resume.Sequence}}
	}
	if err = write(auth); err != nil {
		return err
	}
	readCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	type result struct {
		data []byte
		err  error
	}
	frames := make(chan result, 1)
	go func() {
		for {
			_, data, e := c.Read(readCtx)
			select {
			case frames <- result{data, e}:
			case <-readCtx.Done():
				return
			}
			if e != nil {
				return
			}
		}
	}()
	ticker := time.NewTicker(time.Duration(settings.Interval) * time.Millisecond)
	defer ticker.Stop()
	waiting := false
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if waiting {
				return errors.New("gateway heartbeat not acknowledged")
			}
			if err = write(map[string]any{"op": 1, "d": resume.Sequence}); err != nil {
				return err
			}
			waiting = true
		case next := <-frames:
			if next.err != nil {
				return next.err
			}
			var frame qqGatewayFrame
			if json.Unmarshal(next.data, &frame) != nil {
				return errors.New("invalid gateway frame")
			}
			switch frame.Op {
			case 11:
				waiting = false
			case 7:
				return errors.New("gateway requested reconnect")
			case 9:
				var resumable bool
				if json.Unmarshal(frame.Data, &resumable) != nil || !resumable {
					resume.SessionID = ""
					resume.Sequence = nil
				}
				return errors.New("gateway invalid session")
			case 0:
				if err = dispatch(frame, next.data); err != nil {
					return err
				}
				if frame.Type == "READY" {
					var ready struct {
						SessionID string `json:"session_id"`
					}
					if json.Unmarshal(frame.Data, &ready) != nil || ready.SessionID == "" {
						return errors.New("invalid gateway ready")
					}
					resume.SessionID = ready.SessionID
				}
				if frame.Sequence != nil {
					value := *frame.Sequence
					resume.Sequence = &value
				}
			}
		}
	}
}

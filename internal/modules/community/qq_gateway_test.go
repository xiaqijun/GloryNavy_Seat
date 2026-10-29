package community

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"glorynavy.local/seat/internal/modules/community/internal/store"
)

func TestQQGatewayURL(t *testing.T) {
	for _, tc := range []struct {
		url string
		ok  bool
	}{
		{"wss://api.sgroup.qq.com/websocket", true},
		{"ws://api.sgroup.qq.com/websocket", false},
		{"wss://api.sgroup.qq.com.attacker.example/websocket", false},
		{"wss://user@api.sgroup.qq.com/websocket", false},
	} {
		_, err := qqGatewayURL(tc.url)
		if (err == nil) != tc.ok {
			t.Errorf("qqGatewayURL(%q): %v", tc.url, err)
		}
	}
}

func TestQQGatewayRetiredCallback(t *testing.T) {
	response := httptest.NewRecorder()
	retiredOfficialCallback(response, httptest.NewRequest(http.MethodPost, "/qq/official/webhook", strings.NewReader(`{"op":13}`)))
	if response.Code != http.StatusGone {
		t.Fatalf("status=%d", response.Code)
	}
}

func TestQQGatewayDoesNotFollowRedirect(t *testing.T) {
	hit := make(chan struct{}, 1)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit <- struct{}{}; w.WriteHeader(http.StatusOK) }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer redirect.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := qqGatewaySession(ctx, "ws"+strings.TrimPrefix(redirect.URL, "http"), "secret-token", 1<<25, &qqGatewayResume{}, func(qqGatewayFrame, []byte) error { return nil })
	if err == nil {
		t.Fatal("redirect accepted")
	}
	select {
	case <-hit:
		t.Fatal("redirect target contacted")
	default:
	}
}

func TestQQGatewayStopsWhileWaitingForHello(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer c.CloseNow()
		select {
		case <-r.Context().Done():
		case <-time.After(time.Second):
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if err := qqGatewaySession(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), "token", 1<<25, &qqGatewayResume{}, func(qqGatewayFrame, []byte) error { return nil }); err == nil {
		t.Fatal("hung handshake accepted")
	}
}

func TestQQGatewaySingleConsumer(t *testing.T) {
	_, pool, _, _ := qqBindingFixture(t)
	ctx := context.Background()
	first, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Release()
	second, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Release()
	locked, err := store.TryQQGatewayLock(ctx, first)
	if err != nil || !locked {
		t.Fatalf("first lock: %v %v", locked, err)
	}
	defer func() { _ = store.UnlockQQGateway(context.Background(), first) }()
	locked, err = store.TryQQGatewayLock(ctx, second)
	if err != nil || locked {
		t.Fatalf("second lock: %v %v", locked, err)
	}
	if err = store.UnlockQQGateway(ctx, first); err != nil {
		t.Fatal(err)
	}
	locked, err = store.TryQQGatewayLock(ctx, second)
	if err != nil || !locked {
		t.Fatalf("handover: %v %v", locked, err)
	}
	if err = store.UnlockQQGateway(ctx, second); err != nil {
		t.Fatal(err)
	}
}

func TestQQGatewayDoesNotAdvanceFailedEvent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer c.CloseNow()
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if c.Write(ctx, websocket.MessageText, []byte(`{"op":10,"d":{"heartbeat_interval":1000}}`)) != nil {
			return
		}
		if _, _, err = c.Read(ctx); err != nil {
			return
		}
		_ = c.Write(ctx, websocket.MessageText, []byte(`{"op":0,"t":"C2C_MESSAGE_CREATE","id":"event-1","s":11,"d":{}}`))
	}))
	defer server.Close()
	old := int64(10)
	state := &qqGatewayResume{SessionID: "session", Sequence: &old}
	want := errors.New("storage unavailable")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := qqGatewaySession(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), "token", 1<<25, state, func(qqGatewayFrame, []byte) error { return want })
	if !errors.Is(err, want) || state.Sequence == nil || *state.Sequence != 10 {
		t.Fatalf("err=%v sequence=%v", err, state.Sequence)
	}
}

func TestQQGatewaySessionIdentifiesAndDispatches(t *testing.T) {
	serverErr := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			serverErr <- err
			return
		}
		defer c.CloseNow()
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if err = c.Write(ctx, websocket.MessageText, []byte(`{"op":10,"d":{"heartbeat_interval":1000}}`)); err != nil {
			serverErr <- err
			return
		}
		_, raw, err := c.Read(ctx)
		if err != nil {
			serverErr <- err
			return
		}
		var identify struct {
			Op   int `json:"op"`
			Data struct {
				Token   string `json:"token"`
				Intents uint64 `json:"intents"`
			} `json:"d"`
		}
		if err = json.Unmarshal(raw, &identify); err != nil || identify.Op != 2 || identify.Data.Token != "QQBot test-token" || identify.Data.Intents != 1<<25 {
			serverErr <- context.Canceled
			return
		}
		serverErr <- c.Write(ctx, websocket.MessageText, []byte(`{"op":0,"t":"C2C_MESSAGE_CREATE","id":"event-1","s":1,"d":{"id":"message-1"}}`))
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	seen := false
	err := qqGatewaySession(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), "test-token", 1<<25, &qqGatewayResume{}, func(frame qqGatewayFrame, body []byte) error { seen = frame.ID == "event-1"; cancel(); return nil })
	if !seen {
		t.Fatalf("event not dispatched: %v", err)
	}
	if err = <-serverErr; err != nil {
		t.Fatal(err)
	}
}

func TestQQGatewaySessionResumes(t *testing.T) {
	requests := make(chan struct {
		Op   int `json:"op"`
		Data struct {
			SessionID string `json:"session_id"`
			Sequence  int64  `json:"seq"`
		} `json:"d"`
	}, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer c.CloseNow()
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if c.Write(ctx, websocket.MessageText, []byte(`{"op":10,"d":{"heartbeat_interval":1000}}`)) != nil {
			return
		}
		_, data, err := c.Read(ctx)
		if err != nil {
			return
		}
		var request struct {
			Op   int `json:"op"`
			Data struct {
				SessionID string `json:"session_id"`
				Sequence  int64  `json:"seq"`
			} `json:"d"`
		}
		if json.Unmarshal(data, &request) != nil {
			return
		}
		requests <- request
		if request.Op == 2 {
			_ = c.Write(ctx, websocket.MessageText, []byte(`{"op":0,"t":"READY","s":42,"d":{"session_id":"test-session"}}`))
		} else {
			_ = c.Write(ctx, websocket.MessageText, []byte(`{"op":7,"d":null}`))
		}
	}))
	defer server.Close()
	state := &qqGatewayResume{}
	endpoint := "ws" + strings.TrimPrefix(server.URL, "http")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = qqGatewaySession(ctx, endpoint, "token", 1<<25, state, func(frame qqGatewayFrame, _ []byte) error { return nil })
	if state.SessionID != "test-session" || state.Sequence == nil || *state.Sequence != 42 {
		t.Fatalf("resume state not retained: %#v", state)
	}
	_ = qqGatewaySession(ctx, endpoint, "token", 1<<25, state, func(frame qqGatewayFrame, _ []byte) error { return nil })
	first, second := <-requests, <-requests
	if first.Op != 2 || second.Op != 6 || second.Data.SessionID != "test-session" || second.Data.Sequence != 42 {
		t.Fatalf("unexpected requests: %#v %#v", first, second)
	}
}

func TestQQGatewayInvalidSessionClearsResume(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer c.CloseNow()
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if c.Write(ctx, websocket.MessageText, []byte(`{"op":10,"d":{"heartbeat_interval":1000}}`)) != nil {
			return
		}
		if _, _, err = c.Read(ctx); err != nil {
			return
		}
		_ = c.Write(ctx, websocket.MessageText, []byte(`{"op":9,"d":false}`))
	}))
	defer server.Close()
	seq := int64(7)
	state := &qqGatewayResume{SessionID: "old", Sequence: &seq}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := qqGatewaySession(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), "token", 1<<25, state, func(qqGatewayFrame, []byte) error { return nil })
	if err == nil || state.SessionID != "" || state.Sequence != nil {
		t.Fatalf("state=%#v err=%v", state, err)
	}
}

func TestQQGatewayHeartbeatAcknowledged(t *testing.T) {
	serverErr := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			serverErr <- err
			return
		}
		defer c.CloseNow()
		ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
		defer cancel()
		if err = c.Write(ctx, websocket.MessageText, []byte(`{"op":10,"d":{"heartbeat_interval":1000}}`)); err != nil {
			serverErr <- err
			return
		}
		if _, _, err = c.Read(ctx); err != nil {
			serverErr <- err
			return
		}
		for i := 0; i < 2; i++ {
			_, raw, readErr := c.Read(ctx)
			if readErr != nil {
				serverErr <- readErr
				return
			}
			var heartbeat struct {
				Op int `json:"op"`
			}
			if json.Unmarshal(raw, &heartbeat) != nil || heartbeat.Op != 1 {
				serverErr <- errors.New("expected heartbeat")
				return
			}
			if err = c.Write(ctx, websocket.MessageText, []byte(`{"op":11,"d":null}`)); err != nil {
				serverErr <- err
				return
			}
		}
		serverErr <- c.Write(ctx, websocket.MessageText, []byte(`{"op":0,"t":"EVENT","id":"ok","s":4,"d":{}}`))
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	seen := false
	_ = qqGatewaySession(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), "token", 1<<25, &qqGatewayResume{}, func(frame qqGatewayFrame, _ []byte) error { seen = frame.ID == "ok"; cancel(); return nil })
	if err := <-serverErr; err != nil {
		t.Fatal(err)
	}
	if !seen {
		t.Fatal("session did not survive heartbeat acknowledgements")
	}
}

func TestQQGatewayBindingReplayAndDisabledConfig(t *testing.T) {
	s, pool, user, code := qqBindingFixture(t)
	bot, replies := qqMessageTestBot(t)
	useOfficialQQTestBot(t, s, bot)
	message := officialQQMessage{ID: "message", Content: code}
	message.Author.UserOpenID = "user-openid"
	data, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	frame := qqGatewayFrame{Op: 0, Type: "C2C_MESSAGE_CREATE", ID: "event", Data: data}
	ctx := context.Background()
	if err = s.dispatchQQGatewayEvent(ctx, bot, frame); err != nil {
		t.Fatal(err)
	}
	if err = s.dispatchQQGatewayEvent(ctx, bot, frame); err != nil {
		t.Fatalf("replay: %v", err)
	}
	for i := 0; i < 2; i++ {
		select {
		case <-replies:
		case <-time.After(5 * time.Second):
			t.Fatal("missing passive reply")
		}
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM community_qq_bindings WHERE user_id=$1`, user).Scan(&count); err != nil || count != 1 {
		t.Fatalf("bindings=%d err=%v", count, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE community_qq_bot_settings SET enabled=false`); err != nil {
		t.Fatal(err)
	}
	frame.ID = "late"
	if err = s.dispatchQQGatewayEvent(ctx, bot, frame); !errors.Is(err, ErrQQOfficialDisabled) {
		t.Fatalf("late event: %v", err)
	}
}

func TestQQGatewayGroupRejectsDisabledConfig(t *testing.T) {
	s, pool, _, _ := qqBindingFixture(t)
	bot, _ := qqMessageTestBot(t)
	useOfficialQQTestBot(t, s, bot)
	ctx := context.Background()
	if _, err := s.SaveQQGroupSettings(ctx, []QQGroupSetting{{GroupOpenID: "opaque-group", Enabled: true}}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE community_qq_bot_settings SET enabled=false`); err != nil {
		t.Fatal(err)
	}
	event := OfficialQQGroupMemberEvent{GroupOpenID: "opaque-group", MemberOpenID: "member"}
	err := s.ProcessQQGroupMemberAdded(ctx, "ws:event", event, []byte("event"), bot)
	if !errors.Is(err, ErrQQOfficialDisabled) {
		t.Fatalf("disabled config: %v", err)
	}
}

func TestQQGatewayGroupIgnoresDisabledGroup(t *testing.T) {
	s, pool, _, _ := qqBindingFixture(t)
	bot, _ := qqMessageTestBot(t)
	useOfficialQQTestBot(t, s, bot)
	ctx := context.Background()
	if _, err := s.SaveQQGroupSettings(ctx, []QQGroupSetting{{GroupOpenID: "opaque-group", Enabled: false}}); err != nil {
		t.Fatal(err)
	}
	frame := qqGatewayFrame{Op: 0, Type: "GROUP_MEMBER_ADD", ID: "event", Data: json.RawMessage(`{"group_openid":"opaque-group","member_openid":"member"}`)}
	if err := s.dispatchQQGatewayEvent(ctx, bot, frame); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM community_bot_events WHERE source='qqgroup-member'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("events=%d err=%v", count, err)
	}
}

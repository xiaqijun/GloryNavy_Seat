package community

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"glorynavy.local/seat/internal/modules/community/internal/store"
)

// RunQQGateway is supervised by the application lifecycle. A database session
// lock prevents two application instances from identifying at the same time.
func (s *Service) RunQQGateway(ctx context.Context) {
	if s.pool == nil {
		return
	}
	if s.pool.Config().MaxConns < 2 {
		slog.Warn("QQ gateway requires at least two database connections")
		return
	}
	resume := &qqGatewayResume{}
	var revision string
	backoff := 5 * time.Second
	for ctx.Err() == nil {
		started := time.Now()
		s.runQQGatewayLocked(ctx, resume, &revision)
		if time.Since(started) >= time.Minute {
			backoff = 5 * time.Second
		} else if backoff < time.Minute {
			backoff *= 2
			if backoff > time.Minute {
				backoff = time.Minute
			}
		}
		t := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
	}
}

func (s *Service) runQQGatewayLocked(ctx context.Context, resume *qqGatewayResume, revision *string) {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return
	}
	defer conn.Release()
	locked, err := store.TryQQGatewayLock(ctx, conn)
	if err != nil || !locked {
		return
	}
	defer func() {
		unlock, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if store.UnlockQQGateway(unlock, conn) != nil {
			_ = conn.Hijack().Close(context.Background())
		}
	}()
	row, err := store.New(conn).GetQQBotSettings(ctx)
	if err != nil || !row.Enabled {
		*resume = qqGatewayResume{}
		*revision = ""
		return
	}
	secret, err := s.openQQBotSecret(row.SecretCiphertext)
	if err != nil {
		return
	}
	bot := s.qqOfficial.callbackSnapshot()
	bot.Reconfigure(row.AppID, secret, row.ApiBase)
	if !bot.Configured() {
		return
	}
	currentRevision := strconv.FormatInt(row.Revision, 10)
	if *revision != currentRevision {
		*resume = qqGatewayResume{}
		*revision = currentRevision
	}
	token, err := bot.accessToken(ctx)
	if err != nil {
		slog.Warn("QQ gateway token unavailable")
		return
	}
	endpoint, err := bot.gatewayURL(ctx, token)
	if err != nil {
		slog.Warn("QQ gateway unavailable")
		return
	}
	sessionCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	intents := uint64(1 << 25)
	if s.QQGatewayMemberEvents {
		intents |= 1 << 24
	}
	go func() {
		done <- qqGatewaySession(sessionCtx, endpoint, token, intents, resume, func(frame qqGatewayFrame, body []byte) error {
			if frame.Type == "READY" || frame.Type == "RESUMED" {
				slog.Info("QQ gateway connected", "resumed", frame.Type == "RESUMED")
				return nil
			}
			// An event must have a stable platform identity to be replay safe.
			if frame.ID == "" {
				switch frame.Type {
				case "C2C_MESSAGE_CREATE", "GROUP_AT_MESSAGE_CREATE", "GROUP_MESSAGE_CREATE", "GROUP_JOIN_REQUEST", "GROUP_MEMBER_ADD":
					return errors.New("gateway event lacks identity")
				default:
					return nil
				}
			}
			return s.dispatchQQGatewayEvent(sessionCtx, bot, frame)
		})
	}()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			cancel()
			<-done
			return
		case err = <-done:
			if err != nil && ctx.Err() == nil {
				slog.Warn("QQ gateway disconnected")
			}
			return
		case <-ticker.C:
			check, end := context.WithTimeout(ctx, 3*time.Second)
			var alive int
			err = conn.QueryRow(check, `SELECT 1`).Scan(&alive)
			current, e := s.ListQQBotSettings(check)
			end()
			if err != nil || e != nil || !current.Enabled || current.Revision != currentRevision {
				cancel()
				<-done
				return
			}
		}
	}
}

// dispatchQQGatewayEvent completes the durable effect before the sequence is
// advanced. The payload excludes the connection-specific sequence number.
func (s *Service) dispatchQQGatewayEvent(ctx context.Context, bot *OfficialQQBot, frame qqGatewayFrame) error {
	payload, err := json.Marshal(struct {
		Type string          `json:"t"`
		Data json.RawMessage `json:"d"`
	}{frame.Type, frame.Data})
	if err != nil {
		return err
	}
	eventID := "ws:" + frame.ID
	switch frame.Type {
	case "C2C_MESSAGE_CREATE", "GROUP_AT_MESSAGE_CREATE", "GROUP_MESSAGE_CREATE":
		var message officialQQMessage
		if err = json.Unmarshal(frame.Data, &message); err != nil {
			return err
		}
		scope, openid, target := "c2c", message.Author.UserOpenID, message.Author.UserOpenID
		if frame.Type != "C2C_MESSAGE_CREATE" {
			scope, openid, target = "group", message.Author.MemberOpenID, message.GroupOpenID
		}
		code := normalizeQQCommand(message.Content)
		if frame.Type == "GROUP_MESSAGE_CREATE" {
			code = standaloneQQChallenge(message.Content)
			if code == "" || message.Author.Bot || openid == "" || target == "" || message.ID == "" {
				return nil
			}
		}
		result := "绑定码无效或已过期，请先在本站生成新的 QQ 绑定码。"
		if openid != "" && code != "" {
			err = s.ConfirmQQOfficial(ctx, eventID, openid, code, payload, bot)
			switch {
			case err == nil:
				result = "QQ 已绑定本站账号，社区确认完成。"
			case errors.Is(err, ErrQQOfficialConflict):
				result = "该 QQ 机器人账号已绑定其他本站账号，请联系管理员。"
			case errors.Is(err, ErrQQChallenge):
			default:
				return err
			}
		}
		if target != "" && message.ID != "" {
			go func() {
				replyCtx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
				defer cancel()
				_ = bot.Reply(replyCtx, scope, target, message.ID, result)
			}()
		}
		return nil
	case "GROUP_JOIN_REQUEST":
		var request OfficialQQGroupJoinRequest
		if err = json.Unmarshal(frame.Data, &request); err != nil {
			return err
		}
		return s.ProcessQQGroupJoinRequest(ctx, eventID, request, payload, bot)
	case "GROUP_MEMBER_ADD":
		var event OfficialQQGroupMemberEvent
		if err = json.Unmarshal(frame.Data, &event); err != nil {
			return err
		}
		return s.ProcessQQGroupMemberAdded(ctx, eventID, event, payload, bot)
	default:
		return nil
	}
}

func (s *Service) verifyQQGatewayConfig(ctx context.Context, q *store.Queries, bot *OfficialQQBot) error {
	row, err := q.ShareQQBotSettings(ctx)
	if err != nil {
		return err
	}
	appID, secret, base := bot.configSnapshot()
	current, err := s.openQQBotSecret(row.SecretCiphertext)
	if err != nil || !row.Enabled || row.AppID != appID || row.ApiBase != base || current != secret {
		return ErrQQOfficialDisabled
	}
	return nil
}

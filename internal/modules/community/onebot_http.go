package community

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/modules/community/internal/store"
)

type oneBotEvent struct {
	Time        int64           `json:"time"`
	SelfID      json.Number     `json:"self_id"`
	PostType    string          `json:"post_type"`
	MessageType string          `json:"message_type"`
	SubType     string          `json:"sub_type"`
	GroupID     json.Number     `json:"group_id"`
	UserID      json.Number     `json:"user_id"`
	MessageID   json.Number     `json:"message_id"`
	Message     json.RawMessage `json:"message"`
	Anonymous   json.RawMessage `json:"anonymous"`
}

func (h Handler) legacyQQSync(w http.ResponseWriter, r *http.Request) {
	if selected, err := h.Service.oneBotSelected(r.Context()); err != nil || selected {
		httpapi.Failure(w, r, 503, "qq_bot_replaced", "已切换至 NapCat")
		return
	}
	h.syncQQGroupApplications(w, r)
}

func verifyOneBotSignature(secret, signature string, body []byte) bool {
	if secret == "" || !strings.HasPrefix(signature, "sha1=") {
		return false
	}
	signatureBytes, err := hex.DecodeString(strings.TrimPrefix(signature, "sha1="))
	if err != nil {
		return false
	}
	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(signatureBytes, mac.Sum(nil))
}
func (e oneBotEvent) code() string {
	if e.PostType != "message" || e.MessageType != "group" || e.SubType != "normal" || e.SelfID == e.UserID || !qqPattern.MatchString(e.UserID.String()) || !qqPattern.MatchString(e.GroupID.String()) {
		return ""
	}
	if len(e.Anonymous) > 0 && string(e.Anonymous) != "null" {
		return ""
	}
	if id, err := strconv.ParseInt(e.MessageID.String(), 10, 64); err != nil || id == 0 {
		return ""
	}
	var segments []oneBotSegment
	if json.Unmarshal(e.Message, &segments) != nil || len(segments) == 0 || len(segments) > 8 {
		return ""
	}
	text := ""
	for _, segment := range segments {
		if segment.Type != "text" {
			return ""
		}
		text += segment.Data["text"]
	}
	return standaloneQQChallenge(text)
}

func (h Handler) oneBotWebhook(w http.ResponseWriter, r *http.Request) {
	row, _, err := h.Service.oneBotConnection(r.Context())
	if err != nil {
		httpapi.Failure(w, r, 503, "onebot_unavailable", "NapCat 未启用")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
	if err != nil {
		httpapi.Failure(w, r, 400, "invalid_onebot_event", "机器人事件无效")
		return
	}
	secret, err := h.Service.openQQBotSecret(row.SecretCiphertext)
	if err != nil || !verifyOneBotSignature(secret, r.Header.Get("X-Signature"), body) {
		httpapi.Failure(w, r, 401, "invalid_onebot_signature", "机器人签名无效")
		return
	}
	var event oneBotEvent
	if json.Unmarshal(body, &event) != nil || event.SelfID.String() != row.BotQq || event.Time < time.Now().Unix()-300 || event.Time > time.Now().Unix()+60 {
		httpapi.Failure(w, r, 401, "invalid_onebot_event", "机器人身份或时间无效")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	code := event.code()
	if code == "" {
		_, _ = w.Write([]byte("{}"))
		return
	}
	reply, err := h.Service.confirmOneBot(r.Context(), row.Revision, event, code, body)
	if err != nil {
		httpapi.Failure(w, r, 503, "onebot_event_failed", "机器人事件处理失败")
		return
	}
	if reply == "" {
		_, _ = w.Write([]byte("{}"))
		return
	}
	// NapCat's standard quick operation sends these segments to this same group.
	_ = json.NewEncoder(w).Encode(map[string]any{"reply": oneBotMention(event.UserID.String(), reply), "at_sender": false, "auto_escape": false})
}

func (s *Service) confirmOneBot(ctx context.Context, revision int64, e oneBotEvent, code string, body []byte) (string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(context.Background())
	q := store.New(tx)
	row, err := q.LockOneBotSettings(ctx)
	if err != nil {
		return "", err
	}
	if !row.Enabled || row.Revision != revision || row.BotQq != e.SelfID.String() {
		return "", ErrOneBotUnavailable
	}
	allowed, err := q.OneBotGroupAllowed(ctx, e.GroupID.String())
	if err != nil || !allowed {
		return "", err
	}
	eventID := e.SelfID.String() + ":" + e.GroupID.String() + ":" + e.MessageID.String()
	if err = q.LockOneBotEvent(ctx, eventID); err != nil {
		return "", err
	}
	hash := sha256.Sum256(body)
	previous, err := q.GetBotEvent(ctx, store.GetBotEventParams{Source: "onebot", EventID: eventID})
	if err == nil {
		if !hmac.Equal(previous.PayloadHash, hash[:]) {
			return "", ErrQQBotReplay
		}
		return "", nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	if err = q.InsertBotEvent(ctx, store.InsertBotEventParams{Source: "onebot", EventID: eventID, PayloadHash: hash[:]}); err != nil {
		return "", err
	}
	finish := func(message string) (string, error) {
		if err := tx.Commit(ctx); err != nil {
			return "", err
		}
		return message, nil
	}
	// Serialize identity claims across different site accounts before locking the code.
	if err = q.LockOneBotQQ(ctx, e.UserID.String()); err != nil {
		return "", err
	}
	owner, err := q.OneBotChallengeOwner(ctx, officialChallengeHash(code))
	if errors.Is(err, pgx.ErrNoRows) {
		return finish("验证码无效或已过期")
	}
	if err != nil {
		return "", err
	}
	if s.LockAccounts == nil {
		return "", ErrOneBotUnavailable
	}
	if err = s.LockAccounts(ctx, tx, []string{owner.String()}); err != nil {
		return finish("账号状态已变更，请重新登录本站")
	}
	challenge, err := q.LockQQChallenge(ctx, officialChallengeHash(code))
	if errors.Is(err, pgx.ErrNoRows) {
		return finish("验证码无效或已过期")
	}
	if err != nil {
		return "", err
	}
	if challenge.UsedAt.Valid || !challenge.ExpiresAt.Valid || !time.Now().Before(challenge.ExpiresAt.Time) {
		return finish("验证码无效或已过期")
	}
	profile, err := q.LockProfile(ctx, challenge.UserID)
	if err != nil {
		return "", err
	}
	if profile.QqVersion != challenge.QqVersion {
		return finish("资料已更新，请重新生成验证码")
	}
	if profile.QqNumber != e.UserID.String() {
		return finish("发送者 QQ 号与本站填写的 QQ 号不一致")
	}
	existing, err := q.FindOneBotBinding(ctx, e.UserID.String())
	if err == nil && existing.UserID != challenge.UserID {
		return finish("该 QQ 已绑定其他本站账号")
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	if err = q.SaveOneBotBinding(ctx, store.SaveOneBotBindingParams{UserID: challenge.UserID, QqNumber: profile.QqNumber, QqVersion: profile.QqVersion}); err != nil {
		return "", err
	}
	if err = q.InvalidateQQConfirmations(ctx, challenge.UserID); err != nil {
		return "", err
	}
	if err = q.InsertConfirmation(ctx, store.InsertConfirmationParams{UserID: challenge.UserID, FieldVersion: profile.QqVersion, Source: "onebot", Actor: e.UserID.String(), EventID: eventID}); err != nil {
		return "", err
	}
	if err = q.MarkQQChallengeUsed(ctx, challenge.ID); err != nil {
		return "", err
	}
	return finish("QQ 绑定成功")
}

func (h Handler) getOneBotSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.Service.ListOneBotSettings(r.Context())
	if err != nil {
		httpapi.Failure(w, r, 503, "onebot_unavailable", "NapCat 配置暂不可用")
		return
	}
	httpapi.Respond(w, r, 200, settings)
}
func (h Handler) saveOneBotSettings(w http.ResponseWriter, r *http.Request) {
	var input OneBotSettingsInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF {
		httpapi.Failure(w, r, 400, "invalid_onebot_settings", "NapCat 配置无效")
		return
	}
	if err := validateOneBotInput(&input); err != nil {
		httpapi.Failure(w, r, 400, "invalid_onebot_settings", err.Error())
		return
	}
	settings, err := h.Service.SaveOneBotSettings(r.Context(), input)
	if errors.Is(err, ErrVersion) {
		httpapi.Failure(w, r, 409, "onebot_conflict", err.Error())
		return
	}
	if err != nil {
		httpapi.Failure(w, r, 400, "invalid_onebot_settings", "保存 NapCat 配置失败，请检查字段和密钥")
		return
	}
	httpapi.Respond(w, r, 200, settings)
}
func (h Handler) oneBotStatus(w http.ResponseWriter, r *http.Request) {
	status, err := h.Service.OneBotStatus(r.Context())
	if err != nil {
		httpapi.Failure(w, r, 503, "onebot_unavailable", err.Error())
		return
	}
	httpapi.Respond(w, r, 200, status)
}
func (h Handler) testOneBotMention(w http.ResponseWriter, r *http.Request) {
	var input struct {
		GroupID string `json:"group_id"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF || !qqPattern.MatchString(input.GroupID) {
		httpapi.Failure(w, r, 400, "invalid_onebot_group", "群号无效")
		return
	}
	if err := h.Service.TestOneBotMention(r.Context(), h.User(r), input.GroupID); err != nil {
		httpapi.Failure(w, r, 409, "onebot_test_failed", err.Error())
		return
	}
	httpapi.Respond(w, r, 200, map[string]bool{"sent": true})
}

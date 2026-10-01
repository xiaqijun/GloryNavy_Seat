package community

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"glorynavy.local/seat/internal/httpapi"
)

// officialQQWebhook receives the callback protocol used by q.qq.com. The
// validation handshake is handled synchronously; event work is kept small and
// the reply is sent through the official OpenAPI using the user's openid.
func (h Handler) officialQQWebhook(w http.ResponseWriter, r *http.Request) {
	if h.Service.qqOfficial == nil || !h.Service.qqOfficial.Configured() {
		httpapi.Failure(w, r, 503, "qq_bot_unavailable", "官方 QQ 机器人未配置")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
	if err != nil {
		httpapi.Failure(w, r, 400, "invalid_qq_bot_event", "机器人事件格式无效")
		return
	}
	var envelope officialQQEnvelope
	decoder := json.NewDecoder(bytes.NewReader(body))
	if decoder.Decode(&envelope) != nil {
		httpapi.Failure(w, r, 400, "invalid_qq_bot_event", "机器人事件格式无效")
		return
	}
	// The callback URL validation request (op=13) is the bootstrap handshake;
	// it carries no X-Signature-* headers.  Sign event_ts+plain_token and
	// return before applying normal event verification.
	if envelope.Op == 13 {
		var validation officialQQValidation
		if json.Unmarshal(envelope.Data, &validation) != nil || validation.PlainToken == "" || validation.EventTS == "" {
			httpapi.Failure(w, r, 400, "invalid_qq_bot_event", "机器人验证事件格式无效")
			return
		}
		signature, signErr := h.Service.qqOfficial.ValidationSignature(validation.EventTS, validation.PlainToken)
		if signErr != nil {
			httpapi.Failure(w, r, 503, "qq_bot_unavailable", "官方 QQ 机器人暂不可用")
			return
		}
		// QQ expects these two fields at the top level for op=13. Do not use
		// the site's normal {data,request_id} envelope here.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"plain_token": validation.PlainToken, "signature": signature})
		return
	}
	if err = h.Service.qqOfficial.Verify(r.Header.Get("X-Signature-Timestamp"), r.Header.Get("X-Signature-Ed25519"), body, time.Now().UTC()); err != nil {
		if errors.Is(err, ErrQQOfficialTimestamp) {
			httpapi.Failure(w, r, 401, "invalid_qq_bot_timestamp", "机器人事件已过期")
		} else {
			httpapi.Failure(w, r, 401, "invalid_qq_bot_signature", "机器人签名无效")
		}
		return
	}
	if envelope.Op == 0 {
		switch envelope.Type {
		case "C2C_MESSAGE_CREATE", "GROUP_AT_MESSAGE_CREATE":
			var message officialQQMessage
			if json.Unmarshal(envelope.Data, &message) != nil {
				httpapi.Failure(w, r, 400, "invalid_qq_bot_event", "机器人消息格式无效")
				return
			}
			scope, openid, replyTarget := "c2c", message.Author.UserOpenID, message.Author.UserOpenID
			if envelope.Type == "GROUP_AT_MESSAGE_CREATE" {
				scope, openid, replyTarget = "group", message.Author.MemberOpenID, message.GroupOpenID
			}
			code := normalizeQQCommand(message.Content)
			result := "绑定码无效或已过期，请先在本站生成新的 QQ 绑定码。"
			if openid != "" && code != "" {
				if confirmErr := h.Service.ConfirmQQOfficial(r.Context(), envelope.ID, openid, code, body); confirmErr == nil {
					result = "QQ 已绑定本站账号，社区确认完成。"
				} else if errors.Is(confirmErr, ErrQQOfficialConflict) {
					result = "该 QQ 机器人账号已绑定其他本站账号，请联系管理员。"
				}
			}
			// Official callback acknowledgements must be fast. Reply asynchronously;
			// the bot API uses the event message id for a passive response.
			if replyTarget != "" && message.ID != "" {
				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
					defer cancel()
					_ = h.Service.qqOfficial.Reply(ctx, scope, replyTarget, message.ID, result)
				}()
			}
		case "GROUP_JOIN_REQUEST":
			var request OfficialQQGroupJoinRequest
			if json.Unmarshal(envelope.Data, &request) == nil {
				payload := append([]byte(nil), body...)
				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					_ = h.Service.ProcessQQGroupJoinRequest(ctx, envelope.ID, request, payload)
				}()
			}
		case "GROUP_MEMBER_ADD":
			var event OfficialQQGroupMemberEvent
			if json.Unmarshal(envelope.Data, &event) == nil {
				payload := append([]byte(nil), body...)
				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					_ = h.Service.ProcessQQGroupMemberAdded(ctx, envelope.ID, event, payload)
				}()
			}
		}
	}
	// OP 12 is the official HTTP callback acknowledgement. Unknown events are
	// acknowledged too, so the platform does not retry unsupported event types.
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"op":12,"d":0}`))
}

func (h Handler) createOfficialQQChallenge(w http.ResponseWriter, r *http.Request) {
	code, expires, err := h.Service.CreateQQOfficialChallenge(r.Context(), h.User(r))
	if errors.Is(err, ErrQQNotBound) {
		httpapi.Failure(w, r, 409, "qq_profile_required", "请先填写 QQ 号")
		return
	}
	if err != nil {
		httpapi.Failure(w, r, 503, "community_unavailable", "绑定码生成失败，请稍后重试")
		return
	}
	httpapi.Respond(w, r, http.StatusOK, map[string]string{"code": code, "expires_at": expires.UTC().Format(time.RFC3339)})
}

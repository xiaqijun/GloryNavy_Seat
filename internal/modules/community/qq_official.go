package community

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"glorynavy.local/seat/internal/modules/community/internal/store"
)

const defaultQQBotAPIBase = "https://api.bot.qq.com"

var (
	ErrQQOfficialDisabled  = errors.New("官方 QQ 机器人未配置")
	ErrQQOfficialSignature = errors.New("官方 QQ 机器人签名无效")
	ErrQQOfficialTimestamp = errors.New("官方 QQ 机器人时间戳无效")
	ErrQQChallenge         = errors.New("QQ 绑定码无效或已过期")
	ErrQQOfficialConflict  = errors.New("官方 QQ 账号已绑定其他本站账号")
)

type qqBotToken struct {
	AccessToken string
	ExpiresAt   time.Time
}

type OfficialQQBot struct {
	AppID     string
	AppSecret string
	APIBase   string
	HTTP      *http.Client
	Groups    map[string]struct{}

	mu       sync.Mutex
	configMu sync.RWMutex
	groupsMu sync.RWMutex
	token    qqBotToken
}

func newOfficialQQBot(appID, secret, apiBase string) *OfficialQQBot {
	apiBase = strings.TrimRight(strings.TrimSpace(apiBase), "/")
	if apiBase == "" {
		apiBase = defaultQQBotAPIBase
	}
	return &OfficialQQBot{AppID: strings.TrimSpace(appID), AppSecret: strings.TrimSpace(secret), APIBase: apiBase, HTTP: &http.Client{Timeout: 8 * time.Second}, Groups: map[string]struct{}{}}
}

func (b *OfficialQQBot) configSnapshot() (string, string, string) {
	if b == nil {
		return "", "", ""
	}
	b.configMu.RLock()
	defer b.configMu.RUnlock()
	return b.AppID, b.AppSecret, b.APIBase
}

// Reconfigure applies administrator-managed credentials without replacing the
// bot object used by webhook handlers. The cached access token is discarded so
// a changed application never reuses a token from the previous application.
func (b *OfficialQQBot) Reconfigure(appID, secret, apiBase string) {
	if b == nil {
		return
	}
	apiBase = strings.TrimRight(strings.TrimSpace(apiBase), "/")
	if apiBase == "" {
		apiBase = defaultQQBotAPIBase
	}
	b.configMu.Lock()
	b.AppID = strings.TrimSpace(appID)
	b.AppSecret = strings.TrimSpace(secret)
	b.APIBase = apiBase
	b.configMu.Unlock()
	b.mu.Lock()
	b.token = qqBotToken{}
	b.mu.Unlock()
}

func (b *OfficialQQBot) Configured() bool {
	appID, secret, _ := b.configSnapshot()
	return b != nil && appID != "" && secret != ""
}

func (b *OfficialQQBot) publicKey() (ed25519.PublicKey, error) {
	if !b.Configured() {
		return nil, ErrQQOfficialDisabled
	}
	_, secret, _ := b.configSnapshot()
	seed := secret
	if secret == "" {
		return nil, ErrQQOfficialDisabled
	}
	for len(seed) < ed25519.SeedSize {
		seed += secret
	}
	return ed25519.NewKeyFromSeed([]byte(seed[:ed25519.SeedSize])).Public().(ed25519.PublicKey), nil
}

func (b *OfficialQQBot) Verify(timestamp, signature string, body []byte, now time.Time) error {
	key, err := b.publicKey()
	if err != nil {
		return err
	}
	seconds, err := strconv.ParseInt(strings.TrimSpace(timestamp), 10, 64)
	if err != nil || seconds <= 0 || now.Sub(time.Unix(seconds, 0)) > 5*time.Minute || time.Unix(seconds, 0).Sub(now) > 30*time.Second {
		return ErrQQOfficialTimestamp
	}
	sig, err := hex.DecodeString(strings.TrimSpace(signature))
	if err != nil || len(sig) != ed25519.SignatureSize {
		return ErrQQOfficialSignature
	}
	message := append([]byte(strings.TrimSpace(timestamp)), body...)
	if !ed25519.Verify(key, message, sig) {
		return ErrQQOfficialSignature
	}
	return nil
}

func (b *OfficialQQBot) ValidationSignature(eventTS, plainToken string) (string, error) {
	_, secret, _ := b.configSnapshot()
	keySeed := secret
	if keySeed == "" {
		return "", ErrQQOfficialDisabled
	}
	for len(keySeed) < ed25519.SeedSize {
		keySeed += secret
	}
	key := ed25519.NewKeyFromSeed([]byte(keySeed[:ed25519.SeedSize]))
	sig := ed25519.Sign(key, append([]byte(eventTS), []byte(plainToken)...))
	return hex.EncodeToString(sig), nil
}

func (b *OfficialQQBot) accessToken(ctx context.Context) (string, error) {
	if !b.Configured() {
		return "", ErrQQOfficialDisabled
	}
	b.mu.Lock()
	if b.token.AccessToken != "" && time.Until(b.token.ExpiresAt) > time.Minute {
		t := b.token.AccessToken
		b.mu.Unlock()
		return t, nil
	}
	b.mu.Unlock()
	appID, secret, apiBase := b.configSnapshot()
	payload, _ := json.Marshal(map[string]string{"appId": appID, "clientSecret": secret})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiBase+"/app/getAppAccessToken", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var result struct {
		Code        int             `json:"code"`
		Message     string          `json:"message"`
		AccessToken string          `json:"access_token"`
		ExpiresIn   json.RawMessage `json:"expires_in"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 16<<10)).Decode(&result); err != nil {
		return "", err
	}
	// QQ returns business failures in a JSON body even when the HTTP status is
	// 200.  Treat a missing token as a failure and keep the numeric code in the
	// error for operational diagnostics without exposing the secret.
	if resp.StatusCode/100 != 2 || result.Code != 0 || result.AccessToken == "" {
		return "", fmt.Errorf("qq bot token request failed: status=%d code=%d", resp.StatusCode, result.Code)
	}
	var expires int64
	if len(result.ExpiresIn) > 0 && result.ExpiresIn[0] == '"' {
		_ = json.Unmarshal(result.ExpiresIn, &expires)
	} else {
		_ = json.Unmarshal(result.ExpiresIn, &expires)
	}
	if expires < 60 {
		expires = 60
	}
	b.mu.Lock()
	b.token = qqBotToken{AccessToken: result.AccessToken, ExpiresAt: time.Now().Add(time.Duration(expires) * time.Second)}
	b.mu.Unlock()
	return result.AccessToken, nil
}

func (b *OfficialQQBot) Reply(ctx context.Context, scope, openid, messageID, content string) error {
	token, err := b.accessToken(ctx)
	if err != nil {
		return err
	}
	path := "/v2/users/" + url.PathEscape(openid) + "/messages"
	if scope == "group" {
		path = "/v2/groups/" + url.PathEscape(openid) + "/messages"
	}
	payload, _ := json.Marshal(map[string]any{"content": content, "msg_type": 0, "msg_id": messageID, "msg_seq": 1})
	_, _, apiBase := b.configSnapshot()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiBase+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "QQBot "+token)
	resp, err := b.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
	if readErr != nil {
		return readErr
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("qq bot reply failed: status=%d", resp.StatusCode)
	}
	var result struct {
		ErrCode int    `json:"err_code"`
		Message string `json:"message"`
		TraceID string `json:"trace_id"`
	}
	if len(bytes.TrimSpace(body)) > 0 && json.Unmarshal(body, &result) == nil && result.ErrCode != 0 {
		return fmt.Errorf("qq bot reply failed: err_code=%d trace_id=%s", result.ErrCode, result.TraceID)
	}
	return nil
}

// SetGroups limits automatic admission to explicitly configured group OpenIDs.
// QQ group OpenIDs are bot-scoped and must be copied from the official console
// or a received group event; numeric QQ group numbers are not accepted here.
func (b *OfficialQQBot) SetGroups(groups []string) {
	if b == nil {
		return
	}
	allowed := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		if group = strings.TrimSpace(group); group != "" {
			allowed[group] = struct{}{}
		}
	}
	b.groupsMu.Lock()
	b.Groups = allowed
	b.groupsMu.Unlock()
}

func (b *OfficialQQBot) GroupAllowed(group string) bool {
	if b == nil || strings.TrimSpace(group) == "" {
		return false
	}
	b.groupsMu.RLock()
	defer b.groupsMu.RUnlock()
	if len(b.Groups) == 0 {
		return false
	}
	_, ok := b.Groups[strings.TrimSpace(group)]
	return ok
}

func (b *OfficialQQBot) ConfiguredGroups() []string {
	if b == nil {
		return nil
	}
	b.groupsMu.RLock()
	defer b.groupsMu.RUnlock()
	groups := make([]string, 0, len(b.Groups))
	for group := range b.Groups {
		groups = append(groups, group)
	}
	return groups
}

type officialQQVerifyInfo struct {
	Method        string `json:"method"`
	VerifyMessage string `json:"verify_message"`
	ReviewQAList  []struct {
		Question string `json:"question"`
		Answer   string `json:"answer"`
	} `json:"review_qa_list"`
}

type OfficialQQGroupJoinRequest struct {
	GroupOpenID   string               `json:"group_openid"`
	JoinRequestID string               `json:"join_request_id"`
	MemberOpenID  string               `json:"member_openid"`
	VerifyInfo    officialQQVerifyInfo `json:"verify_info"`
}

type OfficialQQGroupMemberEvent struct {
	GroupOpenID  string `json:"group_openid"`
	MemberOpenID string `json:"member_openid"`
}

type OfficialQQGroupJoinRequestPage struct {
	List       []OfficialQQGroupJoinRequest `json:"list"`
	NextCursor string                       `json:"next_cursor"`
}

func (b *OfficialQQBot) groupAPI(ctx context.Context, method, path string, query url.Values, input any, output any) error {
	token, err := b.accessToken(ctx)
	if err != nil {
		return err
	}
	var body io.Reader
	if input != nil {
		encoded, marshalErr := json.Marshal(input)
		if marshalErr != nil {
			return marshalErr
		}
		body = bytes.NewReader(encoded)
	}
	_, _, apiBase := b.configSnapshot()
	u := apiBase + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return err
	}
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "QQBot "+token)
	resp, err := b.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 128<<10))
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("qq bot group request failed: status=%d", resp.StatusCode)
	}
	var envelope struct {
		Code    int             `json:"code"`
		ErrCode int             `json:"err_code"`
		Message string          `json:"message"`
		TraceID string          `json:"trace_id"`
		Data    json.RawMessage `json:"data"`
	}
	if len(bytes.TrimSpace(raw)) > 0 && json.Unmarshal(raw, &envelope) == nil {
		if envelope.Code != 0 || envelope.ErrCode != 0 {
			return fmt.Errorf("qq bot group request failed: code=%d err_code=%d trace_id=%s", envelope.Code, envelope.ErrCode, envelope.TraceID)
		}
	}
	if output == nil || len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, output); err == nil {
		return nil
	}
	if len(envelope.Data) > 0 && string(envelope.Data) != "null" {
		if err := json.Unmarshal(envelope.Data, output); err == nil {
			return nil
		}
	}
	return fmt.Errorf("qq bot group response format invalid")
}

func (b *OfficialQQBot) ApproveGroupJoinRequest(ctx context.Context, groupOpenID, memberOpenID, joinRequestID string) error {
	if !b.GroupAllowed(groupOpenID) {
		return fmt.Errorf("qq bot group is not configured")
	}
	if strings.TrimSpace(memberOpenID) == "" || strings.TrimSpace(joinRequestID) == "" {
		return fmt.Errorf("qq bot group request identifiers are required")
	}
	payload := map[string]any{"op": "approve", "join_request_id": strings.TrimSpace(joinRequestID)}
	return b.groupAPI(ctx, http.MethodPost, "/v2/groups/"+url.PathEscape(groupOpenID)+"/approval_join_request/"+url.PathEscape(memberOpenID), nil, payload, nil)
}

func (b *OfficialQQBot) GroupJoinRequests(ctx context.Context, groupOpenID, cursor string, limit int) (OfficialQQGroupJoinRequestPage, error) {
	if !b.GroupAllowed(groupOpenID) {
		return OfficialQQGroupJoinRequestPage{}, fmt.Errorf("qq bot group is not configured")
	}
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	query := url.Values{"limit": []string{strconv.Itoa(limit)}}
	if strings.TrimSpace(cursor) != "" {
		query.Set("cursor", strings.TrimSpace(cursor))
	}
	var page OfficialQQGroupJoinRequestPage
	err := b.groupAPI(ctx, http.MethodGet, "/v2/groups/"+url.PathEscape(groupOpenID)+"/join_request_list", query, nil, &page)
	return page, err
}

type officialQQEnvelope struct {
	Op   int             `json:"op"`
	Type string          `json:"t"`
	ID   string          `json:"id"`
	Data json.RawMessage `json:"d"`
}

type officialQQValidation struct {
	PlainToken string `json:"plain_token"`
	EventTS    string `json:"event_ts"`
}

type officialQQMessage struct {
	ID          string `json:"id"`
	Content     string `json:"content"`
	GroupOpenID string `json:"group_openid"`
	Author      struct {
		UserOpenID   string `json:"user_openid"`
		MemberOpenID string `json:"member_openid"`
	} `json:"author"`
}

func normalizeQQCommand(content string) string {
	content = strings.TrimSpace(strings.ReplaceAll(content, "\u200b", ""))
	if index := strings.LastIndex(content, "绑定"); index >= 0 {
		content = strings.TrimSpace(content[index+len("绑定"):])
	}
	fields := strings.Fields(content)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func officialChallengeHash(code string) []byte {
	h := sha256.Sum256([]byte(strings.ToUpper(strings.TrimSpace(code))))
	return h[:]
}

func newQQChallenge() (string, []byte, error) {
	var raw [6]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", nil, err
	}
	code := make([]byte, 8)
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	for i := range code {
		code[i] = alphabet[int(raw[i%len(raw)])%len(alphabet)]
	}
	value := string(code)
	return value, officialChallengeHash(value), nil
}

func (s *Service) CreateQQOfficialChallenge(ctx context.Context, userID string) (string, time.Time, error) {
	var id pgtype.UUID
	if err := id.Scan(userID); err != nil {
		return "", time.Time{}, err
	}
	code, hash, err := newQQChallenge()
	if err != nil {
		return "", time.Time{}, err
	}
	expires := time.Now().UTC().Add(10 * time.Minute)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", time.Time{}, err
	}
	defer tx.Rollback(context.Background())
	q := store.New(tx)
	if err = q.EnsureProfile(ctx, id); err != nil {
		return "", time.Time{}, err
	}
	profileRow, err := q.LockProfile(ctx, id)
	if err != nil {
		return "", time.Time{}, err
	}
	if profileRow.QqNumber == "" {
		return "", time.Time{}, ErrQQNotBound
	}
	if err = q.InvalidateQQChallenges(ctx, id); err != nil {
		return "", time.Time{}, err
	}
	if err = q.InsertQQChallenge(ctx, store.InsertQQChallengeParams{UserID: id, CodeHash: hash, QqVersion: profileRow.QqVersion, ExpiresAt: pgtype.Timestamptz{Time: expires, Valid: true}}); err != nil {
		return "", time.Time{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return "", time.Time{}, err
	}
	return code, expires, nil
}

func (s *Service) ConfirmQQOfficial(ctx context.Context, eventID, openid, code string, payload []byte) error {
	eventID, openid, code = strings.TrimSpace(eventID), strings.TrimSpace(openid), strings.TrimSpace(code)
	if eventID == "" || len(eventID) > 160 || openid == "" || len(openid) > 256 || code == "" {
		return ErrQQChallenge
	}
	hash := sha256.Sum256(payload)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	q := store.New(tx)
	if existing, lookupErr := q.GetBotEvent(ctx, store.GetBotEventParams{Source: "qqofficial", EventID: eventID}); lookupErr == nil {
		if !hmac.Equal(existing.PayloadHash, hash[:]) {
			return ErrQQBotReplay
		}
		return nil
	} else if !errors.Is(lookupErr, pgx.ErrNoRows) {
		return lookupErr
	}
	challenge, err := q.LockQQChallenge(ctx, officialChallengeHash(code))
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrQQChallenge
	}
	if err != nil {
		return err
	}
	if challenge.UsedAt.Valid || !challenge.ExpiresAt.Valid || time.Now().UTC().After(challenge.ExpiresAt.Time) {
		return ErrQQChallenge
	}
	profileRow, err := q.LockProfile(ctx, challenge.UserID)
	if err != nil {
		return err
	}
	if profileRow.QqNumber == "" || profileRow.QqVersion != challenge.QqVersion {
		return ErrQQChallenge
	}
	if existing, lookupErr := q.FindQQBinding(ctx, openid); lookupErr == nil && existing.UserID != challenge.UserID {
		return ErrQQOfficialConflict
	} else if lookupErr != nil && !errors.Is(lookupErr, pgx.ErrNoRows) {
		return lookupErr
	}
	if err = q.InsertBotEvent(ctx, store.InsertBotEventParams{Source: "qqofficial", EventID: eventID, PayloadHash: hash[:]}); err != nil {
		return err
	}
	if err = q.UpsertQQBinding(ctx, store.UpsertQQBindingParams{UserID: challenge.UserID, Openid: openid, QqVersion: challenge.QqVersion}); err != nil {
		return err
	}
	if err = q.InvalidateQQConfirmations(ctx, challenge.UserID); err != nil {
		return err
	}
	if err = q.InsertConfirmation(ctx, store.InsertConfirmationParams{UserID: challenge.UserID, FieldVersion: challenge.QqVersion, Source: "qqofficial", Actor: openid, EventID: eventID}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

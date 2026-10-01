package community

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"glorynavy.local/seat/internal/modules/community/internal/store"
)

// QQBotSettings is the safe projection returned to administrators. The bot
// secret is write-only: only whether one is configured is exposed.
type QQBotSettings struct {
	AppID            string `json:"app_id"`
	APIBase          string `json:"api_base"`
	SecretConfigured bool   `json:"secret_configured"`
	Initialized      bool   `json:"initialized"`
}

var (
	ErrQQBotSecretKey     = errors.New("QQ Bot 配置加密密钥不可用，请先配置 EVE_TOKEN_KEY")
	ErrQQBotSecretMissing = errors.New("已启用 QQ 机器人时必须填写 App Secret")
)

func validateQQBotAPIBase(raw string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(raw), "/")
	if base == "" {
		base = defaultQQBotAPIBase
	}
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil ||
		(u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost" || u.Hostname() == "::1"))) {
		return "", errors.New("QQ Bot API 地址必须是 HTTPS 地址，本地测试可使用 loopback HTTP")
	}
	return base, nil
}

func validateQQBotSettings(appID, apiBase, secret string) (string, string, error) {
	appID = strings.TrimSpace(appID)
	if !utf8.ValidString(appID) || len(appID) > 128 || strings.ContainsFunc(appID, unicode.IsControl) {
		return "", "", errors.New("QQ Bot AppID 格式无效")
	}
	apiBase, err := validateQQBotAPIBase(apiBase)
	if err != nil {
		return "", "", err
	}
	secret = strings.TrimSpace(secret)
	if len(secret) > 512 || strings.ContainsFunc(secret, unicode.IsControl) {
		return "", "", errors.New("QQ Bot App Secret 格式无效")
	}
	if appID == "" {
		// An empty AppID is the explicit disable/clear operation.
		return "", apiBase, nil
	}
	if secret == "" {
		return appID, apiBase, nil
	}
	return appID, apiBase, nil
}

func (s *Service) sealQQBotSecret(secret string) ([]byte, error) {
	if secret == "" {
		return []byte{}, nil
	}
	box := s.qqBotConfigBox()
	if box == nil {
		return nil, ErrQQBotSecretKey
	}
	nonce := make([]byte, box.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return box.Seal(nonce, nonce, []byte(secret), []byte("community.qq-bot.secret.v1")), nil
}

func (s *Service) openQQBotSecret(raw []byte) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	box := s.qqBotConfigBox()
	if box == nil || len(raw) < box.NonceSize() {
		return "", ErrQQBotSecretKey
	}
	plain, err := box.Open(nil, raw[:box.NonceSize()], raw[box.NonceSize():], []byte("community.qq-bot.secret.v1"))
	if err != nil {
		return "", ErrQQBotSecretKey
	}
	return string(plain), nil
}

func fallbackQQBotSettings(bot *OfficialQQBot) QQBotSettings {
	appID, secret, apiBase := bot.configSnapshot()
	return QQBotSettings{AppID: appID, APIBase: apiBase, SecretConfigured: secret != "", Initialized: false}
}

func (s *Service) ListQQBotSettings(ctx context.Context) (QQBotSettings, error) {
	if s.pool == nil {
		return fallbackQQBotSettings(s.qqOfficial), nil
	}
	q := store.New(s.pool)
	initialized, err := q.QQBotSettingsInitialized(ctx)
	if isUndefinedQQGroupSettings(err) {
		return fallbackQQBotSettings(s.qqOfficial), nil
	}
	if err != nil {
		return QQBotSettings{}, err
	}
	if !initialized {
		return fallbackQQBotSettings(s.qqOfficial), nil
	}
	row, err := q.GetQQBotSettings(ctx)
	if err != nil {
		return QQBotSettings{}, err
	}
	return QQBotSettings{AppID: row.AppID, APIBase: row.ApiBase, SecretConfigured: len(row.SecretCiphertext) > 0, Initialized: true}, nil
}

// LoadQQBotSettings applies the persisted administrator configuration on
// startup. Before the first save, the environment values remain a bootstrap
// fallback so an existing deployment can be moved to the UI without downtime.
func (s *Service) LoadQQBotSettings(ctx context.Context) error {
	if s.pool == nil {
		return nil
	}
	q := store.New(s.pool)
	initialized, err := q.QQBotSettingsInitialized(ctx)
	if isUndefinedQQGroupSettings(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !initialized {
		return nil
	}
	row, err := q.GetQQBotSettings(ctx)
	if err != nil {
		return err
	}
	secret, err := s.openQQBotSecret(row.SecretCiphertext)
	if err != nil {
		return fmt.Errorf("读取 QQ Bot 配置失败: %w", err)
	}
	s.SetQQBotOfficial(row.AppID, secret, row.ApiBase)
	return nil
}

// SaveQQBotSettings persists the UI-managed settings and applies them to the
// live callback adapter. An empty secret retains the current secret when the
// AppID remains enabled; clearing AppID explicitly disables and clears it.
func (s *Service) SaveQQBotSettings(ctx context.Context, appID, apiBase, secret string) (QQBotSettings, error) {
	appID, apiBase, err := validateQQBotSettings(appID, apiBase, secret)
	if err != nil {
		return QQBotSettings{}, err
	}
	_, currentSecret, _ := s.qqOfficial.configSnapshot()
	secret = strings.TrimSpace(secret)
	if appID == "" {
		secret = ""
	} else if secret == "" {
		secret = currentSecret
		if secret == "" {
			return QQBotSettings{}, ErrQQBotSecretMissing
		}
	}
	ciphertext, err := s.sealQQBotSecret(secret)
	if err != nil {
		return QQBotSettings{}, err
	}
	if s.pool == nil {
		return QQBotSettings{}, errors.New("数据库不可用")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return QQBotSettings{}, err
	}
	defer tx.Rollback(context.Background())
	if err = store.New(tx).UpsertQQBotSettings(ctx, store.UpsertQQBotSettingsParams{AppID: appID, ApiBase: apiBase, SecretCiphertext: ciphertext}); err != nil {
		if isUndefinedQQGroupSettings(err) {
			return QQBotSettings{}, errors.New("数据库迁移未完成")
		}
		return QQBotSettings{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return QQBotSettings{}, err
	}
	s.SetQQBotOfficial(appID, secret, apiBase)
	return QQBotSettings{AppID: appID, APIBase: apiBase, SecretConfigured: secret != "", Initialized: true}, nil
}

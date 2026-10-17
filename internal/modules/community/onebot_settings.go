package community

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"glorynavy.local/seat/internal/modules/community/internal/store"
)

type OneBotGroup struct {
	ID   string `json:"group_id"`
	Name string `json:"name"`
}
type OneBotSettings struct {
	Enabled          bool          `json:"enabled"`
	APIBase          string        `json:"api_base"`
	BotQQ            string        `json:"bot_qq"`
	TokenConfigured  bool          `json:"token_configured"`
	SecretConfigured bool          `json:"secret_configured"`
	Revision         string        `json:"revision"`
	Groups           []OneBotGroup `json:"groups"`
}
type OneBotSettingsInput struct {
	Enabled  bool          `json:"enabled"`
	APIBase  string        `json:"api_base"`
	BotQQ    string        `json:"bot_qq"`
	Token    string        `json:"token"`
	Secret   string        `json:"secret"`
	Revision string        `json:"revision"`
	Groups   []OneBotGroup `json:"groups"`
}

func (s *Service) oneBotSelected(ctx context.Context) (bool, error) {
	if s.pool == nil {
		return false, nil
	}
	return store.New(s.pool).OneBotSelected(ctx)
}
func (s *Service) ListOneBotSettings(ctx context.Context) (OneBotSettings, error) {
	empty := OneBotSettings{APIBase: "http://127.0.0.1:3000", Revision: "0", Groups: []OneBotGroup{}}
	if s.pool == nil {
		return empty, nil
	}
	q := store.New(s.pool)
	row, err := q.GetOneBotSettings(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return empty, nil
	}
	if err != nil {
		return empty, err
	}
	groups, err := q.ListOneBotGroups(ctx)
	if err != nil {
		return empty, err
	}
	result := OneBotSettings{Enabled: row.Enabled, APIBase: row.ApiBase, BotQQ: row.BotQq, TokenConfigured: len(row.TokenCiphertext) > 0, SecretConfigured: len(row.SecretCiphertext) > 0, Revision: strconv.FormatInt(row.Revision, 10), Groups: []OneBotGroup{}}
	for _, g := range groups {
		result.Groups = append(result.Groups, OneBotGroup{g.GroupID, g.Name})
	}
	return result, nil
}
func validateOneBotInput(input *OneBotSettingsInput) error {
	input.BotQQ = strings.TrimSpace(input.BotQQ)
	if !qqPattern.MatchString(input.BotQQ) {
		return ErrQQ
	}
	if strings.TrimSpace(input.APIBase) == "" {
		input.APIBase = "http://127.0.0.1:3000"
	}
	base, err := validateQQBotAPIBase(input.APIBase)
	if err != nil {
		return err
	}
	input.APIBase = base
	for _, secret := range []string{input.Token, input.Secret} {
		if secret != "" && (len(secret) < 32 || len(secret) > 512 || strings.ContainsFunc(secret, unicode.IsSpace) || strings.ContainsFunc(secret, unicode.IsControl)) {
			return errors.New("访问令牌和回调密钥须为 32–512 个非空白字符")
		}
	}
	if len(input.Groups) > 100 {
		return errors.New("最多配置 100 个群")
	}
	seen := map[string]bool{}
	for _, group := range input.Groups {
		if !qqPattern.MatchString(group.ID) || seen[group.ID] {
			return errors.New("群号无效或重复")
		}
		seen[group.ID] = true
		if !utf8.ValidString(group.Name) || utf8.RuneCountInString(group.Name) > 128 || strings.ContainsFunc(group.Name, unicode.IsControl) {
			return errors.New("群名称无效")
		}
	}
	return nil
}
func (s *Service) SaveOneBotSettings(ctx context.Context, input OneBotSettingsInput) (OneBotSettings, error) {
	if err := validateOneBotInput(&input); err != nil {
		return OneBotSettings{}, err
	}
	if s.pool == nil {
		return OneBotSettings{}, ErrOneBotUnavailable
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return OneBotSettings{}, err
	}
	defer tx.Rollback(context.Background())
	q := store.New(tx)
	if err = q.LockOneBotConfigWrite(ctx); err != nil {
		return OneBotSettings{}, err
	}
	old, err := q.GetOneBotSettings(ctx)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return OneBotSettings{}, err
	}
	if input.Revision != strconv.FormatInt(old.Revision, 10) {
		return OneBotSettings{}, ErrVersion
	}
	token, secret := input.Token, input.Secret
	if token == "" {
		token, err = s.openQQBotSecret(old.TokenCiphertext)
		if err != nil {
			return OneBotSettings{}, err
		}
	}
	if secret == "" {
		secret, err = s.openQQBotSecret(old.SecretCiphertext)
		if err != nil {
			return OneBotSettings{}, err
		}
	}
	if len(token) < 32 || len(secret) < 32 {
		return OneBotSettings{}, errors.New("请配置访问令牌和回调密钥")
	}
	tokenCipher, err := s.sealQQBotSecret(token)
	if err != nil {
		return OneBotSettings{}, err
	}
	secretCipher, err := s.sealQQBotSecret(secret)
	if err != nil {
		return OneBotSettings{}, err
	}
	if err = q.SaveOneBotSettings(ctx, store.SaveOneBotSettingsParams{Enabled: input.Enabled, ApiBase: input.APIBase, BotQq: input.BotQQ, TokenCiphertext: tokenCipher, SecretCiphertext: secretCipher}); err != nil {
		return OneBotSettings{}, err
	}
	if err = q.ClearOneBotGroups(ctx); err != nil {
		return OneBotSettings{}, err
	}
	for _, g := range input.Groups {
		if err = q.InsertOneBotGroup(ctx, store.InsertOneBotGroupParams{GroupID: g.ID, Name: g.Name}); err != nil {
			return OneBotSettings{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return OneBotSettings{}, err
	}
	return s.ListOneBotSettings(ctx)
}
func (s *Service) oneBotConnection(ctx context.Context) (store.CommunityOnebotSetting, oneBotClient, error) {
	if s.pool == nil {
		return store.CommunityOnebotSetting{}, oneBotClient{}, ErrOneBotUnavailable
	}
	row, err := store.New(s.pool).GetOneBotSettings(ctx)
	if err != nil || !row.Enabled {
		return row, oneBotClient{}, ErrOneBotUnavailable
	}
	token, err := s.openQQBotSecret(row.TokenCiphertext)
	if err != nil {
		return row, oneBotClient{}, ErrOneBotUnavailable
	}
	return row, oneBotClient{row.ApiBase, token}, nil
}
func (s *Service) OneBotStatus(ctx context.Context) (map[string]any, error) {
	row, c, err := s.oneBotConnection(ctx)
	if err != nil {
		return nil, err
	}
	login, err := c.login(ctx, row.BotQq)
	if err != nil {
		return nil, err
	}
	var groups []oneBotGroup
	if err = c.call(ctx, "get_group_list", map[string]any{}, &groups); err != nil {
		return nil, err
	}
	result := []OneBotGroup{}
	for _, g := range groups {
		if qqPattern.MatchString(g.GroupID.String()) {
			result = append(result, OneBotGroup{g.GroupID.String(), g.Name})
		}
	}
	return map[string]any{"online": true, "bot_qq": login.UserID.String(), "nickname": login.Nickname, "groups": result}, nil
}
func (s *Service) TestOneBotMention(ctx context.Context, userID, group string) error {
	row, c, err := s.oneBotConnection(ctx)
	if err != nil {
		return err
	}
	q := store.New(s.pool)
	allowed, err := q.OneBotGroupAllowed(ctx, group)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrQQGroupNotConfigured
	}
	var id pgtype.UUID
	if err = id.Scan(userID); err != nil {
		return err
	}
	binding, err := q.GetOneBotBinding(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return errors.New("请先通过群内验证码绑定当前账号")
	}
	if err != nil {
		return err
	}
	if _, err = c.login(ctx, row.BotQq); err != nil {
		return err
	}
	// Recheck settings after network reads; stale administrator forms cannot send to removed groups.
	current, err := q.GetOneBotSettings(ctx)
	if err != nil || !current.Enabled || current.Revision != row.Revision {
		return ErrOneBotUnavailable
	}
	currentBinding, err := q.GetOneBotBinding(ctx, id)
	if err != nil || currentBinding.QqNumber != binding.QqNumber || currentBinding.QqVersion != binding.QqVersion {
		return errors.New("请先通过群内验证码绑定当前账号")
	}
	return c.send(ctx, group, binding.QqNumber, "本站测试提醒")
}

package community

import (
	"context"
	"errors"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
	"glorynavy.local/seat/internal/modules/community/internal/store"
)

// QQGroupSetting is the administrator-managed group scope of the bot
// configuration. Bot credentials are maintained by the separate write-only
// settings endpoint and never appear in this response.
type QQGroupSetting struct {
	GroupOpenID string `json:"group_openid"`
	Label       string `json:"label"`
	Enabled     bool   `json:"enabled"`
}

type QQGroupSettings struct {
	Items       []QQGroupSetting `json:"items"`
	Initialized bool             `json:"initialized"`
}

type QQGroupOptions struct {
	Items []QQGroupSetting `json:"items"`
}

func isUndefinedQQGroupSettings(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "42P01"
}

func fallbackQQGroupSettings(bot *OfficialQQBot) []QQGroupSetting {
	if bot == nil {
		return []QQGroupSetting{}
	}
	groups := bot.ConfiguredGroups()
	items := make([]QQGroupSetting, 0, len(groups))
	for _, group := range groups {
		items = append(items, QQGroupSetting{GroupOpenID: group, Enabled: true})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].GroupOpenID < items[j].GroupOpenID })
	return items
}

func settingFromRow(row store.CommunityQqGroupSetting) QQGroupSetting {
	return QQGroupSetting{GroupOpenID: row.GroupOpenid, Label: row.Label, Enabled: row.Enabled}
}

func (s *Service) ListQQGroupSettings(ctx context.Context) (QQGroupSettings, error) {
	if s.pool == nil {
		return QQGroupSettings{Items: fallbackQQGroupSettings(s.qqOfficial)}, nil
	}
	q := store.New(s.pool)
	initialized, err := q.QQGroupSettingsInitialized(ctx)
	if isUndefinedQQGroupSettings(err) {
		return QQGroupSettings{Items: fallbackQQGroupSettings(s.qqOfficial)}, nil
	}
	if err != nil {
		return QQGroupSettings{}, err
	}
	if !initialized {
		return QQGroupSettings{Items: fallbackQQGroupSettings(s.qqOfficial)}, nil
	}
	rows, err := q.ListQQGroupSettings(ctx)
	if err != nil {
		return QQGroupSettings{}, err
	}
	items := make([]QQGroupSetting, 0, len(rows))
	for _, row := range rows {
		items = append(items, settingFromRow(row))
	}
	return QQGroupSettings{Items: items, Initialized: true}, nil
}

// LoadQQBotGroups applies persisted settings on startup. Before the migration
// exists, or before an admin saves settings, the environment list remains the
// safe bootstrap source.
func (s *Service) LoadQQBotGroups(ctx context.Context) error {
	settings, err := s.ListQQGroupSettings(ctx)
	if err != nil {
		return err
	}
	if !settings.Initialized {
		return nil
	}
	groups := make([]string, 0, len(settings.Items))
	for _, item := range settings.Items {
		if item.Enabled {
			groups = append(groups, item.GroupOpenID)
		}
	}
	s.SetQQBotGroups(groups)
	return nil
}

func (s *Service) ListQQGroupOptions(ctx context.Context) (QQGroupOptions, error) {
	settings, err := s.ListQQGroupSettings(ctx)
	if err != nil {
		return QQGroupOptions{}, err
	}
	items := make([]QQGroupSetting, 0, len(settings.Items))
	for _, item := range settings.Items {
		if item.Enabled {
			items = append(items, item)
		}
	}
	return QQGroupOptions{Items: items}, nil
}

func validateQQGroupSettings(items []QQGroupSetting) ([]QQGroupSetting, error) {
	if len(items) > 100 {
		return nil, errors.New("QQ 入群审批群组最多配置 100 个")
	}
	seen := make(map[string]struct{}, len(items))
	result := make([]QQGroupSetting, 0, len(items))
	for _, item := range items {
		item.GroupOpenID = strings.TrimSpace(item.GroupOpenID)
		item.Label = strings.TrimSpace(item.Label)
		if !validQQGroupOpenID(item.GroupOpenID) {
			return nil, errors.New("群 OpenID 格式无效")
		}
		if !utf8.ValidString(item.Label) || utf8.RuneCountInString(item.Label) > 128 || strings.ContainsFunc(item.Label, unicode.IsControl) {
			return nil, errors.New("群名称格式无效，不能超过 128 个字符")
		}
		if _, ok := seen[item.GroupOpenID]; ok {
			return nil, errors.New("群 OpenID 不能重复")
		}
		seen[item.GroupOpenID] = struct{}{}
		result = append(result, item)
	}
	return result, nil
}

func (s *Service) SaveQQGroupSettings(ctx context.Context, items []QQGroupSetting) (QQGroupSettings, error) {
	items, err := validateQQGroupSettings(items)
	if err != nil {
		return QQGroupSettings{}, err
	}
	if s.pool == nil {
		return QQGroupSettings{}, errors.New("数据库不可用")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return QQGroupSettings{}, err
	}
	defer tx.Rollback(context.Background())
	q := store.New(tx)
	if err = q.ReplaceQQGroupSettings(ctx); err != nil {
		if isUndefinedQQGroupSettings(err) {
			return QQGroupSettings{}, errors.New("数据库迁移未完成")
		}
		return QQGroupSettings{}, err
	}
	for _, item := range items {
		if err = q.InsertQQGroupSetting(ctx, store.InsertQQGroupSettingParams{GroupOpenid: item.GroupOpenID, Label: item.Label, Enabled: item.Enabled}); err != nil {
			return QQGroupSettings{}, err
		}
	}
	if err = q.MarkQQGroupSettingsInitialized(ctx); err != nil {
		return QQGroupSettings{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return QQGroupSettings{}, err
	}
	groups := make([]string, 0, len(items))
	for _, item := range items {
		if item.Enabled {
			groups = append(groups, item.GroupOpenID)
		}
	}
	s.SetQQBotGroups(groups)
	return QQGroupSettings{Items: items, Initialized: true}, nil
}

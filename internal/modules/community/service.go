// Package community owns member-supplied community details and confirmation state.
package community

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/modules/community/internal/store"
)

var (
	ErrQQ                       = errors.New("QQ 号需为 5–12 位数字，且不能以 0 开头")
	ErrKOOK                     = errors.New("KOOK 昵称需为 1–64 个字符，不能包含控制字符")
	ErrVersion                  = errors.New("资料已在其他页面更新，请重新读取后修改")
	ErrQQBotReplay              = errors.New("QQ 机器人事件重放冲突")
	ErrQQNotBound               = errors.New("QQ 号尚未绑定本站账号")
	ErrQQGroupNotConfigured     = errors.New("QQ 入群审批群组未配置")
	ErrQQGroupApplicationExists = errors.New("该群已有待处理的入群申请")
	qqPattern                   = regexp.MustCompile(`^[1-9][0-9]{4,11}$`)
)

type Service struct {
	pool        *pgxpool.Pool
	qqOfficial  *OfficialQQBot
	qqConfigMu  sync.RWMutex
	qqConfigBox cipher.AEAD
}

func New(pool *pgxpool.Pool) *Service {
	// Keep one stable bot object so a runtime configuration update cannot race
	// webhook handlers that already hold its pointer.
	return &Service{pool: pool, qqOfficial: newOfficialQQBot("", "", defaultQQBotAPIBase)}
}

// SetQQBotOfficial enables the official QQ open platform callback adapter.
// App credentials stay in memory and are never returned by an API.
func (s *Service) SetQQBotOfficial(appID, appSecret, apiBase string) {
	if strings.TrimSpace(apiBase) == "" {
		apiBase = defaultQQBotAPIBase
	}
	if s.qqOfficial == nil {
		s.qqOfficial = newOfficialQQBot(appID, appSecret, apiBase)
		return
	}
	s.qqOfficial.Reconfigure(appID, appSecret, apiBase)
}

// SetQQBotConfigKey derives a separate encryption key for the QQ Bot secret
// from the existing server-side EVE token key. The derived key never leaves
// the process; it is deliberately unavailable to browser clients.
func (s *Service) SetQQBotConfigKey(encoded string) error {
	encoded = strings.TrimSpace(encoded)
	s.qqConfigMu.Lock()
	defer s.qqConfigMu.Unlock()
	if encoded == "" {
		s.qqConfigBox = nil
		return nil
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(raw) != 32 {
		return errors.New("QQ Bot 配置加密密钥无效")
	}
	derived := sha256.Sum256(append([]byte("glorynavy:qq-bot-config:v1:"), raw...))
	block, err := aes.NewCipher(derived[:])
	if err != nil {
		return err
	}
	s.qqConfigBox, err = cipher.NewGCM(block)
	return err
}

func (s *Service) qqBotConfigBox() cipher.AEAD {
	s.qqConfigMu.RLock()
	defer s.qqConfigMu.RUnlock()
	return s.qqConfigBox
}

// SetQQBotGroups configures the bot-scoped group OpenIDs eligible for
// automatic admission. Numeric QQ group numbers are deliberately not accepted.
func (s *Service) SetQQBotGroups(groups []string) {
	if s.qqOfficial != nil {
		s.qqOfficial.SetGroups(groups)
	}
}

type Binding struct {
	Value        string `json:"value"`
	Version      string `json:"version"`
	Confirmation string `json:"confirmation"`
}
type Profile struct {
	Version  string  `json:"version"`
	Complete bool    `json:"complete"`
	QQ       Binding `json:"qq"`
	KOOK     Binding `json:"kook"`
}

func binding(value string, version int64, confirmed bool) Binding {
	status := "pending"
	if value == "" {
		status = "unfilled"
	} else if confirmed {
		status = "confirmed"
	}
	return Binding{value, strconv.FormatInt(version, 10), status}
}
func profile(row store.GetProfileRow) Profile {
	return Profile{strconv.FormatInt(row.Version, 10), row.QqNumber != "" && row.KookName != "", binding(row.QqNumber, row.QqVersion, row.QqConfirmed), binding(row.KookName, row.KookVersion, row.KookConfirmed)}
}
func (s *Service) Get(ctx context.Context, userID string) (Profile, error) {
	var id pgtype.UUID
	if err := id.Scan(userID); err != nil {
		return Profile{}, err
	}
	row, err := store.New(s.pool).GetProfile(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return profile(store.GetProfileRow{}), nil
	}
	if err != nil {
		return Profile{}, err
	}
	return profile(row), nil
}
func (s *Service) Complete(ctx context.Context, userID string) (bool, error) {
	p, err := s.Get(ctx, userID)
	return p.Complete, err
}
func (s *Service) Update(ctx context.Context, userID, qq, kook, version string) (Profile, error) {
	qq = strings.TrimSpace(qq)
	kook = strings.TrimSpace(kook)
	if !qqPattern.MatchString(qq) {
		return Profile{}, ErrQQ
	}
	if !utf8.ValidString(kook) || utf8.RuneCountInString(kook) < 1 || utf8.RuneCountInString(kook) > 64 || strings.ContainsFunc(kook, unicode.IsControl) {
		return Profile{}, ErrKOOK
	}
	v, err := strconv.ParseInt(version, 10, 64)
	if err != nil || v < 0 || strconv.FormatInt(v, 10) != version {
		return Profile{}, ErrVersion
	}
	var id pgtype.UUID
	if err = id.Scan(userID); err != nil {
		return Profile{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Profile{}, err
	}
	defer tx.Rollback(context.Background())
	q := store.New(tx)
	if err = q.EnsureProfile(ctx, id); err != nil {
		return Profile{}, err
	}
	old, err := q.LockProfile(ctx, id)
	if err != nil {
		return Profile{}, err
	}
	if old.Version != v {
		return Profile{}, ErrVersion
	}
	changed := []string{}
	if old.QqNumber != qq {
		changed = append(changed, "qq")
	}
	if old.KookName != kook {
		changed = append(changed, "kook")
	}
	if len(changed) > 0 {
		if err = q.UpdateProfile(ctx, store.UpdateProfileParams{UserID: id, QqNumber: qq, KookName: kook}); err != nil {
			return Profile{}, err
		}
		if err = q.InvalidateConfirmations(ctx, store.InvalidateConfirmationsParams{UserID: id, Column2: changed}); err != nil {
			return Profile{}, err
		}
		if err = q.RecordProfileChange(ctx, store.RecordProfileChangeParams{UserID: id, ProfileVersion: v + 1, ChangedPlatforms: changed}); err != nil {
			return Profile{}, err
		}
	}
	row, err := q.GetProfile(ctx, id)
	if err != nil {
		return Profile{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Profile{}, err
	}
	return profile(row), nil
}

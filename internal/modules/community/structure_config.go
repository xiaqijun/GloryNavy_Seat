package community

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"glorynavy.local/seat/internal/modules/community/internal/store"
)

var ErrStructureWatchInvalid = errors.New("建筑提醒配置无效")
var ErrStructureWatchForbidden = errors.New("无权管理建筑提醒")
var ErrStructureWatchBot = errors.New("请先启用官方 QQ 机器人并确认群 OpenID")
var ErrStructureWatchSource = errors.New("无法读取所选军团的建筑")

type StructureWatchSettings struct {
	Enabled         bool   `json:"enabled"`
	GroupOpenID     string `json:"group_openid"`
	CorporationID   string `json:"corporation_id"`
	CorporationName string `json:"corporation_name"`
	Revision        string `json:"revision"`
	BotReady        bool   `json:"bot_ready"`
	BotChanged      bool   `json:"bot_changed"`
}

type StructureWatchInput struct {
	Enabled       bool   `json:"enabled"`
	GroupOpenID   string `json:"group_openid"`
	CorporationID string `json:"corporation_id"`
	Revision      string `json:"revision"`
}

type StructureDelivery = store.StructureDelivery

func (r *StructureJobs) structureAdmin(ctx context.Context, actor string) error {
	if r == nil || !r.Enabled || r.Service == nil || r.Service.pool == nil || r.Administrator == nil || r.Read == nil {
		return ErrStructureWatchForbidden
	}
	ok, err := r.Administrator(ctx, actor)
	if err != nil {
		return err
	}
	if !ok {
		return ErrStructureWatchForbidden
	}
	return nil
}

func (r *StructureJobs) Settings(ctx context.Context, actor string) (StructureWatchSettings, error) {
	if err := r.structureAdmin(ctx, actor); err != nil {
		return StructureWatchSettings{}, err
	}
	w, err := store.ReadStructureWatch(ctx, r.Service.pool)
	if err != nil {
		return StructureWatchSettings{}, err
	}
	bot, err := r.Service.ListQQBotSettings(ctx)
	if err != nil {
		return StructureWatchSettings{}, err
	}
	name := ""
	if w.CorporationID > 0 {
		if rows, readErr := r.Read(ctx, actor, w.CorporationID); readErr == nil && len(rows) > 0 {
			name = rows[0].CorporationName
		}
	}
	return StructureWatchSettings{Enabled: w.Enabled, GroupOpenID: w.Group, CorporationID: strconv.FormatInt(w.CorporationID, 10), CorporationName: name, Revision: strconv.FormatInt(w.Revision, 10), BotReady: bot.Enabled && bot.SecretConfigured, BotChanged: w.AppID != "" && w.AppID != bot.AppID}, nil
}

func (r *StructureJobs) SaveSettings(ctx context.Context, actor string, input StructureWatchInput) (StructureWatchSettings, error) {
	if err := r.structureAdmin(ctx, actor); err != nil {
		return StructureWatchSettings{}, err
	}
	input.GroupOpenID = strings.TrimSpace(input.GroupOpenID)
	corp, err := strconv.ParseInt(input.CorporationID, 10, 64)
	if err != nil || corp < 0 || strconv.FormatInt(corp, 10) != input.CorporationID {
		return StructureWatchSettings{}, ErrStructureWatchInvalid
	}
	revision, err := strconv.ParseInt(input.Revision, 10, 64)
	if err != nil || revision < 1 || strconv.FormatInt(revision, 10) != input.Revision {
		return StructureWatchSettings{}, ErrStructureWatchInvalid
	}
	if input.GroupOpenID != "" && !validQQGroupOpenID(input.GroupOpenID) {
		return StructureWatchSettings{}, ErrStructureWatchInvalid
	}
	if input.Enabled && (corp == 0 || input.GroupOpenID == "") {
		return StructureWatchSettings{}, ErrStructureWatchInvalid
	}
	if input.Enabled {
		if _, err := r.Read(ctx, actor, corp); err != nil {
			return StructureWatchSettings{}, ErrStructureWatchSource
		}
	}
	tx, err := r.Service.pool.Begin(ctx)
	if err != nil {
		return StructureWatchSettings{}, err
	}
	defer tx.Rollback(context.Background())
	var current store.StructureWatch
	err = tx.QueryRow(ctx, `SELECT enabled,app_id,group_openid,coalesce(owner_id::text,''),corporation_id,revision FROM community_qq_structure_watch WHERE id FOR UPDATE`).Scan(&current.Enabled, &current.AppID, &current.Group, &current.Owner, &current.CorporationID, &current.Revision)
	if err != nil {
		return StructureWatchSettings{}, err
	}
	if current.Revision != revision {
		return StructureWatchSettings{}, ErrVersion
	}
	if current.Enabled == input.Enabled && current.Group == input.GroupOpenID && current.CorporationID == corp {
		if err = tx.Commit(ctx); err != nil {
			return StructureWatchSettings{}, err
		}
		return r.Settings(ctx, actor)
	}
	appID, owner := current.AppID, current.Owner
	if input.Enabled || (input.GroupOpenID != "" && input.GroupOpenID != current.Group) {
		bot, e := store.New(tx).ShareQQBotSettings(ctx)
		if e != nil {
			return StructureWatchSettings{}, e
		}
		if input.Enabled && (!bot.Enabled || bot.AppID == "" || len(bot.SecretCiphertext) == 0 || (current.AppID != "" && current.AppID != bot.AppID && input.GroupOpenID == current.Group)) {
			return StructureWatchSettings{}, ErrStructureWatchBot
		}
		if bot.AppID != "" {
			appID = bot.AppID
		}
	}
	if r.Service.LockAccounts == nil {
		return StructureWatchSettings{}, ErrStructureWatchForbidden
	}
	if err = r.Service.LockAccounts(ctx, tx, []string{actor}); err != nil {
		return StructureWatchSettings{}, err
	}
	if err = r.structureAdmin(ctx, actor); err != nil {
		return StructureWatchSettings{}, err
	}
	if input.Enabled || (corp > 0 && corp != current.CorporationID) {
		if _, e := r.Read(ctx, actor, corp); e != nil {
			return StructureWatchSettings{}, ErrStructureWatchSource
		}
	}
	if input.Enabled {
		owner = actor
	}
	_, err = tx.Exec(ctx, `UPDATE community_qq_structure_watch SET enabled=$1,app_id=$2,group_openid=$3,corporation_id=$4,owner_id=NULLIF($5,'')::uuid,revision=revision+1,updated_at=now() WHERE id`, input.Enabled, appID, input.GroupOpenID, corp, owner)
	if err != nil {
		return StructureWatchSettings{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return StructureWatchSettings{}, err
	}
	return r.Settings(ctx, actor)
}

func (r *StructureJobs) Deliveries(ctx context.Context, actor string) ([]StructureDelivery, error) {
	if err := r.structureAdmin(ctx, actor); err != nil {
		return nil, err
	}
	w, err := store.ReadStructureWatch(ctx, r.Service.pool)
	if err != nil {
		return nil, err
	}
	if w.CorporationID <= 0 {
		return []StructureDelivery{}, nil
	}
	if _, err = r.Read(ctx, actor, w.CorporationID); err != nil {
		return nil, ErrStructureWatchSource
	}
	return store.StructureDeliveries(ctx, r.Service.pool, w.CorporationID, w.Revision)
}

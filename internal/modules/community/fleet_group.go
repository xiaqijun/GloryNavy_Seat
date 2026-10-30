package community

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"glorynavy.local/seat/internal/modules/community/internal/store"
)

type FleetGroupResult struct{ State, Reason string }

func fleetGroupVersion(appID, group string, revision int64) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%d", appID, group, revision)))
	return hex.EncodeToString(h[:])
}

func (r *StructureJobs) FleetGroupTarget(ctx context.Context, corporation int64) (string, error) {
	if r == nil || !r.Enabled || r.Service == nil || r.Service.pool == nil || r.Read == nil || r.Administrator == nil {
		return "", errors.New("group unavailable")
	}
	w, err := store.ReadStructureWatch(ctx, r.Service.pool)
	if err != nil || !w.Enabled || w.CorporationID != corporation || w.Owner == "" || w.AppID == "" || !validQQGroupOpenID(w.Group) {
		return "", errors.New("group unavailable")
	}
	allowed, err := r.Administrator(ctx, w.Owner)
	if err != nil || !allowed {
		return "", errors.New("group unavailable")
	}
	if _, err = r.Read(ctx, w.Owner, corporation); err != nil {
		return "", errors.New("group unavailable")
	}
	return fleetGroupVersion(w.AppID, w.Group, w.Revision), nil
}

// FleetGroup sends only to the currently configured, corporation-scoped group.
// The caller owns the durable delivery claim and must not retry unknown results.
func (r *StructureJobs) FleetGroup(ctx context.Context, corporation int64, expected, body string) FleetGroupResult {
	fail := func(reason string) FleetGroupResult { return FleetGroupResult{State: "failed", Reason: reason} }
	if r == nil || !r.Enabled || r.Service == nil || r.Service.pool == nil || r.Read == nil || r.Administrator == nil {
		return fail("service_unavailable")
	}
	w, err := store.ReadStructureWatch(ctx, r.Service.pool)
	if err != nil || !w.Enabled || w.CorporationID != corporation || w.Owner == "" || w.AppID == "" || !validQQGroupOpenID(w.Group) {
		return fail("group_unavailable")
	}
	if fleetGroupVersion(w.AppID, w.Group, w.Revision) != expected {
		return fail("group_changed")
	}
	allowed, err := r.Administrator(ctx, w.Owner)
	if err != nil || !allowed {
		return fail("group_unavailable")
	}
	if _, err = r.Read(ctx, w.Owner, corporation); err != nil {
		return fail("group_unavailable")
	}
	bot := r.Service.qqOfficial.callbackSnapshot()
	if bot == nil {
		return fail("bot_unavailable")
	}
	appID, _, _ := bot.configSnapshot()
	if appID != w.AppID || r.Service.verifyQQGatewayConfig(ctx, store.New(r.Service.pool), bot) != nil {
		return fail("bot_unavailable")
	}
	// Hold the same configuration and account locks as structure notifications
	// across the external request, after the caller has durably claimed delivery.
	tx, err := r.Service.pool.Begin(ctx)
	if err != nil {
		return fail("service_unavailable")
	}
	defer tx.Rollback(context.Background())
	var active bool
	err = tx.QueryRow(ctx, `SELECT enabled AND revision=$1 AND app_id=$2 AND group_openid=$3 AND corporation_id=$4 AND owner_id::text=$5 FROM community_qq_structure_watch WHERE id FOR UPDATE`, w.Revision, w.AppID, w.Group, corporation, w.Owner).Scan(&active)
	if err != nil || !active {
		return fail("group_changed")
	}
	if r.Service.verifyQQGatewayConfig(ctx, store.New(tx), bot) != nil {
		return fail("bot_unavailable")
	}
	if r.Service.LockAccounts == nil || r.Service.LockAccounts(ctx, tx, []string{w.Owner}) != nil {
		return fail("group_unavailable")
	}
	allowed, err = r.Administrator(ctx, w.Owner)
	if err != nil || !allowed {
		return fail("group_unavailable")
	}
	if _, err = r.Read(ctx, w.Owner, corporation); err != nil {
		return fail("group_unavailable")
	}
	result := sendQQGroupNotification(ctx, bot, w.Group, body)
	if tx.Commit(ctx) != nil {
		return FleetGroupResult{State: "unknown", Reason: "confirmation_required"}
	}
	if result.State == "sent" {
		return FleetGroupResult{State: "sent"}
	}
	if result.State == "rejected" {
		return fail(fmt.Sprintf("qq_rejected_%d", result.Code))
	}
	return FleetGroupResult{State: "unknown", Reason: "confirmation_required"}
}

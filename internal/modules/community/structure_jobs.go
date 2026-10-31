package community

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"glorynavy.local/seat/internal/modules/community/internal/store"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/structures"
	"glorynavy.local/seat/internal/platform/jobs"
)

// StructureJobs receives only the same currently authorized projection used by
// the structures page. It never reads another module's store or calls ESI.
type StructureJobs struct {
	Service       *Service
	Enabled       bool
	Read          func(context.Context, string, int64) ([]eve.Structure, error)
	Administrator func(context.Context, string) (bool, error)
}

type structureScanArgs struct{}

func (structureScanArgs) Kind() string { return "community.qq-structure-scan.v1" }

type structureScanWorker struct {
	river.WorkerDefaults[structureScanArgs]
	runtime *StructureJobs
}

func (r *StructureJobs) Extension() jobs.Extension {
	return jobs.Extension{Register: func(w *river.Workers) []*river.PeriodicJob {
		river.AddWorker(w, &structureScanWorker{runtime: r})
		if !r.Enabled {
			return nil
		}
		return []*river.PeriodicJob{river.NewPeriodicJob(river.PeriodicInterval(time.Minute), func() (river.JobArgs, *river.InsertOpts) {
			return structureScanArgs{}, &river.InsertOpts{Queue: "community", UniqueOpts: jobs.ActiveUnique(), MaxAttempts: 3}
		}, &river.PeriodicJobOpts{RunOnStart: true})}
	}, Bind: func(*river.Client[pgx.Tx]) {}}
}

func (w *structureScanWorker) Work(ctx context.Context, _ *river.Job[structureScanArgs]) error {
	r := w.runtime
	if !r.Enabled || r.Read == nil || r.Administrator == nil || r.Service == nil || r.Service.pool == nil {
		return river.JobSnooze(time.Hour)
	}
	if r.Service.pool.Config().MaxConns < 3 {
		slog.Warn("QQ structure notification requires at least three database connections")
		return river.JobSnooze(time.Hour)
	}
	return r.scan(ctx)
}

func cleanStructureText(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > 100 {
		return string(r[:100]) + "…"
	}
	return s
}

// structureNoticeDetails uses only the authorized snapshot. A POS has no
// custom name in ESI, so its type and ID identify it without inventing a name.
func structureNoticeDetails(item eve.Structure) string {
	name := cleanStructureText(item.Name)
	typeName := cleanStructureText(item.TypeName)
	if name == "" {
		name = fmt.Sprintf("编号 %d", item.ID)
	}
	if typeName == "" {
		typeName = "未知"
		if item.TypeID > 0 {
			typeName += fmt.Sprintf("（ID %d）", item.TypeID)
		}
	}
	text := "建筑：" + name + "\n建筑类型：" + typeName
	if system := cleanStructureText(item.SolarSystemName); system != "" {
		text += "\n星系：" + system
	}
	return text
}

func structureNoticeTimes(item eve.Structure) string {
	const layout = "2006-01-02 15:04"
	beijing := time.FixedZone("UTC+8", 8*60*60)
	text := ""
	if item.FuelExpires != nil {
		label := "燃料到期"
		if item.Kind == "pos" {
			label = "燃料预计耗尽"
		}
		text += "\n" + label + "：" + item.FuelExpires.In(beijing).Format(layout) + "（北京时间）"
	}
	if item.UnanchorsAt != nil {
		text += "\n计划拆除：" + item.UnanchorsAt.In(beijing).Format(layout) + "（北京时间）"
	}
	return text
}

func structureReference(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

func structureStateName(state string) string {
	labels := map[string]string{
		"online": "在线", "offline": "离线", "anchoring": "锚定中",
		"shield_vulnerable": "可攻击（护盾）", "armor_vulnerable": "可攻击（装甲）",
		"hull_vulnerable": "可攻击（结构）", "armor_reinforce": "装甲增强",
		"hull_reinforce": "结构增强", "unanchoring": "拆除中", "unanchored": "已拆除",
	}
	if label := labels[state]; label != "" {
		return label
	}
	return cleanStructureText(state)
}

func fuelStage(expires *time.Time, now time.Time) string {
	if expires == nil {
		return ""
	}
	left := expires.Sub(now)
	if left <= 0 {
		return "已耗尽"
	}
	if left <= 24*time.Hour {
		return "24 小时内"
	}
	if left <= 72*time.Hour {
		return "72 小时内"
	}
	return ""
}

func structureActiveReferences(rows []eve.Structure, corporationID int64, now time.Time) (map[int64]bool, map[string]bool) {
	ids, refs := map[int64]bool{}, map[string]bool{}
	for _, item := range rows {
		if item.CorporationID != corporationID || item.ID <= 0 || item.ObservedAt.IsZero() {
			continue
		}
		ids[item.ID] = true
		if stage := fuelStage(item.FuelExpires, now); stage != "" {
			refs[structureReference("fuel", fmt.Sprint(item.ID), item.FuelExpires.UTC().Format("2006-01-02"), stage)] = true
		}
		if item.UnanchorsAt != nil && item.UnanchorsAt.After(now) {
			refs[structureReference("unanchor", fmt.Sprint(item.ID), item.UnanchorsAt.UTC().Format(time.RFC3339))] = true
		}
	}
	return ids, refs
}

func (r *StructureJobs) scan(ctx context.Context) error {
	s := r.Service
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	var locked bool
	if err = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtext('community.qq.structure.scan'))`).Scan(&locked); err != nil || !locked {
		return err
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		var ok bool
		if conn.QueryRow(c, `SELECT pg_advisory_unlock(hashtext('community.qq.structure.scan'))`).Scan(&ok) != nil || !ok {
			_ = conn.Hijack().Close(context.Background())
		}
	}()
	watch, err := store.ReadStructureWatch(ctx, conn)
	if err != nil || !watch.Enabled {
		return err
	}
	if watch.Owner == "" || watch.CorporationID <= 0 || watch.AppID == "" || !validQQGroupOpenID(watch.Group) {
		return nil
	}
	botSettings, err := s.ListQQBotSettings(ctx)
	if err != nil || !botSettings.Enabled || botSettings.AppID != watch.AppID {
		return err
	}
	admin, err := r.Administrator(ctx, watch.Owner)
	if err != nil || !admin {
		return err
	}
	rows, err := r.Read(ctx, watch.Owner, watch.CorporationID)
	if err != nil {
		return err
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	var active bool
	if err = tx.QueryRow(ctx, `SELECT enabled AND revision=$1 AND app_id=$2 AND group_openid=$3 AND corporation_id=$4 AND owner_id::text=$5 FROM community_qq_structure_watch WHERE id FOR SHARE`, watch.Revision, watch.AppID, watch.Group, watch.CorporationID, watch.Owner).Scan(&active); err != nil || !active {
		return err
	}
	now := time.Now().UTC()
	for _, item := range rows {
		if item.CorporationID != watch.CorporationID || item.ID <= 0 || item.ObservedAt.IsZero() {
			continue
		}
		details := structureNoticeDetails(item)
		times := structureNoticeTimes(item)
		old, e := store.ReadStructureObservation(ctx, tx, watch.Revision, watch.CorporationID, item.ID)
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if e == nil && item.ObservedAt.After(old.Observed) {
			var services []eve.StructureService
			if err = json.Unmarshal(old.Services, &services); err != nil {
				return err
			}
			previous := eve.Structure{ID: item.ID, CorporationID: item.CorporationID, ObservedAt: old.Observed, State: old.State, Services: services}
			for _, change := range structures.Transitions([]eve.Structure{previous}, []eve.Structure{item}) {
				field := "建筑状态"
				if strings.HasPrefix(change.Field, "service:") {
					field = "服务 " + cleanStructureText(strings.TrimPrefix(change.Field, "service:"))
				}
				body := fmt.Sprintf("%s\n%s：%s → %s\n观测时间：%s UTC%s", details, field, structureStateName(change.From), structureStateName(change.To), change.ObservedAt.UTC().Format("2006-01-02 15:04"), times)
				ref := structureReference("change", fmt.Sprint(item.ID), change.Field, change.ObservedAt.UTC().Format(time.RFC3339Nano), change.From, change.To)
				if err = store.InsertStructureNotice(ctx, tx, watch, item.ID, ref, "change", body); err != nil {
					return err
				}
			}
		}
		serviceBytes, e := json.Marshal(item.Services)
		if e != nil {
			return e
		}
		if err = store.SaveStructureObservation(ctx, tx, watch.Revision, watch.CorporationID, item.ID, store.StructureObservation{Observed: item.ObservedAt, State: item.State, Services: serviceBytes}); err != nil {
			return err
		}
		if item.FuelExpires != nil {
			stage := fuelStage(item.FuelExpires, now)
			if stage != "" {
				ref := structureReference("fuel", fmt.Sprint(item.ID), item.FuelExpires.UTC().Format("2006-01-02"), stage)
				body := fmt.Sprintf("%s\n燃料：%s%s", details, stage, times)
				if err = store.InsertStructureNotice(ctx, tx, watch, item.ID, ref, "fuel", body); err != nil {
					return err
				}
			}
		}
		if item.UnanchorsAt != nil && item.UnanchorsAt.After(now) {
			ref := structureReference("unanchor", fmt.Sprint(item.ID), item.UnanchorsAt.UTC().Format(time.RFC3339))
			body := fmt.Sprintf("%s\n提醒：已安排拆除%s", details, times)
			if err = store.InsertStructureNotice(ctx, tx, watch, item.ID, ref, "unanchor", body); err != nil {
				return err
			}
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	return r.deliver(ctx, conn, watch)
}

func (r *StructureJobs) deliver(ctx context.Context, conn interface {
	Begin(context.Context) (pgx.Tx, error)
}, watch store.StructureWatch) error {
	// A second current source check fails closed on lost administrator/object permission.
	admin, err := r.Administrator(ctx, watch.Owner)
	if err != nil || !admin {
		return err
	}
	rows, err := r.Read(ctx, watch.Owner, watch.CorporationID)
	if err != nil {
		return err
	}
	currentIDs, currentRefs := structureActiveReferences(rows, watch.CorporationID, time.Now().UTC())
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	var active bool
	if err = tx.QueryRow(ctx, `SELECT enabled AND revision=$1 AND app_id=$2 AND group_openid=$3 AND corporation_id=$4 AND owner_id::text=$5 FROM community_qq_structure_watch WHERE id FOR SHARE`, watch.Revision, watch.AppID, watch.Group, watch.CorporationID, watch.Owner).Scan(&active); err != nil || !active {
		return err
	}
	items, err := store.PendingStructureNotices(ctx, tx, watch.Revision)
	if err != nil || len(items) == 0 {
		return err
	}
	ids := make([]int64, 0, len(items))
	obsolete := make([]int64, 0)
	selected := make([]store.StructureNotice, 0, len(items))
	lines := []string{"SEAT 建筑提醒"}
	for _, item := range items {
		if !currentIDs[item.StructureID] || (item.Kind != "change" && !currentRefs[item.Reference]) {
			obsolete = append(obsolete, item.ID)
			continue
		}
		ids = append(ids, item.ID)
		selected = append(selected, item)
		lines = append(lines, item.Body)
	}
	if len(obsolete) > 0 {
		if err = store.DiscardStructureNotices(ctx, tx, obsolete); err != nil {
			return err
		}
	}
	if len(ids) == 0 {
		return tx.Commit(ctx)
	}
	if err = store.SetStructureNoticeState(ctx, tx, ids, "sending", 0); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	// Once claimed, a crash or ambiguous HTTP response leaves no automatic retry.
	tx, err = conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if err = tx.QueryRow(ctx, `SELECT enabled AND revision=$1 AND app_id=$2 AND group_openid=$3 AND corporation_id=$4 AND owner_id::text=$5 FROM community_qq_structure_watch WHERE id FOR UPDATE`, watch.Revision, watch.AppID, watch.Group, watch.CorporationID, watch.Owner).Scan(&active); err != nil {
		return err
	}
	bot := r.Service.qqOfficial.callbackSnapshot()
	if !active || bot == nil {
		if err = store.SetStructureNoticeState(ctx, tx, ids, "rejected", 0); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	appID, _, _ := bot.configSnapshot()
	if appID != watch.AppID || r.Service.verifyQQGatewayConfig(ctx, store.New(tx), bot) != nil {
		if err = store.SetStructureNoticeState(ctx, tx, ids, "rejected", 0); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	// The claim is durable, but no network request has started. If the owner or
	// source becomes invalid while claiming, safely release the unsent batch.
	guardErr := error(nil)
	if r.Service.LockAccounts == nil {
		guardErr = errors.New("structure notification account guard missing")
	} else {
		guardErr = r.Service.LockAccounts(ctx, tx, []string{watch.Owner})
	}
	if guardErr == nil {
		var allowed bool
		allowed, guardErr = r.Administrator(ctx, watch.Owner)
		if guardErr == nil && !allowed {
			guardErr = errors.New("structure notification administrator unavailable")
		}
	}
	if guardErr == nil {
		var latest []eve.Structure
		latest, guardErr = r.Read(ctx, watch.Owner, watch.CorporationID)
		if guardErr == nil {
			idsNow, refsNow := structureActiveReferences(latest, watch.CorporationID, time.Now().UTC())
			for _, item := range selected {
				if !idsNow[item.StructureID] || (item.Kind != "change" && !refsNow[item.Reference]) {
					guardErr = errors.New("structure notification no longer current")
					break
				}
			}
		}
	}
	if guardErr != nil {
		if err = store.ResetUnsentStructureNotices(ctx, tx, ids); err != nil {
			return err
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
		return guardErr
	}
	result := sendQQGroupNotification(ctx, bot, watch.Group, strings.Join(lines, "\n"))
	if result.State != "sent" {
		slog.Warn("QQ structure notification not confirmed", "state", result.State, "code", result.Code)
	}
	if err = store.SetStructureNoticeState(ctx, tx, ids, result.State, result.Code); err != nil {
		return err
	}
	if result.State == "rejected" && result.Code == 40034105 {
		if _, err = tx.Exec(ctx, `UPDATE community_qq_structure_watch SET enabled=false,revision=revision+1,updated_at=now() WHERE id AND revision=$1`, watch.Revision); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

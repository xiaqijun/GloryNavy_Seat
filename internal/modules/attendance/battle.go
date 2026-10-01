package attendance

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"glorynavy.local/seat/internal/modules/attendance/internal/store"
	"glorynavy.local/seat/internal/modules/eve"
	"strings"
	"time"
)

type ShipEvidence struct {
	ID              int64              `json:"id,string"`
	ShipTypeID      int64              `json:"ship_type_id,string"`
	ShipName        string             `json:"ship_name"`
	SolarSystemName string             `json:"solar_system_name"`
	SolarSystemID   int64              `json:"solar_system_id,string"`
	ObservedAt      time.Time          `json:"observed_at"`
	JoinedAt        *time.Time         `json:"joined_at"`
	State           string             `json:"state"`
	Fitting         *eve.BattleFitting `json:"fitting"`
}
type LossEvidence struct {
	ID              int64            `json:"id,string"`
	ShipTypeID      int64            `json:"ship_type_id,string"`
	ShipName        string           `json:"ship_name"`
	SolarSystemName string           `json:"solar_system_name"`
	SolarSystemID   int64            `json:"solar_system_id,string"`
	OccurredAt      time.Time        `json:"occurred_at"`
	State           string           `json:"state"`
	Version         int64            `json:"version,string"`
	Items           []eve.BattleItem `json:"items"`
}
type BattleStatus struct {
	Kind      string     `json:"kind"`
	State     string     `json:"state"`
	Reason    string     `json:"reason"`
	CheckedAt *time.Time `json:"checked_at"`
	NextDueAt time.Time  `json:"next_due_at"`
}
type BattleDetail struct {
	Ships     []ShipEvidence `json:"ships"`
	Losses    []LossEvidence `json:"losses"`
	Tasks     []BattleStatus `json:"tasks"`
	Truncated bool           `json:"truncated"`
}

func (s *Service) BattleDetail(ctx context.Context, user string, eventID, char int64) (BattleDetail, error) {
	out := BattleDetail{Ships: []ShipEvidence{}, Losses: []LossEvidence{}, Tasks: []BattleStatus{}}
	detail, err := s.Detail(ctx, user, eventID)
	if err != nil {
		return out, err
	}
	visible := false
	for _, e := range detail.Entries {
		if e.ID == char {
			visible = true
		}
	}
	if !visible {
		return out, pgx.ErrNoRows
	}
	q := store.New(s.Pool)
	ships, err := q.ListShips(ctx, store.ListShipsParams{EventID: eventID, CharacterID: char})
	if err != nil {
		return out, err
	}
	if len(ships) > 20 {
		ships = ships[:20]
		out.Truncated = true
	}
	ids := []int64{}
	for _, r := range ships {
		v := ShipEvidence{ID: r.ID, ShipTypeID: r.ShipTypeID, SolarSystemID: r.SolarSystemID, ObservedAt: r.ObservedAt.Time, State: r.FittingState}
		if r.JoinedAt.Valid {
			v.JoinedAt = &r.JoinedAt.Time
		}
		if r.FittingState == "ready" {
			var fit eve.BattleFitting
			if err = json.Unmarshal(r.Fitting, &fit); err != nil {
				return out, err
			}
			v.Fitting = &fit
			for _, i := range fit.Items {
				ids = append(ids, i.TypeID)
			}
		}
		ids = append(ids, r.ShipTypeID)
		out.Ships = append(out.Ships, v)
	}
	losses, err := q.ListLosses(ctx, store.ListLossesParams{EventID: eventID, CharacterID: char})
	if err != nil {
		return out, err
	}
	if len(losses) > 100 {
		losses = losses[:100]
		out.Truncated = true
	}
	for _, r := range losses {
		v := LossEvidence{ID: r.KillmailID, ShipTypeID: r.ShipTypeID, SolarSystemID: r.SolarSystemID, OccurredAt: r.OccurredAt.Time, State: r.State, Version: r.Version, Items: []eve.BattleItem{}}
		if err = json.Unmarshal(r.Items, &v.Items); err != nil {
			return out, err
		}
		ids = append(ids, r.ShipTypeID)
		for _, i := range v.Items {
			ids = append(ids, i.TypeID)
		}
		out.Losses = append(out.Losses, v)
	}
	systemIDs := []int64{}
	for _, v := range out.Ships {
		systemIDs = append(systemIDs, v.SolarSystemID)
	}
	for _, v := range out.Losses {
		systemIDs = append(systemIDs, v.SolarSystemID)
	}
	systemNames := map[int64]eve.StaticTypeName{}
	names := map[int64]eve.StaticTypeName{}
	if s.Names != nil {
		systemNames, err = s.Names.SolarSystemNames(ctx, systemIDs)
		if err != nil {
			return out, err
		}
		names, err = s.Names.TypeNames(ctx, ids)
		if err != nil {
			return out, err
		}
	}
	name := func(id int64) string {
		if n := names[id].Name; n != "" {
			return n
		}
		return fmt.Sprintf("#%d", id)
	}
	for i := range out.Ships {
		v := &out.Ships[i]
		v.ShipName = name(v.ShipTypeID)
		v.SolarSystemName = systemNames[v.SolarSystemID].Name
		if v.Fitting != nil {
			for j := range v.Fitting.Items {
				v.Fitting.Items[j].Name = name(v.Fitting.Items[j].TypeID)
			}
		}
	}
	for i := range out.Losses {
		v := &out.Losses[i]
		v.ShipName = name(v.ShipTypeID)
		v.SolarSystemName = systemNames[v.SolarSystemID].Name
		for j := range v.Items {
			v.Items[j].Name = name(v.Items[j].TypeID)
		}
	}
	tasks, err := q.ListBattleTasks(ctx, store.ListBattleTasksParams{EventID: eventID, CharacterID: char})
	if err != nil {
		return out, err
	}
	for _, t := range tasks {
		v := BattleStatus{Kind: t.Kind, State: t.State, Reason: t.Reason, NextDueAt: t.NextDueAt.Time}
		if t.CheckedAt.Valid {
			v.CheckedAt = &t.CheckedAt.Time
		}
		out.Tasks = append(out.Tasks, v)
	}
	return out, nil
}

type LossReview struct {
	Version    int64  `json:"version,string"`
	RequestKey string `json:"request_key"`
	State      string `json:"state"`
	Reason     string `json:"reason"`
}

func (s *Service) ReviewLoss(ctx context.Context, user string, id, lossID int64, c LossReview) error {
	c.Reason = strings.TrimSpace(c.Reason)
	if c.Version < 1 || (c.State != "confirmed" && c.State != "rejected") || len([]rune(c.Reason)) < 1 || len([]rune(c.Reason)) > 200 {
		return ErrInvalid
	}
	actor, err := uuid(user)
	if err != nil {
		return err
	}
	key, err := uuid(c.RequestKey)
	if err != nil {
		return err
	}
	q := store.New(s.Pool)
	ev, err := q.GetEvent(ctx, id)
	if err != nil {
		return err
	}
	if err = s.require(ctx, user, ev.CorporationID); err != nil {
		return err
	}
	body, _ := json.Marshal(struct {
		ID     int64
		Review LossReview
	}{lossID, c})
	digest := sha256.Sum256(body)
	fingerprint := hex.EncodeToString(digest[:])
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	q = store.New(tx)
	if _, err = q.LockEvent(ctx, id); err != nil {
		return err
	}
	if a, e := q.FindAudit(ctx, store.FindAuditParams{EventID: id, RequestKey: key}); e == nil {
		var p auditPayload
		if json.Unmarshal(a.Payload, &p) != nil {
			return ErrUnavailable
		}
		if a.ActorID == actor && a.Action == "loss_review" && p.Fingerprint == fingerprint {
			return nil
		}
		return ErrConflict
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return e
	}
	loss, err := q.LockLoss(ctx, store.LockLossParams{EventID: id, KillmailID: lossID})
	if err != nil {
		return err
	}
	if loss.Version != c.Version {
		return ErrConflict
	}
	if err = q.ReviewLoss(ctx, store.ReviewLossParams{EventID: id, KillmailID: lossID, State: c.State}); err != nil {
		var p *pgconn.PgError
		if errors.As(err, &p) && p.Code == "23505" {
			return ErrConflict
		}
		return err
	}
	p := auditPayload{Fingerprint: fingerprint, Reason: c.Reason, CharacterID: loss.CharacterID, LossID: lossID, LossState: c.State}
	data, _ := json.Marshal(p)
	if err = q.Audit(ctx, store.AuditParams{EventID: id, ActorID: actor, Action: "loss_review", RequestKey: key, Payload: data}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Refresh retries missing loss evidence without editing the original ship snapshot.
func (s *Service) RefreshBattle(ctx context.Context, user string, eventID, char int64, c Change) error {
	actor, err := uuid(user)
	if err != nil {
		return err
	}
	key, err := uuid(c.RequestKey)
	if err != nil {
		return err
	}
	q := store.New(s.Pool)
	ev, err := q.GetEvent(ctx, eventID)
	if err != nil {
		return err
	}
	if err = s.require(ctx, user, ev.CorporationID); err != nil {
		return err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	bound, err := s.Bindings(ctx, tx, []int64{char})
	if err != nil {
		return err
	}
	if len(bound) != 1 {
		return ErrInvalid
	}
	b := bound[0]
	q = store.New(tx)
	ev, err = q.LockEvent(ctx, eventID)
	if err != nil {
		return err
	}
	fingerprint := fmt.Sprintf("battle_refresh:%d:%d", char, c.Version)
	if a, e := q.FindAudit(ctx, store.FindAuditParams{EventID: eventID, RequestKey: key}); e == nil {
		var p auditPayload
		if json.Unmarshal(a.Payload, &p) == nil && a.ActorID == actor && a.Action == "battle_refresh" && p.Fingerprint == fingerprint {
			return nil
		}
		return ErrConflict
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return e
	}
	if ev.Version != c.Version {
		return ErrConflict
	}
	entry, err := q.BattleEntry(ctx, store.BattleEntryParams{EventID: eventID, CharacterID: char})
	if err != nil {
		return err
	}
	if !entry.Present || entry.AccountID.String() != b.UserID {
		return ErrInvalid
	}
	account, _ := uuid(b.UserID)
	// The original task owner hash is immutable; retry never moves evidence to a new owner.
	tasks, err := q.ListBattleTasks(ctx, store.ListBattleTasksParams{EventID: eventID, CharacterID: char})
	if err != nil {
		return err
	}
	for _, t := range tasks {
		if t.Kind != "losses" {
			continue
		}
		locked, e := q.LockBattleTask(ctx, t.ID)
		if e != nil {
			return e
		}
		if subtle.ConstantTimeCompare(locked.OwnerHash, b.OwnerHash) != 1 {
			return ErrInvalid
		}
	}
	if err = q.RetryBattleTasks(ctx, store.RetryBattleTasksParams{EventID: eventID, CharacterID: char, AccountID: account, OwnerHash: b.OwnerHash}); err != nil {
		return err
	}
	data, _ := json.Marshal(auditPayload{Fingerprint: fingerprint, CharacterID: char})
	if err = q.Audit(ctx, store.AuditParams{EventID: eventID, ActorID: actor, Action: "battle_refresh", RequestKey: key, Payload: data}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

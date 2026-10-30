package attendance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"glorynavy.local/seat/internal/modules/attendance/internal/store"
)

type FleetDispatchInput struct {
	Revision   int64    `json:"revision"`
	RequestKey string   `json:"request_key"`
	Channels   []string `json:"channels"`
}

var ErrFleetChannelUnavailable = errors.New("fleet notification channel unavailable")

type FleetDispatchCount struct {
	Channel string `json:"channel"`
	State   string `json:"state"`
	Reason  string `json:"reason,omitempty"`
	Count   int64  `json:"count"`
}
type FleetDispatchStatus struct {
	ID     int64                `json:"id,string"`
	Counts []FleetDispatchCount `json:"counts"`
}

func (s *Service) FleetDispatch(ctx context.Context, user string, eventID int64, in FleetDispatchInput) (FleetDispatchStatus, error) {
	var out FleetDispatchStatus
	actor, err := uuid(user)
	if err != nil {
		return out, ErrInvalid
	}
	key, err := uuid(in.RequestKey)
	if err != nil || in.Revision < 1 || len(in.Channels) < 1 || len(in.Channels) > 3 {
		return out, ErrInvalid
	}
	in.Channels = slices.Clone(in.Channels)
	slices.Sort(in.Channels)
	if len(slices.Compact(slices.Clone(in.Channels))) != len(in.Channels) {
		return out, ErrInvalid
	}
	for _, ch := range in.Channels {
		if ch != "qq" && ch != "eve_mail" && ch != "email" {
			return out, ErrInvalid
		}
	}
	q := store.New(s.Pool)
	ev, err := q.GetEvent(ctx, eventID)
	if err != nil {
		return out, err
	}
	if err = s.require(ctx, user, ev.CorporationID); err != nil {
		return out, err
	}
	raw, _ := json.Marshal(in)
	digest := sha256.Sum256(raw)
	fingerprint := hex.EncodeToString(digest[:])
	replay := func(db store.DBTX, a store.AttendanceAudit) (FleetDispatchStatus, error) {
		var p auditPayload
		if a.Action != "fleet_dispatch" || a.ActorID != actor || json.Unmarshal(a.Payload, &p) != nil || p.Fingerprint != fingerprint {
			return out, ErrConflict
		}
		id, e := store.FleetDispatchByKey(ctx, db, eventID, key)
		if e != nil {
			return out, e
		}
		return s.fleetDispatchStatus(ctx, db, id)
	}
	if old, e := q.FindAudit(ctx, store.FindAuditParams{EventID: eventID, RequestKey: key}); e == nil {
		return replay(s.Pool, old)
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return out, e
	}
	if s.FleetTarget == nil || s.FleetSend == nil {
		return out, ErrFleetChannelUnavailable
	}
	targets := map[string]string{}
	for _, ch := range in.Channels {
		target, e := s.FleetTarget(ctx, ch, ev.CorporationID)
		if e != nil || target == "" {
			return out, ErrFleetChannelUnavailable
		}
		targets[ch] = target
	}
	var accounts []string
	if slices.Contains(in.Channels, "email") {
		if s.ReportBindings == nil {
			return out, ErrUnavailable
		}
		bindings, e := s.ReportBindings(ctx, user, "", ev.CorporationID)
		if e != nil {
			return out, e
		}
		for _, b := range bindings {
			if b.UserID != "" && !slices.Contains(accounts, b.UserID) {
				accounts = append(accounts, b.UserID)
			}
		}
		slices.Sort(accounts)
	}
	if err = s.require(ctx, user, ev.CorporationID); err != nil {
		return out, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(context.Background())
	q = store.New(tx)
	ev, err = q.LockEvent(ctx, eventID)
	if err != nil {
		return out, err
	}
	if old, e := q.FindAudit(ctx, store.FindAuditParams{EventID: eventID, RequestKey: key}); e == nil {
		return replay(tx, old)
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return out, e
	}
	if ev.State != "open" {
		return out, ErrConflict
	}
	n, err := store.FleetNoticeByEvent(ctx, tx, eventID)
	if err != nil {
		return out, err
	}
	if n.Revision != in.Revision {
		return out, ErrConflict
	}
	outstanding, err := store.HasOutstandingFleetDeliveries(ctx, tx, eventID, in.Revision, in.Channels)
	if err != nil {
		return out, err
	}
	if outstanding {
		return out, ErrConflict
	}
	subject := strings.Join(strings.Fields(ev.Title), " ")
	if subject == "" {
		return out, ErrInvalid
	}
	body := fleetNoticeText(event(ev), fleetNoticeDTO(n))
	id, err := store.InsertFleetDispatch(ctx, tx, eventID, ev.CorporationID, n.Revision, actor, key, subject, body)
	if err != nil {
		return out, err
	}
	for _, ch := range in.Channels {
		if ch == "email" && len(accounts) > 0 {
			for _, account := range accounts {
				a, e := uuid(account)
				if e != nil {
					return out, ErrInvalid
				}
				if e = store.InsertFleetDelivery(ctx, tx, id, ch, targets[ch], a); e != nil {
					return out, e
				}
			}
		} else if err = store.InsertFleetDelivery(ctx, tx, id, ch, targets[ch], pgtype.UUID{}); err != nil {
			return out, err
		}
	}
	payload, _ := json.Marshal(auditPayload{Fingerprint: fingerprint})
	if err = q.Audit(ctx, store.AuditParams{EventID: eventID, ActorID: actor, Action: "fleet_dispatch", RequestKey: key, Payload: payload}); err != nil {
		return out, err
	}
	out, err = s.fleetDispatchStatus(ctx, tx, id)
	if err != nil {
		return out, err
	}
	if err = tx.Commit(ctx); err != nil {
		return FleetDispatchStatus{}, err
	}
	return out, nil
}

func (s *Service) fleetDispatchStatus(ctx context.Context, db store.DBTX, id int64) (FleetDispatchStatus, error) {
	rows, err := store.FleetDeliveryCounts(ctx, db, id)
	if err != nil {
		return FleetDispatchStatus{}, err
	}
	out := FleetDispatchStatus{ID: id, Counts: []FleetDispatchCount{}}
	for _, v := range rows {
		out.Counts = append(out.Counts, FleetDispatchCount{Channel: v.Channel, State: v.State, Reason: v.Reason, Count: v.Count})
	}
	return out, nil
}

func (s *Service) LatestFleetDispatch(ctx context.Context, user string, eventID int64) (*FleetDispatchStatus, error) {
	ev, err := store.New(s.Pool).GetEvent(ctx, eventID)
	if err != nil {
		return nil, err
	}
	if err = s.require(ctx, user, ev.CorporationID); err != nil {
		return nil, err
	}
	id, err := store.LatestFleetDispatch(ctx, s.Pool, eventID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out, err := s.fleetDispatchStatus(ctx, s.Pool, id)
	return &out, err
}

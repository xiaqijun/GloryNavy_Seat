package attendance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/attendance/internal/store"
)

type FleetNotice struct {
	SolarSystem  string    `json:"solar_system"`
	Doctrine     string    `json:"doctrine"`
	MeetingPoint string    `json:"meeting_point"`
	Commander    string    `json:"commander"`
	Details      string    `json:"details"`
	Revision     int64     `json:"revision"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type FleetNoticeInput struct {
	SolarSystem  string `json:"solar_system"`
	Doctrine     string `json:"doctrine"`
	MeetingPoint string `json:"meeting_point"`
	Commander    string `json:"commander"`
	Details      string `json:"details"`
	Revision     int64  `json:"revision"`
	RequestKey   string `json:"request_key"`
}

func fleetNoticeDTO(n store.FleetNotice) FleetNotice {
	return FleetNotice{SolarSystem: n.SolarSystem, Doctrine: n.Doctrine, MeetingPoint: n.MeetingPoint, Commander: n.Commander, Details: n.Details, Revision: n.Revision, UpdatedAt: n.UpdatedAt}
}

func (s *Service) FleetNotice(ctx context.Context, user string, eventID int64) (*FleetNotice, error) {
	ev, err := store.New(s.Pool).GetEvent(ctx, eventID)
	if err != nil {
		return nil, err
	}
	if err = s.require(ctx, user, ev.CorporationID); err != nil {
		return nil, err
	}
	n, err := store.FleetNoticeByEvent(ctx, s.Pool, eventID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := fleetNoticeDTO(n)
	return &out, nil
}

func (s *Service) SaveFleetNotice(ctx context.Context, user string, eventID int64, input FleetNoticeInput) (FleetNotice, error) {
	var out FleetNotice
	input.SolarSystem = strings.TrimSpace(input.SolarSystem)
	input.Doctrine = strings.TrimSpace(input.Doctrine)
	input.MeetingPoint = strings.TrimSpace(input.MeetingPoint)
	input.Commander = strings.TrimSpace(input.Commander)
	input.Details = strings.TrimSpace(input.Details)
	actor, err := uuid(user)
	if err != nil {
		return out, ErrInvalid
	}
	key, err := uuid(input.RequestKey)
	if err != nil || input.Revision < 0 || input.SolarSystem == "" || input.Doctrine == "" || utf8.RuneCountInString(input.SolarSystem) > 120 || utf8.RuneCountInString(input.Doctrine) > 500 || utf8.RuneCountInString(input.MeetingPoint) > 300 || utf8.RuneCountInString(input.Commander) > 120 || utf8.RuneCountInString(input.Details) > 3000 {
		return out, ErrInvalid
	}
	ev, err := store.New(s.Pool).GetEvent(ctx, eventID)
	if err != nil {
		return out, err
	}
	if err = s.require(ctx, user, ev.CorporationID); err != nil {
		return out, err
	}
	raw, _ := json.Marshal(input)
	digest := sha256.Sum256(raw)
	fingerprint := hex.EncodeToString(digest[:])
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(context.Background())
	q := store.New(tx)
	if _, err = q.LockEvent(ctx, eventID); err != nil {
		return out, err
	}
	if old, e := q.FindAudit(ctx, store.FindAuditParams{EventID: eventID, RequestKey: key}); e == nil {
		var p struct {
			Fingerprint string `json:"fingerprint"`
			Revision    int64  `json:"revision"`
		}
		if old.Action != "fleet_notice" || old.ActorID != actor || json.Unmarshal(old.Payload, &p) != nil || p.Fingerprint != fingerprint {
			return out, ErrConflict
		}
		n, e := store.FleetNoticeByEvent(ctx, tx, eventID)
		if e != nil || n.Revision != p.Revision {
			return out, ErrConflict
		}
		return fleetNoticeDTO(n), nil
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return out, e
	}
	n, err := store.SaveFleetNotice(ctx, tx, store.FleetNotice{EventID: eventID, SolarSystem: input.SolarSystem, Doctrine: input.Doctrine, MeetingPoint: input.MeetingPoint, Commander: input.Commander, Details: input.Details, Revision: input.Revision, UpdatedBy: actor})
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrConflict
	}
	if err != nil {
		return out, err
	}
	payload, _ := json.Marshal(map[string]any{"fingerprint": fingerprint, "revision": n.Revision})
	if err = q.Audit(ctx, store.AuditParams{EventID: eventID, ActorID: actor, Action: "fleet_notice", RequestKey: key, Payload: payload}); err != nil {
		return out, err
	}
	if err = tx.Commit(ctx); err != nil {
		return out, err
	}
	return fleetNoticeDTO(n), nil
}

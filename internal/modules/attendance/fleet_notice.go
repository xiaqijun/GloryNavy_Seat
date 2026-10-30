package attendance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/attendance/internal/store"
)

type FleetNotice struct {
	SolarSystem  string           `json:"solar_system"`
	Doctrine     string           `json:"doctrine"`
	MeetingPoint string           `json:"meeting_point"`
	Commander    string           `json:"commander"`
	Details      string           `json:"details"`
	Revision     int64            `json:"revision"`
	UpdatedAt    time.Time        `json:"updated_at"`
	Links        FleetNoticeLinks `json:"links"`
}

type FleetNoticeLinks struct {
	CommanderID       string `json:"commander_id"`
	FireAnchor        string `json:"fire_anchor"`
	FireAnchorID      string `json:"fire_anchor_id"`
	LogisticsAnchor   string `json:"logistics_anchor"`
	LogisticsAnchorID string `json:"logistics_anchor_id"`
	FittingDNA        string `json:"fitting_dna"`
}

var fleetDNA = regexp.MustCompile(`^[1-9][0-9]*(?::[1-9][0-9]*_?;[1-9][0-9]*)*::$`)

func validNoticeID(id string) bool {
	if id == "" {
		return true
	}
	n, err := strconv.ParseInt(id, 10, 64)
	return err == nil && n > 0 && strconv.FormatInt(n, 10) == id
}

func validFleetNoticeLinks(n FleetNoticeLinks, commander string) bool {
	return validNoticeID(n.CommanderID) && validNoticeID(n.FireAnchorID) && validNoticeID(n.LogisticsAnchorID) &&
		(n.CommanderID == "" || commander != "") && (n.FireAnchorID == "" || n.FireAnchor != "") &&
		(n.LogisticsAnchorID == "" || n.LogisticsAnchor != "") && utf8.RuneCountInString(n.FireAnchor) <= 120 &&
		utf8.RuneCountInString(n.LogisticsAnchor) <= 120 && len(n.FittingDNA) <= 6000 &&
		(n.FittingDNA == "" || fleetDNA.MatchString(n.FittingDNA))
}

func selectedNoticeCharacter(id, name string, candidates []Binding, previousID, previousName string) bool {
	if id == "" || (id == previousID && name == previousName) {
		return true
	}
	for _, c := range candidates {
		if strconv.FormatInt(c.ID, 10) == id && c.Name == name {
			return true
		}
	}
	return false
}

type FleetNoticeInput struct {
	SolarSystem  string            `json:"solar_system"`
	Doctrine     string            `json:"doctrine"`
	MeetingPoint string            `json:"meeting_point"`
	Commander    string            `json:"commander"`
	Details      string            `json:"details"`
	Revision     int64             `json:"revision"`
	RequestKey   string            `json:"request_key"`
	Links        *FleetNoticeLinks `json:"links,omitempty"`
}

func fleetNoticeDTO(n store.FleetNotice) FleetNotice {
	var links FleetNoticeLinks
	_ = json.Unmarshal(n.Links, &links)
	return FleetNotice{SolarSystem: n.SolarSystem, Doctrine: n.Doctrine, MeetingPoint: n.MeetingPoint, Commander: n.Commander, Details: n.Details, Revision: n.Revision, UpdatedAt: n.UpdatedAt, Links: links}
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
	if input.Links != nil {
		input.Links.FireAnchor = strings.TrimSpace(input.Links.FireAnchor)
		input.Links.LogisticsAnchor = strings.TrimSpace(input.Links.LogisticsAnchor)
		if !validFleetNoticeLinks(*input.Links, input.Commander) {
			return out, ErrInvalid
		}
	}
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
	if input.Links != nil && s.ReportBindings != nil {
		candidates, e := s.ReportBindings(ctx, user, "", ev.CorporationID)
		if e != nil {
			return out, e
		}
		var previous FleetNotice
		old, e := store.FleetNoticeByEvent(ctx, s.Pool, eventID)
		if e == nil {
			previous = fleetNoticeDTO(old)
		} else if !errors.Is(e, pgx.ErrNoRows) {
			return out, e
		}
		if !selectedNoticeCharacter(input.Links.CommanderID, input.Commander, candidates, previous.Links.CommanderID, previous.Commander) ||
			!selectedNoticeCharacter(input.Links.FireAnchorID, input.Links.FireAnchor, candidates, previous.Links.FireAnchorID, previous.Links.FireAnchor) ||
			!selectedNoticeCharacter(input.Links.LogisticsAnchorID, input.Links.LogisticsAnchor, candidates, previous.Links.LogisticsAnchorID, previous.Links.LogisticsAnchor) {
			return out, ErrInvalid
		}
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
	var links json.RawMessage
	if input.Links != nil {
		links, _ = json.Marshal(input.Links)
	} else if input.Revision > 0 {
		// Old clients cannot edit the new fields. Clear only links whose visible
		// text they changed, so a link can never silently point at the old object.
		previous, e := store.FleetNoticeByEvent(ctx, tx, eventID)
		if e != nil {
			return out, e
		}
		old := fleetNoticeDTO(previous).Links
		if previous.Commander != input.Commander {
			old.CommanderID = ""
		}
		if previous.Doctrine != input.Doctrine {
			old.FittingDNA = ""
		}
		links, _ = json.Marshal(old)
	}
	n, err := store.SaveFleetNotice(ctx, tx, store.FleetNotice{EventID: eventID, SolarSystem: input.SolarSystem, Doctrine: input.Doctrine, MeetingPoint: input.MeetingPoint, Commander: input.Commander, Details: input.Details, Revision: input.Revision, UpdatedBy: actor, Links: links})
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

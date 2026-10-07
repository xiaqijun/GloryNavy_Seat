// Package attendance owns event attendance and account-level activity reports.
package attendance

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/modules/attendance/internal/store"
	"glorynavy.local/seat/internal/modules/eve"
)

var ErrInvalid = errors.New("invalid attendance input")
var ErrConflict = errors.New("attendance changed; refresh and retry")
var ErrUnavailable = errors.New("fleet unavailable")

type Binding struct {
	ID           int64
	UserID, Name string
	OwnerHash    []byte
}
type Corporation struct {
	ID   int64  `json:"id,string"`
	Name string `json:"name"`
}
type Gateway interface {
	Fleet(context.Context, int64, []byte) (eve.FleetSnapshot, error)
	GuardFleet(context.Context, pgx.Tx, eve.FleetSnapshot) error
	ActivityProfiles(context.Context, []int64) ([]eve.FleetMember, error)
	OnlineDataTx(context.Context, pgx.Tx, []int64, time.Time, time.Time, ...int64) (eve.OnlineData, error)
}
type Service struct {
	Administrator          func(context.Context, string) (bool, error)
	AlliancePAP            AlliancePAPConfig
	CoinConversion         func(context.Context, pgx.Tx, string, string, string, string, []PAPCoinAward) (PAPCoinQuote, error)
	AllianceCoinConversion func(context.Context, pgx.Tx, string, string, string, string, []PAPCoinAward) (PAPCoinQuote, error)
	AllianceCoinAwards     func(context.Context, pgx.Tx, string, string, []PAPCoinAward) error
	AllianceLockAccounts   func(context.Context, pgx.Tx, []string) error
	CoinAwards             func(context.Context, pgx.Tx, string, string, []PAPCoinAward) error
	Pool                   *pgxpool.Pool
	EVE                    Gateway
	Battle                 BattleGateway
	Names                  interface {
		TypeNames(context.Context, []int64) (map[int64]eve.StaticTypeName, error)
		SolarSystemNames(context.Context, []int64) (map[int64]eve.StaticTypeName, error)
	}
	// Every callback is injected by the host; modules never import each other's stores.
	Manage         func(context.Context, string, int64) (bool, error)
	Corporations   func(context.Context, string) ([]Corporation, error)
	Bindings       func(context.Context, pgx.Tx, []int64) ([]Binding, error)
	Own            func(context.Context, string) ([]Binding, error)
	ReportBindings func(context.Context, string, string, int64) ([]Binding, error)
	// AlliancePAPMemberNames is a host-provided display projection for already
	// authorized account IDs. It does not grant access to member data.
	AlliancePAPMemberNames func(context.Context, []string) (map[string]string, error)
}

type Event struct {
	CanConvert    bool       `json:"can_convert"`
	PAPPoints     int32      `json:"pap_points"`
	PAPIssued     bool       `json:"pap_issued"`
	ID            int64      `json:"id,string"`
	CorporationID int64      `json:"corporation_id,string"`
	Title         string     `json:"title"`
	StartsAt      time.Time  `json:"starts_at"`
	State         string     `json:"state"`
	Version       int64      `json:"version,string"`
	Participants  int32      `json:"participants"`
	CanManage     bool       `json:"can_manage"`
	EndsAt        *time.Time `json:"ends_at"`
}
type Entry struct {
	PAPPoints          int32      `json:"pap_points"`
	SolarSystemName    string     `json:"solar_system_name"`
	SolarSystemID      *int64     `json:"solar_system_id,string"`
	LocationObservedAt *time.Time `json:"location_observed_at"`
	ShipTypeID         int64      `json:"ship_type_id,string"`
	ShipName           string     `json:"ship_name"`
	Losses             int32      `json:"losses"`
	ID                 int64      `json:"character_id,string"`
	Name               string     `json:"name"`
	AccountID          *string    `json:"account_id"`
	Source             string     `json:"source"`
	Present            bool       `json:"present"`
	RecordedAt         time.Time  `json:"recorded_at"`
}
type Detail struct {
	Event   Event   `json:"event"`
	Entries []Entry `json:"entries"`
}

func event(r store.AttendanceEvent) Event {
	v := Event{PAPPoints: r.PapPoints, PAPIssued: r.PapIssued, ID: r.ID, CorporationID: r.CorporationID, Title: r.Title, StartsAt: r.StartsAt.Time, State: r.State, Version: r.Version}
	if r.EndsAt.Valid {
		v.EndsAt = &r.EndsAt.Time
	}
	return v
}
func stamp(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }
func uuid(s string) (pgtype.UUID, error) {
	var v pgtype.UUID
	if v.Scan(s) != nil || !v.Valid || v.String() == "00000000-0000-0000-0000-000000000000" {
		return v, ErrInvalid
	}
	return v, nil
}
func (s *Service) require(ctx context.Context, user string, corp int64) error {
	if corp <= 0 {
		return pgx.ErrNoRows
	}
	ok, err := s.Manage(ctx, user, corp)
	if err != nil {
		return err
	}
	if !ok {
		return pgx.ErrNoRows
	}
	return nil
}

type Create struct {
	CorporationID int64     `json:"corporation_id,string"`
	Title         string    `json:"title"`
	StartsAt      time.Time `json:"starts_at"`
	RequestKey    string    `json:"request_key"`
}

func (s *Service) Create(ctx context.Context, user string, c Create) (Event, error) {
	c.Title = strings.TrimSpace(c.Title)
	key, err := uuid(c.RequestKey)
	actor, e := uuid(user)
	if err != nil || e != nil || len([]rune(c.Title)) < 1 || len([]rune(c.Title)) > 80 || c.StartsAt.IsZero() || c.StartsAt.Before(time.Now().AddDate(-1, 0, 0)) || c.StartsAt.After(time.Now().AddDate(1, 0, 0)) {
		return Event{}, ErrInvalid
	}
	if err = s.require(ctx, user, c.CorporationID); err != nil {
		return Event{}, err
	}
	corps, err := s.Corporations(ctx, user)
	if err != nil {
		return Event{}, err
	}
	known := false
	for _, corp := range corps {
		if corp.ID == c.CorporationID {
			known = true
		}
	}
	if !known {
		return Event{}, pgx.ErrNoRows
	}
	r, err := store.New(s.Pool).CreateEvent(ctx, store.CreateEventParams{CorporationID: c.CorporationID, Title: c.Title, StartsAt: stamp(c.StartsAt), CreatedBy: actor, RequestKey: key})
	if err != nil {
		return Event{}, err
	}
	if r.CorporationID != c.CorporationID || r.Title != c.Title || !r.StartsAt.Time.Equal(c.StartsAt.Truncate(time.Microsecond)) {
		return Event{}, ErrConflict
	}
	dto := event(r)
	dto.CanManage = true
	return dto, nil
}

type EventList struct {
	Events     []Event `json:"events"`
	NextCursor string  `json:"next_cursor"`
}

func (s *Service) List(ctx context.Context, user string, before int64) (EventList, error) {
	result := EventList{Events: []Event{}}
	corps, err := s.Corporations(ctx, user)
	if err != nil {
		return result, err
	}
	ids := []int64{}
	for _, c := range corps {
		ids = append(ids, c.ID)
	}
	actor, err := uuid(user)
	if err != nil {
		return result, err
	}
	if before == 0 {
		before = math.MaxInt64
	}
	rows, err := store.New(s.Pool).ListEvents(ctx, store.ListEventsParams{BeforeID: before, Corporations: ids, AccountID: actor})
	if err != nil {
		return result, err
	}
	if len(rows) > 30 {
		rows = rows[:30]
		result.NextCursor = strconv.FormatInt(rows[29].ID, 10)
	}
	for _, r := range rows {
		v := Event{PAPPoints: r.PapPoints, PAPIssued: r.PapIssued, ID: r.ID, CorporationID: r.CorporationID, Title: r.Title, StartsAt: r.StartsAt.Time, State: r.State, Version: r.Version, Participants: r.Participants, CanManage: slices.Contains(ids, r.CorporationID)}
		if r.EndsAt.Valid {
			v.EndsAt = &r.EndsAt.Time
		}
		result.Events = append(result.Events, v)
	}
	return result, nil
}

// PendingPAP returns closed activities that the current user can manage and
// that still have at least one bound participant. It is intentionally a
// separate read model for the PAP page: the actual write still goes through
// SetPAP once per activity, preserving its version, audit and idempotency key.
func (s *Service) PendingPAP(ctx context.Context, user string) (EventList, error) {
	result := EventList{Events: []Event{}}
	corps, err := s.Corporations(ctx, user)
	if err != nil {
		return result, err
	}
	ids := make([]int64, 0, len(corps))
	for _, c := range corps {
		ids = append(ids, c.ID)
	}
	rows, err := store.New(s.Pool).ListPendingPAP(ctx, ids)
	if err != nil {
		return result, err
	}
	for _, r := range rows {
		v := Event{
			PAPPoints:     r.PapPoints,
			PAPIssued:     r.PapIssued,
			ID:            r.ID,
			CorporationID: r.CorporationID,
			Title:         r.Title,
			StartsAt:      r.StartsAt.Time,
			State:         r.State,
			Version:       r.Version,
			Participants:  r.Participants,
			CanManage:     true,
		}
		if r.EndsAt.Valid {
			v.EndsAt = &r.EndsAt.Time
		}
		result.Events = append(result.Events, v)
	}
	return result, nil
}

func (s *Service) Detail(ctx context.Context, user string, id int64) (Detail, error) {
	result := Detail{Entries: []Entry{}}
	q := store.New(s.Pool)
	r, err := q.GetEvent(ctx, id)
	if err != nil {
		return result, err
	}
	manage, err := s.Manage(ctx, user, r.CorporationID)
	if err != nil {
		return result, err
	}
	rows, err := q.ListEntries(ctx, id)
	if err != nil {
		return result, err
	}
	present := map[string]bool{}
	for _, e := range rows {
		if !manage && (!e.AccountID.Valid || e.AccountID.String() != user) {
			continue
		}
		var account *string
		if e.AccountID.Valid {
			v := e.AccountID.String()
			account = &v
		}
		result.Entries = append(result.Entries, Entry{ID: e.CharacterID, Name: e.CharacterName, AccountID: account, Source: e.Source, Present: e.Present, RecordedAt: e.RecordedAt.Time})
		if e.Present && account != nil {
			present[*account] = true
		}
	}
	if !manage && len(result.Entries) == 0 {
		return result, pgx.ErrNoRows
	}
	result.Event = event(r)
	result.Event.CanManage = manage
	if manage && s.Administrator != nil && s.CoinConversion != nil {
		result.Event.CanConvert, err = s.Administrator(ctx, user)
		if err != nil {
			return result, err
		}
	}
	result.Event.Participants = int32(len(present))
	ships, err := q.EntryShips(ctx, id)
	if err != nil {
		return result, err
	}
	losses, err := q.EntryLossCounts(ctx, id)
	if err != nil {
		return result, err
	}
	systemIDs := []int64{}
	awards, err := q.PAPAwards(ctx, id)
	if err != nil {
		return result, err
	}
	typeIDs := []int64{}
	for _, ship := range ships {
		typeIDs = append(typeIDs, ship.ShipTypeID)
		systemIDs = append(systemIDs, ship.SolarSystemID)
	}
	systemNames := map[int64]eve.StaticTypeName{}
	names := map[int64]eve.StaticTypeName{}
	if s.Names != nil {
		systemNames, err = s.Names.SolarSystemNames(ctx, systemIDs)
		if err != nil {
			return result, err
		}
		names, err = s.Names.TypeNames(ctx, typeIDs)
		if err != nil {
			return result, err
		}
	}
	for i := range result.Entries {
		for _, a := range awards {
			if a.CharacterID == result.Entries[i].ID && result.Entries[i].AccountID != nil && a.AccountID.String() == *result.Entries[i].AccountID {
				result.Entries[i].PAPPoints = a.Points
			}
		}
		for _, ship := range ships {
			if ship.CharacterID == result.Entries[i].ID {
				result.Entries[i].ShipTypeID = ship.ShipTypeID
				result.Entries[i].ShipName = names[ship.ShipTypeID].Name
				if ship.SolarSystemID > 0 {
					id, at := ship.SolarSystemID, ship.ObservedAt.Time
					result.Entries[i].SolarSystemID = &id
					result.Entries[i].SolarSystemName = systemNames[id].Name
					result.Entries[i].LocationObservedAt = &at
				}
			}
		}
		for _, loss := range losses {
			if loss.CharacterID == result.Entries[i].ID {
				result.Entries[i].Losses = loss.Losses
			}
		}
	}
	return result, nil
}

type Change struct {
	Version     int64  `json:"version,string"`
	RequestKey  string `json:"request_key"`
	SourceID    int64  `json:"source_character_id,string,omitempty"`
	CharacterID int64  `json:"character_id,string,omitempty"`
	Present     bool   `json:"present"`
	Reason      string `json:"reason"`
}
type ChangeResult struct {
	ExcludedExternal *int  `json:"excluded_external"`
	ExcludedUnbound  *int  `json:"excluded_unbound"`
	AutoClosed       bool  `json:"auto_closed,omitempty"`
	Event            Event `json:"event"`
	Recorded         int   `json:"recorded"`
	Excluded         int   `json:"excluded"`
}
type auditPayload struct {
	PAPPoints        int32      `json:"pap_points"`
	PAPDelta         int64      `json:"pap_delta"`
	PAPRevoked       bool       `json:"pap_revoked"`
	ExcludedExternal *int       `json:"excluded_external,omitempty"`
	ExcludedUnbound  *int       `json:"excluded_unbound,omitempty"`
	LossID           int64      `json:"loss_id,string,omitempty"`
	LossState        string     `json:"loss_state,omitempty"`
	Fingerprint      string     `json:"fingerprint"`
	Reason           string     `json:"reason,omitempty"`
	SourceID         int64      `json:"source_character_id,string,omitempty"`
	Generation       int64      `json:"generation,string,omitempty"`
	FleetID          int64      `json:"fleet_id,string,omitempty"`
	CharacterID      int64      `json:"character_id,string,omitempty"`
	Present          bool       `json:"present"`
	Recorded         int        `json:"recorded"`
	Excluded         int        `json:"excluded"`
	ObservedAt       *time.Time `json:"observed_at,omitempty"`
	AutoClosed       bool       `json:"auto_closed,omitempty"`
}

// Change publishes under identity -> credential -> event locks, after network work.
func (s *Service) Change(ctx context.Context, user string, id int64, action string, c Change) (ChangeResult, error) {
	var result ChangeResult
	key, err := uuid(c.RequestKey)
	actor, e := uuid(user)
	c.Reason = strings.TrimSpace(c.Reason)
	if err != nil || e != nil || c.Version < 1 || len([]rune(c.Reason)) > 200 {
		return result, ErrInvalid
	}
	if action != "capture" && action != "manual" && action != "close" && action != "reopen" {
		return result, ErrInvalid
	}
	if (action == "manual" || action == "reopen") && c.Reason == "" {
		return result, ErrInvalid
	}
	if action == "capture" && c.SourceID <= 0 || action == "manual" && c.CharacterID <= 0 {
		return result, ErrInvalid
	}
	q := store.New(s.Pool)
	ev, err := q.GetEvent(ctx, id)
	if err != nil {
		return result, err
	}
	if err = s.require(ctx, user, ev.CorporationID); err != nil {
		return result, err
	}
	canonical, _ := json.Marshal(struct {
		Action string
		Change Change
	}{action, c})
	digest := sha256.Sum256(canonical)
	fingerprint := hex.EncodeToString(digest[:])
	replay := func(a store.AttendanceAudit) (ChangeResult, error) {
		var p auditPayload
		if json.Unmarshal(a.Payload, &p) != nil {
			return result, ErrUnavailable
		}
		if a.ActorID != actor || p.Fingerprint != fingerprint {
			return result, ErrConflict
		}
		v := event(ev)
		v.CanManage = true
		return ChangeResult{Event: v, Recorded: p.Recorded, Excluded: p.Excluded, ExcludedExternal: p.ExcludedExternal, ExcludedUnbound: p.ExcludedUnbound, AutoClosed: p.AutoClosed}, nil
	}
	if a, e := q.FindAudit(ctx, store.FindAuditParams{EventID: id, RequestKey: key}); e == nil {
		return replay(a)
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return result, e
	}
	if action == "reopen" && ev.PapIssued {
		return result, ErrConflict
	}
	if ev.Version != c.Version ||
		(ev.State == "closed" && action == "capture") ||
		(ev.State == "open" && (action == "reopen" || action == "manual")) {
		return result, ErrConflict
	}
	var fleet eve.FleetSnapshot
	fleetEnded := false
	members := []eve.FleetMember{}
	lockIDs := []int64{}
	observed := time.Now()
	excluded := 0
	external, unbound := 0, 0
	if action == "capture" {
		if ev.StartsAt.Time.After(time.Now()) {
			return result, ErrInvalid
		}
		own, err := s.Own(ctx, user)
		if err != nil {
			return result, err
		}
		var source *Binding
		for i := range own {
			if own[i].ID == c.SourceID {
				source = &own[i]
				break
			}
		}
		if source == nil {
			return result, pgx.ErrNoRows
		}
		fleet, err = s.EVE.Fleet(ctx, source.ID, source.OwnerHash)
		if errors.Is(err, eve.ErrFleetDisbanded) {
			fleetEnded = true
			// A disbanded fleet has no valid roster snapshot. Do not record stale
			// members that a cached or test source may still carry.
			fleet.Members = nil
			observed = time.Now().UTC()
		} else if err != nil {
			return result, ErrUnavailable
		} else {
			observed = fleet.ObservedAt
		}
		lockIDs = append(lockIDs, source.ID)
		for _, m := range fleet.Members {
			if m.CorporationID == ev.CorporationID {
				members = append(members, m)
			} else {
				excluded++
				external++
			}
		}
	} else if action == "manual" {
		existing, e := q.ListEntries(ctx, id)
		if e != nil {
			return result, e
		}
		for _, r := range existing {
			if r.CharacterID == c.CharacterID {
				members = append(members, eve.FleetMember{ID: r.CharacterID, Name: r.CharacterName, CorporationID: ev.CorporationID})
				break
			}
		}
		if len(members) == 0 {
			members, err = s.EVE.ActivityProfiles(ctx, []int64{c.CharacterID})
			if err != nil {
				return result, ErrUnavailable
			}
		}
		if len(members) != 1 || members[0].CorporationID != ev.CorporationID {
			return result, ErrInvalid
		}
	}
	for _, m := range members {
		lockIDs = append(lockIDs, m.ID)
	}
	if err = s.require(ctx, user, ev.CorporationID); err != nil {
		return result, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(context.Background())
	q = store.New(tx)
	bindings, err := s.Bindings(ctx, tx, lockIDs)
	if err != nil {
		return result, err
	}
	bound := map[int64]Binding{}
	for _, b := range bindings {
		bound[b.ID] = b
	}
	if action == "capture" {
		source, ok := bound[c.SourceID]
		ownerHash := fleet.OwnerHash
		if fleetEnded {
			ownerHash = source.OwnerHash
		}
		if !ok || source.UserID != user || subtle.ConstantTimeCompare(source.OwnerHash, ownerHash) != 1 {
			return result, ErrConflict
		}
		if !fleetEnded {
			if err = s.EVE.GuardFleet(ctx, tx, fleet); err != nil {
				return result, ErrConflict
			}
		}
	}
	ev, err = q.LockEvent(ctx, id)
	if err != nil {
		return result, err
	}

	if a, e := q.FindAudit(ctx, store.FindAuditParams{EventID: id, RequestKey: key}); e == nil {
		return replay(a)
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return result, e
	}
	if action == "reopen" && ev.PapIssued {
		return result, ErrConflict
	}
	if ev.Version != c.Version ||
		(ev.State == "closed" && action == "capture") ||
		(ev.State == "open" && (action == "reopen" || action == "manual")) {
		return result, ErrConflict
	}
	source := "fleet"
	if action == "manual" {
		source = "manual"
	}
	recorded := 0
	for _, m := range members {
		var account pgtype.UUID
		if b, ok := bound[m.ID]; ok {
			account, _ = uuid(b.UserID)
		}
		// New attendance requires a bound participant. Existing history remains readable.
		if !account.Valid {
			if action == "manual" {
				if _, e := q.BattleEntry(ctx, store.BattleEntryParams{EventID: id, CharacterID: m.ID}); e != nil {
					return result, ErrInvalid
				}
			} else {
				excluded++
				unbound++
				continue
			}
		}
		present := true
		if action == "manual" {
			present = c.Present
		}
		if err = q.SaveEntry(ctx, store.SaveEntryParams{EventID: id, CharacterID: m.ID, AccountID: account, CharacterName: m.Name, Source: source, Present: present, RecordedAt: stamp(observed)}); err != nil {
			return result, err
		}
		recorded++
		entry, e := q.BattleEntry(ctx, store.BattleEntryParams{EventID: id, CharacterID: m.ID})
		if e != nil {
			return result, e
		}
		// Rebinding must never collect private evidence into another account's history.
		if entry.AccountID != account || !entry.Present {
			continue
		}
		b := bound[m.ID]
		if s.Battle != nil && len(b.OwnerHash) > 0 {
			if action == "capture" && m.ShipTypeID > 0 {
				var joined pgtype.Timestamptz
				if !m.JoinedAt.IsZero() {
					joined = stamp(m.JoinedAt)
				}
				sid, e := q.SaveShip(ctx, store.SaveShipParams{EventID: id, CharacterID: m.ID, RequestKey: key, ShipTypeID: m.ShipTypeID, SolarSystemID: m.SolarSystemID, JoinedAt: joined, ObservedAt: stamp(observed)})
				if e != nil {
					return result, e
				}
				if e = q.SeedBattleTask(ctx, store.SeedBattleTaskParams{EventID: id, CharacterID: m.ID, AccountID: account, OwnerHash: b.OwnerHash, Kind: "fitting", SnapshotID: sid}); e != nil {
					return result, e
				}
			}
			if e := q.SeedBattleTask(ctx, store.SeedBattleTaskParams{EventID: id, CharacterID: m.ID, AccountID: account, OwnerHash: b.OwnerHash, Kind: "losses"}); e != nil {
				return result, e
			}
		}
	}
	state := ev.State
	if action == "close" || fleetEnded {
		state = "closed"
	}
	if action == "reopen" {
		state = "open"
	}
	updated, err := q.UpdateEvent(ctx, store.UpdateEventParams{ID: id, State: state})
	if err != nil {
		return result, err
	}
	auditAction := action
	if fleetEnded {
		c.Reason = "舰队已解散，活动自动结束"
		auditAction = "auto_close"
	}
	p := auditPayload{Fingerprint: fingerprint, Reason: c.Reason, SourceID: c.SourceID, Generation: fleet.Generation, FleetID: fleet.FleetID, CharacterID: c.CharacterID, Present: c.Present, Recorded: recorded, Excluded: excluded, ExcludedExternal: &external, ExcludedUnbound: &unbound, AutoClosed: fleetEnded}
	if action == "capture" || action == "manual" {
		p.ObservedAt = &observed
	}
	payload, _ := json.Marshal(p)
	if err = q.Audit(ctx, store.AuditParams{EventID: id, ActorID: actor, Action: auditAction, RequestKey: key, Payload: payload}); err != nil {
		return result, err
	}
	if err = tx.Commit(ctx); err != nil {
		return result, err
	}
	v := event(updated)
	v.CanManage = true
	return ChangeResult{Event: v, Recorded: recorded, Excluded: excluded, ExcludedExternal: &external, ExcludedUnbound: &unbound, AutoClosed: fleetEnded}, nil
}

func (s *Service) EventCorporations(ctx context.Context) ([]int64, error) {
	return store.New(s.Pool).EventCorporations(ctx)
}

type PAPCoinAward struct {
	Reference, AccountID string
	Previous, Units      int64
}

var ErrCoinRateRequired = errors.New("configure PAP coin rate before issuance")

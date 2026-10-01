package eve

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
	"slices"
	"time"
)

func (s *AuthorizationService) GuardCharacterLoss(ctx context.Context, tx pgx.Tx, char, id int64, owner []byte) error {
	c, e := store.New(tx).GetCredentialForUpdate(ctx, char)
	if e != nil {
		return e
	}
	if c.State == "reauthorize" || !slices.Contains(c.Scopes, LossReadScope) || subtle.ConstantTimeCompare(c.OwnerHash, owner) != 1 {
		return pgx.ErrNoRows
	}
	rows, e := store.CharacterLosses(ctx, tx, char, 0, id)
	if e != nil {
		return e
	}
	if len(rows) != 1 {
		return pgx.ErrNoRows
	}
	return nil
}

type lossArgs syncArgs

func (lossArgs) Kind() string { return "eve.character-killmails.v1" }

type lossWorker struct {
	river.WorkerDefaults[lossArgs]
	s *SyncService
}

func (w *lossWorker) Work(ctx context.Context, j *river.Job[lossArgs]) error {
	if !w.s.lossesEnabled {
		return river.JobSnooze(time.Hour)
	}
	return w.s.work(ctx, syncArgs(j.Args), j.ID, "killmails")
}
func (s *SyncService) SetLossesEnabled(v bool) { s.lossesEnabled = v }

type killRef struct {
	ID   int64  `json:"killmail_id"`
	Hash string `json:"killmail_hash"`
}
type lossCursor struct {
	Page    int       `json:"page"`
	Pages   int       `json:"pages"`
	Pending []killRef `json:"pending"`
	Expires time.Time `json:"expires"`
}
type CharacterLoss struct {
	ID               int64          `json:"id,string"`
	CharacterID      int64          `json:"character_id,string"`
	CorporationID    int64          `json:"corporation_id,string"`
	ShipTypeID       int64          `json:"ship_type_id,string"`
	ShipName         string         `json:"ship_name"`
	SolarSystemID    int64          `json:"solar_system_id,string"`
	SolarSystemName  string         `json:"solar_system_name"`
	At               time.Time      `json:"occurred_at"`
	ObservedAt       time.Time      `json:"observed_at"`
	Items            []BattleItem   `json:"items"`
	DamageTaken      int64          `json:"damage_taken,omitempty"`
	VictimAllianceID int64          `json:"victim_alliance_id,string,omitempty"`
	Attackers        []KillAttacker `json:"attackers,omitempty"`
}
type KillAttacker struct {
	CharacterID     int64   `json:"character_id,string,omitempty"`
	CorporationID   int64   `json:"corporation_id,string,omitempty"`
	AllianceID      int64   `json:"alliance_id,string,omitempty"`
	FactionID       int64   `json:"faction_id,string,omitempty"`
	ShipTypeID      int64   `json:"ship_type_id,string,omitempty"`
	WeaponTypeID    int64   `json:"weapon_type_id,string,omitempty"`
	DamageDone      int64   `json:"damage_done"`
	FinalBlow       bool    `json:"final_blow"`
	SecurityStatus  float64 `json:"security_status,omitempty"`
	Name            string  `json:"name,omitempty"`
	CorporationName string  `json:"corporation_name,omitempty"`
	AllianceName    string  `json:"alliance_name,omitempty"`
	ShipName        string  `json:"ship_name,omitempty"`
	WeaponName      string  `json:"weapon_name,omitempty"`
}
type lossBatch struct {
	Cursor lossCursor
	Ref    *killRef
	Loss   CharacterLoss
	Done   bool
}

func validKillRef(r killRef) bool {
	if r.ID <= 0 || len(r.Hash) != 40 {
		return false
	}
	for _, c := range r.Hash {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func (s *SyncService) collectLosses(ctx context.Context, t store.EveSyncTarget, c store.EveCredential) (syncResult, error) {
	out := syncResult{}
	if !slices.Contains(c.Scopes, LossReadScope) {
		return out, syncFault{Reason: "missing_scope"}
	}
	b := &lossBatch{Cursor: lossCursor{Page: 1}}
	raw, e := store.LossCursor(ctx, s.pool, t.ID, c.GrantGeneration)
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return out, e
	}
	if e == nil && json.Unmarshal(raw, &b.Cursor) != nil {
		return out, errESI
	}
	cur := &b.Cursor
	if cur.Page < 1 || cur.Page > 1000 {
		return out, errESI
	}
	if len(cur.Pending) == 0 {
		req := ESIRequest{Method: "GET", Path: fmt.Sprintf("/characters/%d/killmails/recent/?page=%d", c.CharacterID, cur.Page), CharacterID: c.CharacterID, Generation: c.GrantGeneration, Scopes: []string{LossReadScope}, ExpectPages: true}
		var refs []killRef
		response, e := s.auth.esi.Request(ctx, req, &refs)
		if e != nil {
			var f syncFault
			if cur.Page > 1 && errors.As(e, &f) && f.Status == 404 {
				return out, syncFault{Reason: "pagination_changed", Temporary: true}
			}
			return out, e
		}
		if refs == nil || len(refs) > 1000 || response.Pages < 1 || response.Pages > 1000 {
			return out, errESI
		}
		if cur.Page > 1 && cur.Pages != response.Pages {
			return out, syncFault{Reason: "pagination_changed", Temporary: true}
		}
		cur.Pages = response.Pages
		if cur.Expires.IsZero() || response.ExpiresAt.Before(cur.Expires) {
			cur.Expires = response.ExpiresAt
		}
		seen := map[int64]bool{}
		for _, r := range refs {
			if !validKillRef(r) || seen[r.ID] {
				return out, errESI
			}
			seen[r.ID] = true
		}
		if cur.Page == cur.Pages {
			id, hash, backfillErr := store.ArchivedLossMissingAttackers(ctx, s.pool, c.CharacterID, c.OwnerHash)
			if backfillErr != nil && !errors.Is(backfillErr, pgx.ErrNoRows) {
				return out, backfillErr
			}
			if backfillErr == nil && !seen[id] {
				ref := killRef{ID: id, Hash: hash}
				if !validKillRef(ref) {
					return out, errESI
				}
				refs = append(refs, ref)
			}
		}
		cur.Pending = refs
	}
	// One new detail per publication: budget waits never discard earlier progress.
	for len(cur.Pending) > 0 {
		ref := cur.Pending[0]
		known, e := store.KillmailSeen(ctx, s.pool, c.CharacterID, ref.ID, c.OwnerHash, ref.Hash)
		if e != nil {
			return out, e
		}
		if known {
			cur.Pending = cur.Pending[1:]
			continue
		}
		var detail struct {
			ID     int64     `json:"killmail_id"`
			At     time.Time `json:"killmail_time"`
			System int64     `json:"solar_system_id"`
			Victim struct {
				ID          int64      `json:"character_id"`
				Corporation int64      `json:"corporation_id"`
				Alliance    int64      `json:"alliance_id"`
				Ship        int64      `json:"ship_type_id"`
				DamageTaken int64      `json:"damage_taken"`
				Items       []killItem `json:"items"`
			} `json:"victim"`
			Attackers []struct {
				CharacterID    int64   `json:"character_id"`
				CorporationID  int64   `json:"corporation_id"`
				AllianceID     int64   `json:"alliance_id"`
				FactionID      int64   `json:"faction_id"`
				ShipTypeID     int64   `json:"ship_type_id"`
				WeaponTypeID   int64   `json:"weapon_type_id"`
				DamageDone     int64   `json:"damage_done"`
				FinalBlow      bool    `json:"final_blow"`
				SecurityStatus float64 `json:"security_status"`
			} `json:"attackers"`
		}
		r, e := s.auth.esi.Request(ctx, ESIRequest{Method: "GET", Path: fmt.Sprintf("/killmails/%d/%s/", ref.ID, ref.Hash)}, &detail)
		if e != nil {
			return out, e
		}
		if detail.ID != ref.ID || detail.At.IsZero() || detail.System <= 0 || detail.Victim.Ship <= 0 || r.ValidatedAt.IsZero() {
			return out, errESI
		}
		items := []BattleItem{}
		flattenKillItems(detail.Victim.Items, "", &items)
		if len(items) > 2000 {
			return out, errESI
		}
		for _, i := range items {
			if i.TypeID <= 0 || i.Destroyed < 0 || i.Dropped < 0 || i.Quantity < 0 {
				return out, errESI
			}
		}
		if detail.Victim.DamageTaken < 0 || len(detail.Attackers) > 2000 {
			return out, errESI
		}
		attackers := make([]KillAttacker, 0, len(detail.Attackers))
		for _, a := range detail.Attackers {
			if a.DamageDone < 0 || a.CharacterID < 0 || a.CorporationID < 0 || a.ShipTypeID < 0 || a.WeaponTypeID < 0 {
				return out, errESI
			}
			attackers = append(attackers, KillAttacker{CharacterID: a.CharacterID, CorporationID: a.CorporationID, AllianceID: a.AllianceID, FactionID: a.FactionID, ShipTypeID: a.ShipTypeID, WeaponTypeID: a.WeaponTypeID, DamageDone: a.DamageDone, FinalBlow: a.FinalBlow, SecurityStatus: a.SecurityStatus})
		}
		nameIDs := make([]int64, 0, min(len(attackers)*3, 100))
		seenNames := map[int64]bool{}
		addName := func(id int64) {
			if id > 0 && !seenNames[id] && len(nameIDs) < 100 {
				seenNames[id] = true
				nameIDs = append(nameIDs, id)
			}
		}
		for _, a := range attackers {
			addName(a.CharacterID)
			addName(a.CorporationID)
			addName(a.AllianceID)
		}
		if len(nameIDs) > 0 {
			body, _ := json.Marshal(nameIDs)
			var names []struct {
				ID       int64  `json:"id"`
				Name     string `json:"name"`
				Category string `json:"category"`
			}
			if _, nameErr := s.auth.esi.Request(ctx, ESIRequest{Method: "POST", Path: "/universe/names/", Body: body}, &names); nameErr == nil {
				resolved := map[int64]string{}
				for _, n := range names {
					if n.Category == "character" || n.Category == "corporation" || n.Category == "alliance" {
						resolved[n.ID] = n.Name
					}
				}
				for i := range attackers {
					attackers[i].Name = resolved[attackers[i].CharacterID]
					attackers[i].CorporationName = resolved[attackers[i].CorporationID]
					attackers[i].AllianceName = resolved[attackers[i].AllianceID]
				}
			}
		}
		b.Ref = &ref
		b.Loss = CharacterLoss{ID: ref.ID, CharacterID: detail.Victim.ID, CorporationID: detail.Victim.Corporation, VictimAllianceID: detail.Victim.Alliance, ShipTypeID: detail.Victim.Ship, SolarSystemID: detail.System, At: detail.At, ObservedAt: r.ValidatedAt, Items: items, DamageTaken: detail.Victim.DamageTaken, Attackers: attackers}
		cur.Pending = cur.Pending[1:]
		break
	}
	if len(cur.Pending) == 0 {
		if cur.Page >= cur.Pages {
			b.Done = true
		} else {
			cur.Page++
		}
	}
	out.losses = b
	out.next = maxTime(cur.Expires, time.Now().Add(time.Minute))
	if b.Ref != nil {
		out.content = b.Loss.ObservedAt
	}
	return out, nil
}
func (s *SyncService) saveLossBatch(ctx context.Context, tx pgx.Tx, t store.EveSyncTarget, c store.EveCredential, b *lossBatch) error {
	if b.Ref != nil {
		var payload []byte
		if b.Loss.CharacterID == c.CharacterID {
			payload, _ = json.Marshal(b.Loss)
		}
		if e := store.SaveKillmail(ctx, tx, c.CharacterID, b.Ref.ID, b.Loss.CharacterID, c.OwnerHash, b.Ref.Hash, b.Loss.At, b.Loss.ObservedAt, payload); e != nil {
			return e
		}
	}
	if b.Done {
		return store.DeleteLossCursor(ctx, tx, t.ID)
	}
	raw, _ := json.Marshal(b.Cursor)
	return store.SaveLossCursor(ctx, tx, t.ID, c.GrantGeneration, raw)
}

// Object authorization is supplied by the host before calling these local reads.
func (s *AuthorizationService) CharacterLosses(ctx context.Context, character, before, id int64) ([]CharacterLoss, error) {
	rows, e := store.CharacterLosses(ctx, s.pool, character, before, id)
	if e != nil {
		return nil, e
	}
	out := []CharacterLoss{}
	for _, b := range rows {
		var v CharacterLoss
		if e = json.Unmarshal(b, &v); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, nil
}

// CachedLossNames decorates attacker identities without adding ESI requests to
// the approval read path. Unknown names stay absent instead of blocking review.
func (s *AuthorizationService) CachedLossNames(ctx context.Context, ids []int64) map[int64]string {
	if len(ids) == 0 {
		return nil
	}
	rows, err := store.New(s.pool).ReadEntityNames(ctx, store.ReadEntityNamesParams{Ids: ids, Language: "en"})
	if err != nil {
		return nil
	}
	out := make(map[int64]string, len(rows))
	for _, row := range rows {
		if row.Name != "" {
			out[row.EntityID] = row.Name
		}
	}
	return out
}

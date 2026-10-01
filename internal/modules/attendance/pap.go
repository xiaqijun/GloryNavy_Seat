package attendance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/attendance/internal/store"
	"slices"
	"strconv"
	"strings"
	"time"
)

type PAPChange struct {
	Version    int64  `json:"version,string"`
	RequestKey string `json:"request_key"`
	Points     int32  `json:"points"`
	Revoke     bool   `json:"revoke"`
	Supplement bool   `json:"supplement,omitempty"`
	Reason     string `json:"reason"`
}

// SetPAP reconciles the event's character balances, never adds the full score twice.
// The event lock serializes capture/close/reopen and all PAP operations.
func (s *Service) SetPAP(ctx context.Context, user string, id int64, c PAPChange) error {
	c.Reason = strings.TrimSpace(c.Reason)
	actor, e := uuid(user)
	key, err := uuid(c.RequestKey)
	if e != nil || err != nil || c.Version < 1 || c.Points < 1 || c.Points > 10000 || len([]rune(c.Reason)) < 1 || len([]rune(c.Reason)) > 200 {
		return ErrInvalid
	}
	q := store.New(s.Pool)
	ev, err := q.GetEvent(ctx, id)
	if err != nil {
		return err
	}
	if err = s.require(ctx, user, ev.CorporationID); err != nil {
		return err
	}
	raw, _ := json.Marshal(c)
	sum := sha256.Sum256(raw)
	fingerprint := hex.EncodeToString(sum[:])
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	q = store.New(tx)
	ev, err = q.LockEvent(ctx, id)
	if err != nil {
		return err
	}
	if old, e := q.FindAudit(ctx, store.FindAuditParams{EventID: id, RequestKey: key}); e == nil {
		var p auditPayload
		if json.Unmarshal(old.Payload, &p) != nil {
			return ErrUnavailable
		}
		if old.Action != "pap" || old.ActorID != actor || p.Fingerprint != fingerprint {
			return ErrConflict
		}
		return nil
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return e
	}
	if ev.State != "closed" || ev.Version != c.Version {
		return ErrConflict
	}
	if c.Supplement && (c.Revoke || !ev.PapIssued || ev.PapPoints != c.Points) {
		return ErrConflict
	}
	if c.Revoke && !ev.PapIssued || !c.Revoke && ev.PapIssued && ev.PapPoints == c.Points && !c.Supplement {
		return ErrConflict
	}
	entries, err := q.ListEntries(ctx, id)
	if err != nil {
		return err
	}
	old, err := q.PAPAwards(ctx, id)
	if err != nil {
		return err
	}
	balances := map[int64]store.AttendancePapAward{}
	for _, a := range old {
		balances[a.CharacterID] = a
	}
	desired := map[int64]store.AttendancePapAward{}
	if !c.Revoke {
		for _, v := range entries {
			if v.Present && v.AccountID.Valid {
				desired[v.CharacterID] = store.AttendancePapAward{EventID: id, CharacterID: v.CharacterID, AccountID: v.AccountID, CharacterName: v.CharacterName, Points: c.Points}
			}
		}
		if len(desired) == 0 {
			return ErrInvalid
		}
	}
	for char, prior := range balances {
		if _, ok := desired[char]; !ok {
			prior.Points = 0
			desired[char] = prior
		}
	}
	// Persist changes and ledger in deterministic character order.
	ids := make([]int64, 0, len(desired))
	for id := range desired {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	accounts := []string{}
	for _, a := range desired {
		account := a.AccountID.String()
		if !slices.Contains(accounts, account) {
			accounts = append(accounts, account)
		}
	}
	slices.Sort(accounts)
	for _, account := range accounts {
		if err = q.LockPAPAccount(ctx, account); err != nil {
			return err
		}
	}
	if s.CoinAwards != nil {
		awards := []PAPCoinAward{}
		for _, char := range ids {
			a := desired[char]
			awards = append(awards, PAPCoinAward{Reference: strconv.FormatInt(id, 10) + "/" + strconv.FormatInt(char, 10), AccountID: a.AccountID.String(), Previous: int64(balances[char].Points), Units: int64(a.Points)})
		}
		if err = s.CoinAwards(ctx, tx, c.RequestKey, c.Reason, awards); err != nil {
			return err
		}
	}
	var delta int64
	for _, char := range ids {
		a := desired[char]
		prior, exists := balances[char]
		if exists && prior.AccountID != a.AccountID {
			return ErrConflict
		}
		diff := a.Points - prior.Points
		if diff == 0 {
			continue
		}
		if err = q.SavePAPAward(ctx, store.SavePAPAwardParams{EventID: id, CharacterID: char, AccountID: a.AccountID, CharacterName: a.CharacterName, Points: a.Points}); err != nil {
			return err
		}
		if err = q.AddPAPLedger(ctx, store.AddPAPLedgerParams{EventID: id, CharacterID: char, AccountID: a.AccountID, ActorID: actor, RequestKey: key, Delta: diff, Balance: a.Points, Reason: c.Reason}); err != nil {
			return err
		}
		delta += int64(diff)
	}
	if c.Supplement && delta == 0 {
		return ErrConflict
	}
	if err = q.SetEventPAP(ctx, store.SetEventPAPParams{ID: id, PapPoints: c.Points, PapIssued: !c.Revoke}); err != nil {
		return err
	}
	payload, _ := json.Marshal(auditPayload{Fingerprint: fingerprint, Reason: c.Reason, PAPPoints: c.Points, PAPDelta: delta, PAPRevoked: c.Revoke})
	if err = q.Audit(ctx, store.AuditParams{EventID: id, ActorID: actor, Action: "pap", RequestKey: key, Payload: payload}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type PAPRow struct {
	EventID     int64     `json:"event_id,string"`
	CharacterID int64     `json:"character_id,string"`
	AccountID   string    `json:"account_id"`
	Name        string    `json:"name"`
	Title       string    `json:"title"`
	StartsAt    time.Time `json:"starts_at"`
	Points      int32     `json:"points"`
}
type PAPReport struct {
	Points         int64    `json:"points"`
	Events         int64    `json:"events"`
	Participations int64    `json:"participations"`
	Rows           []PAPRow `json:"rows"`
	More           bool     `json:"more"`
}

func (s *Service) PAPReport(ctx context.Context, user string, corp int64, period string, page int32) (PAPReport, error) {
	out := PAPReport{Rows: []PAPRow{}}
	if corp < 0 || page < 0 || page > 42949672 || (period != "month" && period != "30d") {
		return out, ErrInvalid
	}
	actor, err := uuid(user)
	if err != nil {
		return out, err
	}
	if corp > 0 {
		if err = s.require(ctx, user, corp); err != nil {
			return out, err
		}
	}
	tz := time.FixedZone("Asia/Shanghai", 8*3600)
	now := time.Now().In(tz)
	until := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, tz)
	since := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, tz)
	if period == "30d" {
		since = until.AddDate(0, 0, -30)
	}
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return out, err
	}
	defer tx.Rollback(context.Background())
	q := store.New(tx)
	totals, err := q.PAPTotals(ctx, store.PAPTotalsParams{Since: stamp(since), Until: stamp(until), CorporationID: corp, AccountID: actor})
	if err != nil {
		return out, err
	}
	out.Points, out.Events, out.Participations = totals.Points, totals.Events, totals.Participations
	rows, err := q.PAPReport(ctx, store.PAPReportParams{Since: stamp(since), Until: stamp(until), CorporationID: corp, AccountID: actor, RowOffset: page * 50})
	if err != nil {
		return out, err
	}
	if len(rows) > 50 {
		out.More = true
		rows = rows[:50]
	}
	for _, r := range rows {
		out.Rows = append(out.Rows, PAPRow{r.EventID, r.CharacterID, r.AccountID.String(), r.CharacterName, r.Title, r.StartsAt.Time, r.Points})
	}
	return out, tx.Commit(ctx)
}

type PAPLedgerItem struct {
	ID          int64     `json:"id,string"`
	CharacterID int64     `json:"character_id,string"`
	Name        string    `json:"name"`
	Delta       int32     `json:"delta"`
	Balance     int32     `json:"balance"`
	Reason      string    `json:"reason"`
	At          time.Time `json:"created_at"`
}
type PAPHistory struct {
	Items      []PAPLedgerItem `json:"items"`
	NextCursor string          `json:"next_cursor"`
}

func (s *Service) PAPHistory(ctx context.Context, user string, id, after int64) (PAPHistory, error) {
	out := PAPHistory{Items: []PAPLedgerItem{}}
	detail, err := s.Detail(ctx, user, id)
	if err != nil {
		return out, err
	}
	actor, err := uuid(user)
	if err != nil {
		return out, err
	}
	rows, err := store.New(s.Pool).PAPLedger(ctx, store.PAPLedgerParams{EventID: id, AfterID: after, Manage: detail.Event.CanManage, AccountID: actor})
	if err != nil {
		return out, err
	}
	if len(rows) > 50 {
		rows = rows[:50]
		out.NextCursor = strconv.FormatInt(rows[49].ID, 10)
	}
	for _, r := range rows {
		out.Items = append(out.Items, PAPLedgerItem{r.ID, r.CharacterID, r.CharacterName, r.Delta, r.Balance, r.Reason, r.CreatedAt.Time})
	}
	return out, nil
}

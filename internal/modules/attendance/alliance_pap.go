package attendance

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"glorynavy.local/seat/internal/modules/attendance/internal/store"
)

// AlliancePAPConfig configures the private monthly snapshot service.
// The token is read from a file at execution time and never enters a job
// payload, response or log line.
type AlliancePAPConfig struct {
	URL      string
	AuthFile string
}

type alliancePAPRow struct {
	CharacterID   string `json:"character_id"`
	CharacterName string `json:"character_name"`
	PAP           string `json:"pap"`
}
type alliancePAPResponse struct {
	OK           bool             `json:"ok"`
	Source       string           `json:"source"`
	Year         int              `json:"year"`
	Month        int              `json:"month"`
	Complete     bool             `json:"complete"`
	RecordsTotal int              `json:"records_total"`
	Rows         []alliancePAPRow `json:"rows"`
}

type AlliancePAPReport struct {
	Source       string                 `json:"source"`
	Month        string                 `json:"month"`
	Points       float64                `json:"points"`
	Available    bool                   `json:"available"`
	Complete     bool                   `json:"complete"`
	State        string                 `json:"state"`
	RecordsTotal int32                  `json:"records_total"`
	LastSyncedAt *time.Time             `json:"last_synced_at"`
	Characters   []AlliancePAPCharacter `json:"characters"`
	Version      int64                  `json:"version,string"`
	CanManage    bool                   `json:"can_manage"`
}

// AlliancePAPFulfillmentReport is the administrator-only aggregate used by
// the operations dashboard. The denominator is one per bound site account;
// all of that account's bound characters are summed before checking the
// monthly target.
type AlliancePAPFulfillmentReport struct {
	Source           string     `json:"source"`
	Month            string     `json:"month"`
	Target           int32      `json:"target"`
	EligibleAccounts int32      `json:"eligible_accounts"`
	AchievedAccounts int32      `json:"achieved_accounts"`
	RateBPS          int32      `json:"rate_bps"`
	Available        bool       `json:"available"`
	State            string     `json:"state"`
	LastSyncedAt     *time.Time `json:"last_synced_at"`
	Version          int64      `json:"version,string"`
}

type AlliancePAPMembersReport struct {
	Source       string              `json:"source"`
	Month        string              `json:"month"`
	Target       int32               `json:"target"`
	Available    bool                `json:"available"`
	Complete     bool                `json:"complete"`
	State        string              `json:"state"`
	RecordsTotal int32               `json:"records_total"`
	LastSyncedAt *time.Time          `json:"last_synced_at"`
	Version      int64               `json:"version,string"`
	Members      []AlliancePAPMember `json:"members"`
}

// AlliancePAPMemberMonths lists complete snapshots that administrators may
// select when reviewing member totals.
func (s *Service) AlliancePAPMemberMonths(ctx context.Context, user string) ([]string, error) {
	if s.Administrator == nil || s.Pool == nil {
		return nil, pgx.ErrNoRows
	}
	ok, err := s.Administrator(ctx, user)
	if err != nil || !ok {
		return nil, pgx.ErrNoRows
	}
	q := store.New(s.Pool)
	histories, err := q.AlliancePAPSyncHistories(ctx)
	if err != nil {
		return nil, err
	}
	months := make([]string, 0, len(histories)+1)
	seen := make(map[string]struct{}, len(histories)+1)
	current, err := q.AlliancePAPSync(ctx)
	if err != nil {
		return nil, err
	}
	if current.Month.Year() >= 2020 && ((current.State == "ready" && current.Complete) || (current.State == "error" && current.RecordsTotal > 0)) {
		value := current.Month.Format("2006-01")
		months = append(months, value)
		seen[value] = struct{}{}
	}
	for _, history := range histories {
		value := history.Month.Format("2006-01")
		if _, ok := seen[value]; ok {
			continue
		}
		months = append(months, value)
		seen[value] = struct{}{}
	}
	return months, nil
}

type AlliancePAPMember struct {
	UserID     string                 `json:"user_id"`
	Name       string                 `json:"name"`
	Points     float64                `json:"points"`
	Achieved   bool                   `json:"achieved"`
	Characters []AlliancePAPCharacter `json:"characters"`
}

type AlliancePAPCharacter struct {
	CharacterID   int64   `json:"character_id"`
	CharacterName string  `json:"character_name"`
	PAP           float64 `json:"pap"`
}

type AlliancePAPConversionMonth struct {
	Month   string `json:"month"`
	Version int64  `json:"version,string"`
	PAPCoinQuote
}

func allianceMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func parseAlliancePAP(value string) error {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 32 {
		return ErrInvalid
	}
	r := new(big.Rat)
	if _, ok := r.SetString(value); !ok || r.Sign() < 0 || r.Num().BitLen() > 60 || r.Denom().BitLen() > 20 {
		return ErrInvalid
	}
	// The upstream currently returns two decimal places. Keep the database
	// scale explicit so retries cannot silently change a value.
	if strings.Contains(value, ".") && len(strings.TrimRight(strings.SplitN(value, ".", 2)[1], "0")) > 2 {
		return ErrInvalid
	}
	return nil
}

func (s *Service) allianceToken() (string, error) {
	if s.AlliancePAP.URL == "" || s.AlliancePAP.AuthFile == "" {
		return "", pgx.ErrNoRows
	}
	b, err := os.ReadFile(s.AlliancePAP.AuthFile)
	if err != nil {
		return "", err
	}
	return parseAlliancePAPToken(b)
}

// parseAlliancePAPToken accepts the least-privilege read_token used by the
// current seat-pap service and the legacy api_token field used by its original
// auth file. Keeping the fallback here lets a rotated service file be adopted
// without putting a secret in application configuration or a River payload.
func parseAlliancePAPToken(b []byte) (string, error) {
	var v struct {
		ReadToken string `json:"read_token"`
		APIToken  string `json:"api_token"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return "", errors.New("alliance PAP read token is unavailable")
	}
	if token := strings.TrimSpace(v.ReadToken); token != "" {
		return token, nil
	}
	if token := strings.TrimSpace(v.APIToken); token != "" {
		return token, nil
	}
	return "", errors.New("alliance PAP read token is unavailable")
}

func (s *Service) fetchAlliancePAP(ctx context.Context) (alliancePAPResponse, error) {
	var out alliancePAPResponse
	token, err := s.allianceToken()
	if err != nil {
		return out, err
	}
	endpoint := strings.TrimRight(s.AlliancePAP.URL, "/") + "/api/pap/monthly"
	// The private service can briefly return a gateway error while refreshing
	// its source page. Retry one transient network/5xx failure within the
	// worker's bounded context so a short upstream flap does not invalidate the
	// last complete local snapshot.
	client := &http.Client{Timeout: 25 * time.Second}
	for attempt := 0; attempt < 2; attempt++ {
		req, requestErr := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if requestErr != nil {
			return out, requestErr
		}
		req.Header.Set("Authorization", "Bearer "+token)
		res, requestErr := client.Do(req)
		if requestErr != nil {
			if attempt == 0 && ctx.Err() == nil {
				time.Sleep(250 * time.Millisecond)
				continue
			}
			return out, requestErr
		}
		if res.StatusCode >= 500 && res.StatusCode <= 599 && attempt == 0 {
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
			time.Sleep(250 * time.Millisecond)
			continue
		}
		if res.StatusCode != http.StatusOK {
			_ = res.Body.Close()
			return out, fmt.Errorf("alliance PAP upstream status %d", res.StatusCode)
		}
		decodeErr := json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&out)
		_ = res.Body.Close()
		if decodeErr != nil {
			return out, errors.New("invalid alliance PAP response")
		}
		break
	}
	if !out.OK || out.Source != "winterco" || !out.Complete || out.Year < 2020 || out.Month < 1 || out.Month > 12 || out.RecordsTotal != len(out.Rows) || len(out.Rows) > 10000 {
		return out, errors.New("incomplete alliance PAP snapshot")
	}
	seen := make(map[int64]struct{}, len(out.Rows))
	for _, row := range out.Rows {
		id, err := strconv.ParseInt(strings.TrimSpace(row.CharacterID), 10, 64)
		if err != nil || id <= 0 || row.CharacterName == "" || parseAlliancePAP(row.PAP) != nil {
			return out, ErrInvalid
		}
		if _, ok := seen[id]; ok {
			return out, errors.New("duplicate alliance PAP character")
		}
		seen[id] = struct{}{}
	}
	return out, nil
}

// SyncAlliancePAP fetches a complete upstream snapshot before publishing it.
// Existing rows are retained when the upstream is unavailable or incomplete.
func (s *Service) SyncAlliancePAP(ctx context.Context) error {
	before, err := store.New(s.Pool).AlliancePAPSync(ctx)
	if err != nil {
		return err
	}
	response, err := s.fetchAlliancePAP(ctx)
	month := allianceMonth(time.Now().UTC())
	if err == nil && (response.Year != month.Year() || response.Month != int(month.Month())) {
		err = ErrConflict
	}
	if err != nil {
		// An older failed request must not replace a newer successful publication.
		tx, e := s.Pool.Begin(ctx)
		if e == nil {
			defer tx.Rollback(context.Background())
			q := store.New(tx)
			now, e := q.LockAlliancePAPSync(ctx)
			if e == nil && now.Version == before.Version {
				if e = q.MarkAlliancePAPError(ctx, month, "upstream_unavailable"); e == nil {
					_ = tx.Commit(ctx)
				}
			}
		}
		return err
	}
	oldRows, err := store.New(s.Pool).AllianceMonthAwards(ctx, month)
	if err != nil {
		return err
	}
	ids := make([]int64, 0, len(response.Rows))
	for _, row := range response.Rows {
		id, _ := strconv.ParseInt(row.CharacterID, 10, 64)
		ids = append(ids, id)
	}
	for _, row := range oldRows {
		ids = append(ids, row.CharacterID)
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	q := store.New(tx)
	bindings, err := s.Bindings(ctx, tx, ids)
	if err != nil {
		return err
	}
	byID := make(map[int64]Binding, len(bindings))
	for _, b := range bindings {
		byID[b.ID] = b
	}
	accounts := []string{}
	for _, row := range oldRows {
		if row.AccountID.Valid {
			accounts = append(accounts, row.AccountID.String())
		}
	}
	for _, b := range bindings {
		accounts = append(accounts, b.UserID)
	}
	if s.AllianceLockAccounts != nil {
		if err = s.AllianceLockAccounts(ctx, tx, accounts); err != nil {
			return err
		}
	}
	current, err := q.LockAlliancePAPSync(ctx)
	if err != nil {
		return err
	}
	if current.Version != before.Version {
		return ErrConflict
	}
	oldRows, err = q.AllianceMonthAwards(ctx, month)
	if err != nil {
		return err
	}
	previous := map[int64]store.AlliancePAPAward{}
	for _, row := range oldRows {
		previous[row.CharacterID] = row
	}
	awards := []PAPCoinAward{}
	publishedIDs := []int64{}
	if err = q.MarkAlliancePAPSyncing(ctx, month); err != nil {
		return err
	}
	for _, row := range response.Rows {
		id, _ := strconv.ParseInt(row.CharacterID, 10, 64)
		binding := byID[id]
		var account *pgtype.UUID
		if binding.UserID != "" {
			v, parseErr := uuid(binding.UserID)
			if parseErr != nil {
				return parseErr
			}
			account = &v
		}
		publishedIDs = append(publishedIDs, id)
		old, known := previous[id]
		delete(previous, id)
		units, _ := alliancePAPUnits(row.PAP)
		prev := units // First observation of each bound character/month is a baseline.
		if known && old.AccountID.Valid {
			prev, _ = alliancePAPUnits(old.PAP)
		}
		if old.AccountID.Valid {
			account = &old.AccountID
		}
		if account != nil {
			// Retained ownership never permits new coins for a detached/transferred character.
			if binding.UserID != account.String() && units > prev {
				prev = units
			}
			awards = append(awards, PAPCoinAward{Reference: "alliance/" + month.Format("2006-01") + "/" + strconv.FormatInt(id, 10), AccountID: account.String(), Previous: prev, Units: units})
		}
		if err = q.SaveAlliancePAP(ctx, month, id, row.CharacterName, row.PAP, account); err != nil {
			return err
		}
	}
	for id, old := range previous {
		if old.AccountID.Valid {
			prev, _ := alliancePAPUnits(old.PAP)
			awards = append(awards, PAPCoinAward{Reference: "alliance/" + month.Format("2006-01") + "/" + strconv.FormatInt(id, 10), AccountID: old.AccountID.String(), Previous: prev, Units: 0})
		}
	}
	if s.AllianceCoinAwards != nil {
		if err = s.AllianceCoinAwards(ctx, tx, allianceSyncKey(), "联盟 PAP 同步", awards); err != nil {
			return err
		}
	}
	if err = q.PruneAlliancePAP(ctx, month, publishedIDs); err != nil {
		return err
	}
	if err = q.MarkAlliancePAPReady(ctx, month, int32(response.RecordsTotal)); err != nil {
		return err
	}
	if err = q.MarkAlliancePAPHistoryReady(ctx, month, int32(response.RecordsTotal)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) AlliancePAPReport(ctx context.Context, user string) (AlliancePAPReport, error) {
	out := AlliancePAPReport{Source: "alliance", Characters: []AlliancePAPCharacter{}}
	if _, err := uuid(user); err != nil {
		return out, ErrInvalid
	}
	r, err := store.New(s.Pool).AlliancePAPAccount(ctx, user)
	if err != nil {
		return out, err
	}
	if r.Month.Year() > 1 {
		out.Month = r.Month.Format("2006-01")
	}
	out.Complete, out.State, out.RecordsTotal = r.Complete, r.State, r.RecordsTotal
	out.Version = r.Version
	if s.Administrator != nil {
		out.CanManage, _ = s.Administrator(ctx, user)
	}
	out.Available = r.State == "ready" && r.Complete && out.Month != ""
	if out.Month != time.Now().UTC().Format("2006-01") {
		out.Month = time.Now().UTC().Format("2006-01")
		out.Available = false
	}
	if out.Points, err = strconv.ParseFloat(r.Points, 64); err != nil {
		return out, err
	}
	if out.Available {
		rows, rowErr := store.New(s.Pool).AlliancePAPCharacters(ctx, user)
		if rowErr != nil {
			return out, rowErr
		}
		out.Characters = make([]AlliancePAPCharacter, 0, len(rows))
		for _, row := range rows {
			points, parseErr := strconv.ParseFloat(row.PAP, 64)
			if parseErr != nil {
				return out, parseErr
			}
			out.Characters = append(out.Characters, AlliancePAPCharacter{CharacterID: row.CharacterID, CharacterName: row.CharacterName, PAP: points})
		}
	}
	if r.LastSyncedAt.Valid && r.LastSyncedAt.Time.Year() > 1 {
		t := r.LastSyncedAt.Time
		out.LastSyncedAt = &t
	}
	return out, nil
}

// AlliancePAPFulfillmentReport returns the current-month aggregate for site
// administrators. It deliberately uses the last complete local snapshot and
// never treats a partial or failed sync as a zero-rate result.
func (s *Service) AlliancePAPFulfillmentReport(ctx context.Context, user string) (AlliancePAPFulfillmentReport, error) {
	out := AlliancePAPFulfillmentReport{Source: "alliance", State: "idle"}
	if _, err := uuid(user); err != nil {
		return out, ErrInvalid
	}
	if s.Administrator == nil {
		return out, pgx.ErrNoRows
	}
	ok, err := s.Administrator(ctx, user)
	if err != nil {
		return out, err
	}
	if !ok {
		return out, pgx.ErrNoRows
	}
	q := store.New(s.Pool)
	requirement, err := q.PAPRequirement(ctx)
	if err != nil {
		return out, err
	}
	sync, err := q.AlliancePAPSync(ctx)
	if err != nil {
		return out, err
	}
	out.Target = requirement.MonthlyPoints
	out.State = sync.State
	out.Version = sync.Version
	if sync.LastSyncedAt.Valid && sync.LastSyncedAt.Time.Year() > 1 {
		t := sync.LastSyncedAt.Time
		out.LastSyncedAt = &t
	}
	if sync.Month.Year() > 1 {
		out.Month = sync.Month.Format("2006-01")
	}
	nowMonth := time.Now().UTC().Format("2006-01")
	if out.Month != nowMonth || sync.State != "ready" || !sync.Complete {
		out.Month = nowMonth
		return out, nil
	}
	out.Available = true
	out.Month = nowMonth
	out.EligibleAccounts, out.AchievedAccounts, err = q.AlliancePAPFulfillment(ctx, sync.Month, requirement.MonthlyPoints)
	if err != nil {
		return out, err
	}
	if out.EligibleAccounts > 0 {
		out.RateBPS = int32((int64(out.AchievedAccounts)*10000 + int64(out.EligibleAccounts)/2) / int64(out.EligibleAccounts))
	}
	return out, nil
}

// AlliancePAPMembersReport returns the selected complete snapshot grouped by
// currently bound site account. It is administrator-only and rechecks the
// live character ownership before exposing another member's points.
func (s *Service) AlliancePAPMembersReport(ctx context.Context, user, monthValue string) (AlliancePAPMembersReport, error) {
	out := AlliancePAPMembersReport{Source: "alliance", State: "idle", Version: 1, Members: []AlliancePAPMember{}}
	if _, err := uuid(user); err != nil {
		return out, ErrInvalid
	}
	if s.Administrator == nil || s.Bindings == nil || s.Pool == nil {
		return out, pgx.ErrNoRows
	}
	ok, err := s.Administrator(ctx, user)
	if err != nil {
		return out, err
	}
	if !ok {
		return out, pgx.ErrNoRows
	}
	month, err := parseAlliancePAPMonth(monthValue)
	if err != nil {
		return out, err
	}
	out.Month = month.Format("2006-01")
	requirement, err := store.New(s.Pool).PAPRequirement(ctx)
	if err != nil {
		return out, err
	}
	out.Target = requirement.MonthlyPoints
	q := store.New(s.Pool)
	nowMonth := allianceMonth(time.Now().UTC())
	if month.Equal(nowMonth) {
		sync, syncErr := q.AlliancePAPSync(ctx)
		if syncErr != nil {
			return out, syncErr
		}
		out.State, out.Complete, out.RecordsTotal, out.Version = sync.State, sync.Complete, sync.RecordsTotal, sync.Version
		if sync.LastSyncedAt.Valid && sync.LastSyncedAt.Time.Year() > 1 {
			t := sync.LastSyncedAt.Time
			out.LastSyncedAt = &t
		}
		// An upstream failure does not erase a locally retained snapshot. Admin
		// member review may use those rows while the response still exposes the
		// error state for the UI to surface.
		out.Available = sync.Month.Equal(month) && ((sync.State == "ready" && sync.Complete) || (sync.State == "error" && sync.RecordsTotal > 0))
	} else {
		histories, historyErr := q.AlliancePAPSyncHistories(ctx)
		if historyErr != nil {
			return out, historyErr
		}
		for _, history := range histories {
			if !history.Month.Equal(month) {
				continue
			}
			out.State, out.Complete, out.RecordsTotal, out.Version = history.State, history.Complete, history.RecordsTotal, history.Version
			if history.LastSyncedAt.Valid && history.LastSyncedAt.Time.Year() > 1 {
				t := history.LastSyncedAt.Time
				out.LastSyncedAt = &t
			}
			out.Available = history.State == "ready" && history.Complete
			break
		}
	}
	if !out.Available {
		return out, nil
	}
	rows, err := q.AlliancePAPMemberRows(ctx, month)
	if err != nil {
		return out, err
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.CharacterID)
	}
	bindings, err := s.Bindings(ctx, nil, ids)
	if err != nil {
		return out, err
	}
	bound := make(map[int64]string, len(bindings))
	for _, binding := range bindings {
		bound[binding.ID] = binding.UserID
	}
	accountRows := make(map[string]int)
	accountIDs := make([]string, 0)
	for _, row := range rows {
		accountID := row.AccountID.String()
		if bound[row.CharacterID] != accountID {
			continue
		}
		if _, ok := accountRows[accountID]; !ok {
			accountRows[accountID] = len(out.Members)
			accountIDs = append(accountIDs, accountID)
			out.Members = append(out.Members, AlliancePAPMember{UserID: accountID, Characters: []AlliancePAPCharacter{}})
		}
		index := accountRows[accountID]
		points, parseErr := strconv.ParseFloat(row.PAP, 64)
		if parseErr != nil {
			return out, parseErr
		}
		out.Members[index].Points += points
		out.Members[index].Characters = append(out.Members[index].Characters, AlliancePAPCharacter{CharacterID: row.CharacterID, CharacterName: row.CharacterName, PAP: points})
	}
	if s.AlliancePAPMemberNames != nil && len(accountIDs) > 0 {
		names, nameErr := s.AlliancePAPMemberNames(ctx, accountIDs)
		if nameErr != nil {
			return out, nameErr
		}
		for i := range out.Members {
			out.Members[i].Name = names[out.Members[i].UserID]
		}
	}
	for i := range out.Members {
		out.Members[i].Achieved = out.Members[i].Points >= float64(out.Target)
	}
	return out, nil
}

func parseAlliancePAPMonth(value string) (time.Time, error) {
	if value == "" {
		return allianceMonth(time.Now().UTC()), nil
	}
	month, err := time.Parse("2006-01", value)
	if err != nil || month.Format("2006-01") != value || month.Year() < 2020 {
		return time.Time{}, ErrInvalid
	}
	return allianceMonth(month), nil
}

// ConvertAlliancePAP converts one complete alliance snapshot once per
// character reference. The exchange service owns the coin ledger and replay
// protection; attendance only supplies the frozen source snapshot.
func (s *Service) ConvertAlliancePAP(ctx context.Context, user string, c *PAPConversion, monthValues ...string) (PAPCoinQuote, error) {
	var out PAPCoinQuote
	if s.Administrator == nil || s.AllianceCoinConversion == nil || s.Pool == nil {
		return out, pgx.ErrNoRows
	}
	ok, err := s.Administrator(ctx, user)
	if err != nil || !ok {
		return out, pgx.ErrNoRows
	}
	monthValue := ""
	if len(monthValues) > 0 {
		monthValue = monthValues[0]
	}
	if c != nil && c.Month != "" {
		if monthValue != "" && monthValue != c.Month {
			return out, ErrInvalid
		}
		monthValue = c.Month
	}
	month, err := parseAlliancePAPMonth(monthValue)
	if err != nil {
		return out, err
	}
	return s.convertAlliancePAPMonth(ctx, user, c, month)
}

func (s *Service) convertAlliancePAPMonth(ctx context.Context, user string, c *PAPConversion, month time.Time) (PAPCoinQuote, error) {
	var out PAPCoinQuote
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(context.Background())
	q := store.New(tx)
	history, err := q.LockAlliancePAPSyncHistory(ctx, month)
	if err != nil {
		return out, err
	}
	if history.State != "ready" || !history.Complete {
		return out, ErrConflict
	}
	preview, err := q.AllianceMonthAwards(ctx, month)
	if err != nil {
		return out, err
	}
	ids := make([]int64, 0, len(preview))
	accounts := []string{user}
	for _, v := range preview {
		ids = append(ids, v.CharacterID)
		if v.AccountID.Valid {
			accounts = append(accounts, v.AccountID.String())
		}
	}
	bindings, err := s.Bindings(ctx, tx, ids)
	if err != nil {
		return out, err
	}
	bound := map[int64]string{}
	for _, b := range bindings {
		bound[b.ID] = b.UserID
		accounts = append(accounts, b.UserID)
	}
	if s.AllianceLockAccounts != nil {
		if err = s.AllianceLockAccounts(ctx, tx, accounts); err != nil {
			return out, err
		}
	}
	if c != nil && c.Version != history.Version {
		return out, ErrConflict
	}
	awards := make([]PAPCoinAward, 0, len(preview))
	for _, row := range preview {
		if !row.AccountID.Valid || bound[row.CharacterID] != row.AccountID.String() {
			continue
		}
		units, parseErr := alliancePAPUnits(row.PAP)
		if parseErr != nil || units == 0 {
			continue
		}
		awards = append(awards, PAPCoinAward{
			Reference: "alliance/" + month.Format("2006-01") + "/" + strconv.FormatInt(row.CharacterID, 10),
			AccountID: row.AccountID.String(),
			Previous:  0,
			Units:     units,
		})
	}
	out, err = s.AllianceCoinConversion(ctx, tx, user, valueString(c), valueReason(c), valueToken(c), awards)
	if c != nil && err == nil {
		err = tx.Commit(ctx)
	}
	return out, err
}

// AlliancePAPConversionMonths returns complete retained months and their
// current pending quote. It is administrator-only because it exposes the
// settlement queue for all mapped accounts.
func (s *Service) AlliancePAPConversionMonths(ctx context.Context, user string) ([]AlliancePAPConversionMonth, error) {
	if s.Administrator == nil || s.AllianceCoinConversion == nil || s.Pool == nil {
		return nil, pgx.ErrNoRows
	}
	ok, err := s.Administrator(ctx, user)
	if err != nil || !ok {
		return nil, pgx.ErrNoRows
	}
	histories, err := store.New(s.Pool).AlliancePAPSyncHistories(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AlliancePAPConversionMonth, 0, len(histories))
	for _, history := range histories {
		quote, err := s.convertAlliancePAPMonth(ctx, user, nil, history.Month)
		if err != nil {
			return nil, err
		}
		if quote.Pending == 0 {
			continue
		}
		out = append(out, AlliancePAPConversionMonth{Month: history.Month.Format("2006-01"), Version: history.Version, PAPCoinQuote: quote})
	}
	return out, nil
}

func valueString(c *PAPConversion) string {
	if c == nil {
		return ""
	}
	return c.RequestKey
}
func valueReason(c *PAPConversion) string {
	if c == nil {
		return ""
	}
	return c.Reason
}
func valueToken(c *PAPConversion) string {
	if c == nil {
		return ""
	}
	return c.Token
}

func alliancePAPUnits(value string) (int64, error) {
	value = strings.TrimSpace(value)
	parts := strings.SplitN(value, ".", 2)
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || whole < 0 || whole > 90000000000000000 {
		return 0, ErrInvalid
	}
	frac := int64(0)
	if len(parts) == 2 {
		if len(parts[1]) > 2 {
			return 0, ErrInvalid
		}
		f := parts[1] + strings.Repeat("0", 2-len(parts[1]))
		frac, err = strconv.ParseInt(f, 10, 64)
		if err != nil {
			return 0, ErrInvalid
		}
	}
	if whole > 92233720368547758 || whole*100 > 9223372036854775807-frac {
		return 0, ErrInvalid
	}
	return whole*100 + frac, nil
}

func allianceSyncKey() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}

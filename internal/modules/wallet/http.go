// Package wallet exposes read-only EVE ISK wallets. Synchronization is owned by eve.
package wallet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/module"
	"glorynavy.local/seat/internal/modules/eve"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Owner struct {
	Kind         string `json:"kind"`
	ID           int64  `json:"id,string"`
	Name         string `json:"name"`
	Divisions    []int  `json:"divisions"`
	Journal      bool   `json:"journal"`
	Transactions bool   `json:"transactions"`
}
type Handler struct {
	User                           func(*http.Request) string
	Owners                         func(context.Context, string, string) ([]Owner, error)
	PersonalOwners                 func(context.Context, string) ([]Owner, error)
	Authorize                      func(context.Context, string, string, int64) (Owner, error)
	Data                           func(context.Context, eve.WalletFilter) ([]map[string]any, error)
	Summaries                      func(context.Context, []int64, time.Time) ([]eve.WalletSummary, error)
	CorporationSummary             func(context.Context, int64, []int, time.Time) (eve.WalletSummary, error)
	CorporationFinanceTrend        func(context.Context, int64, []int, time.Time, time.Time) ([]eve.WalletFinanceTrend, error)
	CorporationPersonalSummary     func(context.Context, string, int64, time.Time) (eve.WalletSummary, error)
	CorporationPersonalIncomeTrend func(context.Context, string, int64, time.Time, time.Time) ([]eve.WalletIncomeTrend, error)
	Names                          func(context.Context, []int64) (map[string]string, error)
	Types                          interface {
		TypeNames(context.Context, []int64) (map[int64]eve.StaticTypeName, error)
	}
}

func (h Handler) Module() module.Definition {
	return module.Definition{Manifest: module.Manifest{ID: "wallet", Version: "0.1.0", APIVersion: 1, Requires: []module.Dependency{{ID: "identity", APIVersion: 1}, {ID: "eve", APIVersion: 1}, {ID: "access", APIVersion: 1}}}, Permissions: []string{"wallet.self"}, Routes: []module.Route{
		{Method: "GET", Path: "/context", Permission: "wallet.self", Handler: http.HandlerFunc(h.context)},
		{Method: "GET", Path: "/records", Permission: "wallet.self", Handler: http.HandlerFunc(h.records)},
		{Method: "GET", Path: "/summary", Permission: "wallet.self", Handler: http.HandlerFunc(h.summary)},
		{Method: "GET", Path: "/corporation-summary", Permission: "wallet.self", Handler: http.HandlerFunc(h.corporationSummary)},
		{Method: "GET", Path: "/corporation-finance-trend", Permission: "wallet.self", Handler: http.HandlerFunc(h.corporationFinanceTrend)},
		{Method: "GET", Path: "/personal-corporation-summary", Permission: "wallet.self", Handler: http.HandlerFunc(h.personalCorporationSummary)},
		{Method: "GET", Path: "/personal-corporation-income-trend", Permission: "wallet.self", Handler: http.HandlerFunc(h.personalCorporationIncomeTrend)},
	}}
}

func parseSummaryFrom(r *http.Request) (time.Time, error) {
	from, e := time.Parse(time.RFC3339, r.URL.Query().Get("from"))
	if e != nil || from.After(time.Now()) || time.Since(from) > 45*24*time.Hour {
		return time.Time{}, errors.New("invalid summary range")
	}
	return from, nil
}
func walletError(w http.ResponseWriter, r *http.Request, e error) {
	if errors.Is(e, pgx.ErrNoRows) {
		httpapi.Failure(w, r, 404, "wallet_unavailable", "钱包不可用")
	} else {
		httpapi.Failure(w, r, 503, "wallet_unavailable", "钱包暂不可用")
	}
}
func (h Handler) context(w http.ResponseWriter, r *http.Request) {
	v, e := h.Owners(r.Context(), h.User(r), r.URL.Query().Get("member"))
	if e != nil {
		walletError(w, r, e)
		return
	}
	httpapi.Respond(w, r, 200, map[string]any{"owners": v})
}

func (h Handler) summary(w http.ResponseWriter, r *http.Request) {
	from, e := parseSummaryFrom(r)
	if e != nil {
		httpapi.Failure(w, r, 400, "invalid_request", "钱包查询条件无效")
		return
	}
	// This endpoint is for the signed-in account's workbench. Do not accept a
	// member selector: privileged cross-account reads use the object API.
	actor := h.User(r)
	var owners []Owner
	if h.PersonalOwners != nil {
		owners, e = h.PersonalOwners(r.Context(), actor)
	} else {
		owners, e = h.Owners(r.Context(), actor, "")
	}
	if e != nil {
		walletError(w, r, e)
		return
	}
	ids := make([]int64, 0, len(owners))
	for _, owner := range owners {
		if owner.Kind == "character" && owner.Journal && len(owner.Divisions) == 1 && owner.Divisions[0] == 0 {
			ids = append(ids, owner.ID)
		}
	}
	rows, e := h.Summaries(r.Context(), ids, from)
	if e != nil {
		walletError(w, r, e)
		return
	}
	httpapi.Respond(w, r, 200, map[string]any{"items": rows})
}

func (h Handler) corporationSummary(w http.ResponseWriter, r *http.Request) {
	from, e := parseSummaryFrom(r)
	ownerID, parseErr := parseID(r.URL.Query().Get("owner_id"), false)
	if e != nil || parseErr != nil || h.CorporationSummary == nil {
		httpapi.Failure(w, r, 400, "invalid_request", "钱包查询条件无效")
		return
	}
	owner, e := h.Authorize(r.Context(), h.User(r), "corporation", ownerID)
	if e != nil || owner.Kind != "corporation" || !owner.Journal || len(owner.Divisions) == 0 {
		walletError(w, r, pgx.ErrNoRows)
		return
	}
	row, e := h.CorporationSummary(r.Context(), ownerID, owner.Divisions, from)
	if e != nil {
		walletError(w, r, e)
		return
	}
	httpapi.Respond(w, r, 200, map[string]any{"items": []eve.WalletSummary{row}})
}

func (h Handler) corporationFinanceTrend(w http.ResponseWriter, r *http.Request) {
	from, until, rangeErr := parseTrendRange(r)
	ownerID, parseErr := parseID(r.URL.Query().Get("owner_id"), false)
	if rangeErr != nil || parseErr != nil || h.CorporationFinanceTrend == nil {
		httpapi.Failure(w, r, 400, "invalid_request", "钱包趋势查询条件无效")
		return
	}
	owner, e := h.Authorize(r.Context(), h.User(r), "corporation", ownerID)
	if e != nil || owner.Kind != "corporation" || !owner.Journal || len(owner.Divisions) == 0 {
		walletError(w, r, pgx.ErrNoRows)
		return
	}
	rows, e := h.CorporationFinanceTrend(r.Context(), ownerID, owner.Divisions, from, until)
	if e != nil {
		walletError(w, r, e)
		return
	}
	httpapi.Respond(w, r, 200, map[string]any{"items": rows})
}

func (h Handler) personalCorporationSummary(w http.ResponseWriter, r *http.Request) {
	from, e := parseSummaryFrom(r)
	corporationID, parseErr := parseID(r.URL.Query().Get("corporation_id"), false)
	if e != nil || parseErr != nil || h.CorporationPersonalSummary == nil {
		httpapi.Failure(w, r, 400, "invalid_request", "钱包查询条件无效")
		return
	}
	row, e := h.CorporationPersonalSummary(r.Context(), h.User(r), corporationID, from)
	if e != nil {
		walletError(w, r, e)
		return
	}
	httpapi.Respond(w, r, 200, map[string]any{"items": []eve.WalletSummary{row}})
}

func parseTrendRange(r *http.Request) (time.Time, time.Time, error) {
	from, e := time.Parse(time.RFC3339, r.URL.Query().Get("from"))
	if e != nil {
		return time.Time{}, time.Time{}, e
	}
	until := time.Now().UTC()
	if raw := r.URL.Query().Get("until"); raw != "" {
		until, e = time.Parse(time.RFC3339, raw)
		if e != nil {
			return time.Time{}, time.Time{}, e
		}
	}
	if from.After(until) || until.After(time.Now().Add(2*time.Minute)) || until.Sub(from) > 370*24*time.Hour {
		return time.Time{}, time.Time{}, errors.New("invalid trend range")
	}
	return from, until, nil
}

func (h Handler) personalCorporationIncomeTrend(w http.ResponseWriter, r *http.Request) {
	from, until, rangeErr := parseTrendRange(r)
	corporationID, parseErr := parseID(r.URL.Query().Get("corporation_id"), false)
	if rangeErr != nil || parseErr != nil || h.CorporationPersonalIncomeTrend == nil {
		httpapi.Failure(w, r, 400, "invalid_request", "钱包趋势查询条件无效")
		return
	}
	rows, e := h.CorporationPersonalIncomeTrend(r.Context(), h.User(r), corporationID, from, until)
	if e != nil {
		walletError(w, r, e)
		return
	}
	httpapi.Respond(w, r, 200, map[string]any{"items": rows})
}
func parseID(v string, optional bool) (int64, error) {
	if optional && v == "" {
		return 0, nil
	}
	n, e := strconv.ParseInt(v, 10, 64)
	if e != nil || n <= 0 || strconv.FormatInt(n, 10) != v {
		return 0, errors.New("invalid ID")
	}
	return n, nil
}
func parseFilter(r *http.Request) (eve.WalletFilter, error) {
	q := r.URL.Query()
	f := eve.WalletFilter{Kind: q.Get("owner_kind"), Part: q.Get("part"), Search: strings.TrimSpace(q.Get("search")), Ref: q.Get("ref_type"), Direction: q.Get("direction")}
	var e error
	if f.Kind != "character" && f.Kind != "corporation" {
		return f, errors.New("invalid owner")
	}
	if f.Part != "balance" && f.Part != "journal" && f.Part != "transactions" && f.Part != "divisions" {
		return f, errors.New("invalid part")
	}
	if len(f.Search) > 200 || len(f.Ref) > 100 {
		return f, errors.New("invalid filter")
	}
	if f.Owner, e = parseID(q.Get("owner_id"), false); e != nil {
		return f, e
	}
	before, entry, hasEntry := strings.Cut(q.Get("before"), ".")
	if hasEntry && (f.Part != "journal" || !regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(entry)) {
		return f, errors.New("invalid cursor")
	}
	f.BeforeEntry = entry
	if f.Before, e = parseID(before, true); e != nil {
		return f, e
	}
	if f.Party, e = parseID(q.Get("party_id"), true); e != nil {
		return f, e
	}
	if f.ID, e = parseID(q.Get("id"), true); e != nil {
		return f, e
	}
	if f.Kind == "corporation" {
		f.Division, e = strconv.Atoi(q.Get("division"))
		if e != nil || f.Division < 1 || f.Division > 7 {
			return f, errors.New("invalid division")
		}
	}
	if f.Direction != "" && !((f.Part == "journal" && (f.Direction == "in" || f.Direction == "out")) || (f.Part == "transactions" && (f.Direction == "buy" || f.Direction == "sell"))) {
		return f, errors.New("invalid direction")
	}
	for k, dst := range map[string]**time.Time{"from": &f.From, "until": &f.Until} {
		if v := q.Get(k); v != "" {
			t, e := time.Parse(time.RFC3339, v)
			if e != nil {
				return f, e
			}
			*dst = &t
		}
	}
	if f.From != nil && f.Until != nil && !f.Until.After(*f.From) {
		return f, errors.New("invalid date range")
	}
	return f, nil
}
func (h Handler) records(w http.ResponseWriter, r *http.Request) {
	f, e := parseFilter(r)
	if e != nil {
		httpapi.Failure(w, r, 400, "invalid_request", "钱包查询条件无效")
		return
	}
	o, e := h.Authorize(r.Context(), h.User(r), f.Kind, f.Owner)
	if e != nil {
		walletError(w, r, e)
		return
	}
	allowed := false
	for _, d := range o.Divisions {
		if d == f.Division {
			allowed = true
		}
	}
	if !allowed || (f.Part == "journal" && !o.Journal) || (f.Part == "transactions" && !o.Transactions) {
		walletError(w, r, pgx.ErrNoRows)
		return
	}
	// Division names are a single corporate snapshot; only requested, authorized name is exposed.
	selectedDivision := f.Division
	if f.Part == "divisions" {
		if f.Kind != "corporation" {
			walletError(w, r, pgx.ErrNoRows)
			return
		}
		f.Division = 0
	}
	rows, e := h.Data(r.Context(), f)
	if e != nil {
		walletError(w, r, e)
		return
	}
	next := ""
	if len(rows) > 50 {
		rows = rows[:50]
		next, _ = rows[49]["id"].(string)
		if entry, ok := rows[49]["entry_key"].(string); ok && entry != "" {
			next += "." + entry
		}
	}
	if f.Part == "divisions" {
		for _, row := range rows {
			raw, _ := row["wallet"].(json.RawMessage)
			var names []struct {
				Division int    `json:"division"`
				Name     string `json:"name"`
			}
			_ = json.Unmarshal(raw, &names)
			name := ""
			for _, n := range names {
				if n.Division == selectedDivision {
					name = n.Name
				}
			}
			delete(row, "hangar")
			row["wallet"] = nil
			row["name"] = name
			row["division"] = selectedDivision
		}
	}
	ids, types := []int64{}, []int64{}
	for _, row := range rows {
		for _, k := range []string{"first_party_id", "second_party_id", "client_id", "tax_receiver_id"} {
			if s, ok := row[k].(string); ok {
				n, _ := strconv.ParseInt(s, 10, 64)
				if n > 0 {
					ids = append(ids, n)
				}
			}
		}
		if s, ok := row["type_id"].(string); ok {
			n, _ := strconv.ParseInt(s, 10, 64)
			types = append(types, n)
		}
	}
	names := map[string]string{}
	if h.Names != nil && len(ids) > 0 {
		if n, e := h.Names(r.Context(), ids); e == nil {
			names = n
		}
	}
	if h.Types != nil && len(types) > 0 {
		if values, e := h.Types.TypeNames(r.Context(), types); e == nil {
			for _, row := range rows {
				n, _ := strconv.ParseInt(fmt.Sprint(row["type_id"]), 10, 64)
				if v, ok := values[n]; ok {
					row["type_name"] = v.Name
				}
			}
		}
	}
	httpapi.Respond(w, r, 200, map[string]any{"items": rows, "names": names, "next_cursor": next})
}

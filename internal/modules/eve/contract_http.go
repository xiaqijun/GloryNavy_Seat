package eve

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/module"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
)

type ContractOwner struct {
	Kind       string `json:"kind"`
	ID         int64  `json:"id,string"`
	Name       string `json:"name"`
	AllianceID int64  `json:"-"`
	CEOID      int64  `json:"-"`
}

// Owners is injected by the host. Every request rechecks current bindings and
// corporation policy, including detail pages and pagination requests.
type ContractHTTP struct {
	pool         *pgxpool.Pool
	esi          *ESIService
	User         func(*http.Request) string
	Owners       func(context.Context, string) ([]ContractOwner, error)
	MemberOwners func(context.Context, string, string) ([]ContractOwner, error)
	LookupOwner  func(context.Context, string, string, int64) (ContractOwner, error)
	// BindingOwner is supplied by identity; private location names require a current binding.
	BindingOwner  func(context.Context, int64) ([]byte, error)
	locationMu    sync.Mutex
	locationRetry map[structureKey]time.Time
	StaticData    interface {
		TypeNames(context.Context, []int64) (map[int64]StaticTypeName, error)
	}
}

func NewContractHTTP(pool *pgxpool.Pool, auth *AuthorizationService, user func(*http.Request) string) *ContractHTTP {
	h := &ContractHTTP{pool: pool, User: user, StaticData: NewStaticData(pool, "", 0)}
	if auth != nil {
		h.esi = auth.esi
	}
	return h
}
func (h *ContractHTTP) Corporations(ctx context.Context) ([]ContractOwner, error) {
	rows, e := store.New(h.pool).ReadContractCorporations(ctx)
	result := make([]ContractOwner, 0, len(rows))
	for _, r := range rows {
		result = append(result, ContractOwner{"corporation", r.CorporationID, r.CorporationName, r.AllianceID, r.CeoID})
	}
	return result, e
}
func (h *ContractHTTP) Routes() []module.Route {
	result := []module.Route{}
	for _, r := range []struct {
		path    string
		handler http.HandlerFunc
	}{
		{"/contracts/owners", h.owners},
		{"/contracts/{kind}/{id}", h.list},
		{"/contracts/{kind}/{id}/{contract}", h.detail},
		{"/contracts/{kind}/{id}/{contract}/items", h.items},
		{"/contracts/{kind}/{id}/{contract}/bids", h.bids},
	} {
		result = append(result, module.Route{Method: "GET", Path: r.path, Permission: "eve.contracts.read", Handler: r.handler})
	}
	return result
}
func contractFailure(w http.ResponseWriter, r *http.Request, e error) {
	if errors.Is(e, pgx.ErrNoRows) {
		httpapi.Failure(w, r, 404, "contract_unavailable", "合同不存在或没有查看权限")
		return
	}
	httpapi.Failure(w, r, 503, "contracts_unavailable", "合同暂时无法读取，请重试")
}
func (h *ContractHTTP) owners(w http.ResponseWriter, r *http.Request) {
	var rows []ContractOwner
	var e error
	if member := r.URL.Query().Get("member"); member != "" {
		if h.MemberOwners == nil {
			contractFailure(w, r, pgx.ErrNoRows)
			return
		}
		rows, e = h.MemberOwners(r.Context(), h.User(r), member)
	} else {
		rows, e = h.Owners(r.Context(), h.User(r))
	}
	if e != nil {
		contractFailure(w, r, e)
		return
	}
	if rows == nil {
		rows = []ContractOwner{}
	}
	httpapi.Respond(w, r, 200, map[string]any{"owners": rows})
}
func (h *ContractHTTP) owned(w http.ResponseWriter, r *http.Request) (ContractOwner, bool) {
	id, e := parseSyncID(r)
	kind := chi.URLParam(r, "kind")
	if e != nil || !slices.Contains([]string{"character", "corporation"}, kind) {
		contractFailure(w, r, pgx.ErrNoRows)
		return ContractOwner{}, false
	}
	if h.LookupOwner != nil {
		o, err := h.LookupOwner(r.Context(), h.User(r), kind, id)
		if err != nil {
			contractFailure(w, r, err)
			return ContractOwner{}, false
		}
		return o, true
	}
	owners, e := h.Owners(r.Context(), h.User(r))
	if e != nil {
		contractFailure(w, r, e)
		return ContractOwner{}, false
	}
	for _, o := range owners {
		if o.Kind == kind && o.ID == id {
			return o, true
		}
	}
	contractFailure(w, r, pgx.ErrNoRows)
	return ContractOwner{}, false
}
func readCursor(raw string, fallback int64) (int64, error) {
	if raw == "" {
		return fallback, nil
	}
	id, e := strconv.ParseInt(raw, 10, 64)
	if e != nil || id <= 0 || strconv.FormatInt(id, 10) != raw {
		return 0, errors.New("invalid cursor")
	}
	return id, nil
}

type contractEntity struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Category     string `json:"category"`
	NameLanguage string `json:"name_language"`
}
type contractView struct {
	ID             string         `json:"id"`
	Title          string         `json:"title"`
	Summary        string         `json:"summary"`
	TradeDirection string         `json:"trade_direction"`
	Type           string         `json:"type"`
	Status         string         `json:"status"`
	Availability   string         `json:"availability"`
	ForCorporation bool           `json:"for_corporation"`
	Issuer         contractEntity `json:"issuer"`
	Assignee       contractEntity `json:"assignee"`
	Acceptor       contractEntity `json:"acceptor"`
	Start          contractEntity `json:"start"`
	End            contractEntity `json:"end"`
	Price          *string        `json:"price"`
	Reward         *string        `json:"reward"`
	Collateral     *string        `json:"collateral"`
	Buyout         *string        `json:"buyout"`
	Volume         *string        `json:"volume"`
	DaysToComplete *string        `json:"days_to_complete"`
	Issued         string         `json:"date_issued"`
	Expired        string         `json:"date_expired"`
	Accepted       string         `json:"date_accepted"`
	Completed      string         `json:"date_completed"`
	Checked        time.Time      `json:"checked_at"`
}

func entity(id int64, category string) contractEntity {
	if category == "station" && id >= 1000000000000 {
		category = "structure"
	}
	return contractEntity{ID: strconv.FormatInt(id, 10), Category: category}
}
func viewContract(row store.EveContract) (contractView, error) {
	var p struct {
		Title, Availability                       string
		ForCorporation                            bool  `json:"for_corporation"`
		Issuer                                    int64 `json:"issuer_id"`
		Corporation                               int64 `json:"issuer_corporation_id"`
		Assignee                                  int64 `json:"assignee_id"`
		Acceptor                                  int64 `json:"acceptor_id"`
		Start                                     int64 `json:"start_location_id"`
		End                                       int64 `json:"end_location_id"`
		Price, Reward, Collateral, Buyout, Volume json.Number
		Days                                      json.Number `json:"days_to_complete"`
		Issued                                    string      `json:"date_issued"`
		Expired                                   string      `json:"date_expired"`
		Accepted                                  string      `json:"date_accepted"`
		Completed                                 string      `json:"date_completed"`
	}
	if e := json.Unmarshal(row.Payload, &p); e != nil {
		return contractView{}, e
	}
	number := func(n json.Number) *string {
		if n == "" {
			return nil
		}
		s := string(n)
		return &s
	}
	issuer := entity(p.Issuer, "character")
	if p.ForCorporation {
		issuer = entity(p.Corporation, "corporation")
	}
	return contractView{ID: strconv.FormatInt(row.ContractID, 10), Title: p.Title, Type: row.ContractType, Status: row.Status, Availability: p.Availability, ForCorporation: p.ForCorporation, Issuer: issuer, Assignee: entity(p.Assignee, ""), Acceptor: entity(p.Acceptor, ""), Start: entity(p.Start, "station"), End: entity(p.End, "station"), Price: number(p.Price), Reward: number(p.Reward), Collateral: number(p.Collateral), Buyout: number(p.Buyout), Volume: number(p.Volume), DaysToComplete: number(p.Days), Issued: p.Issued, Expired: p.Expired, Accepted: p.Accepted, Completed: p.Completed, Checked: row.CheckedAt.Time}, nil
}

// Names are optional decoration. Public IDs only; never probe private structures.
// Failure preserves the contract and its exact IDs instead of hiding business data.
func (h *ContractHTTP) names(parent context.Context, entities ...*contractEntity) {
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	h.localizedTypeNames(ctx, entities...)
	ids := []int64{}
	seen := map[int64]bool{}
	for _, v := range entities {
		id, _ := strconv.ParseInt(v.ID, 10, 64)
		if v.Category == "inventory_type" {
			continue
		}
		if id > 0 && id < 1000000000000 && !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	if len(ids) == 0 {
		return
	}
	q := store.New(h.pool)
	cached, e := q.ReadEntityNames(ctx, store.ReadEntityNamesParams{Ids: ids, Language: "en"})
	if e != nil {
		return
	}
	found := map[int64]contractEntity{}
	for _, v := range cached {
		found[v.EntityID] = contractEntity{strconv.FormatInt(v.EntityID, 10), v.Name, v.Category, "en"}
	}
	missing := []int64{}
	for _, id := range ids {
		if _, ok := found[id]; !ok && len(missing) < 100 {
			missing = append(missing, id)
		}
	}
	if h.esi != nil && len(missing) > 0 {
		body, _ := json.Marshal(missing)
		var rows []struct {
			ID       int64  `json:"id"`
			Name     string `json:"name"`
			Category string `json:"category"`
		}
		if e = h.decorationRequest(ctx, ESIRequest{Method: "POST", Path: "/universe/names/", Body: body}, &rows); e == nil {
			for _, v := range rows {
				if !seen[v.ID] || v.Name == "" {
					continue
				}
				found[v.ID] = contractEntity{strconv.FormatInt(v.ID, 10), v.Name, v.Category, "en"}
				_ = q.SaveEntityName(ctx, store.SaveEntityNameParams{EntityID: v.ID, Name: v.Name, Category: v.Category, Language: "en"})
			}
		}
	}
	for _, v := range entities {
		id, _ := strconv.ParseInt(v.ID, 10, 64)
		if n, ok := found[id]; ok && v.Category != "inventory_type" {
			*v = n
		}
	}
}

// SDE is authoritative for static item names. Missing types use existing ESI
// name caches only: rendering an item page does not issue per-item ESI requests.
func (h *ContractHTTP) localizedTypeNames(ctx context.Context, entities ...*contractEntity) {
	ids := []int64{}
	for _, v := range entities {
		id, _ := strconv.ParseInt(v.ID, 10, 64)
		if v.Category == "inventory_type" && id > 0 {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return
	}
	if h.StaticData == nil {
		return
	}
	found, err := h.StaticData.TypeNames(ctx, ids)
	if err != nil {
		return
	}
	for _, v := range entities {
		id, _ := strconv.ParseInt(v.ID, 10, 64)
		if n, ok := found[id]; ok && v.Category == "inventory_type" {
			v.Name = n.Name
			v.NameLanguage = n.Language
		}
	}
}

func (h *ContractHTTP) decorate(parent context.Context, owner ContractOwner, rows []contractView) {
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	refs := []*contractEntity{}
	places := []*contractEntity{}
	for i := range rows {
		v := &rows[i]
		refs = append(refs, &v.Issuer, &v.Assignee, &v.Acceptor)
		places = append(places, &v.Start, &v.End)
	}
	// A bad party ID must not prevent resolving otherwise valid locations.
	var wg sync.WaitGroup
	wg.Go(func() { h.names(ctx, refs...) })
	wg.Go(func() { h.contractLocations(ctx, owner, places) })
	wg.Wait()
}
func (h *ContractHTTP) list(w http.ResponseWriter, r *http.Request) {
	o, ok := h.owned(w, r)
	if !ok {
		return
	}
	query := r.URL.Query()
	before, e := readCursor(query.Get("before"), math.MaxInt64)
	search := strings.TrimSpace(query.Get("q"))
	kind := query.Get("type")
	status := query.Get("status")
	if e != nil || len([]rune(search)) > 100 || !slices.Contains([]string{"", "item_exchange", "auction", "courier", "loan", "unknown"}, kind) || !slices.Contains([]string{"", "outstanding", "in_progress", "finished_issuer", "finished_contractor", "finished", "cancelled", "rejected", "failed", "deleted", "reversed"}, status) {
		httpapi.Failure(w, r, 400, "invalid_filter", "合同筛选条件无效")
		return
	}
	rows, e := store.New(h.pool).ReadContractList(r.Context(), store.ReadContractListParams{OwnerKind: o.Kind, OwnerID: o.ID, BeforeID: before, Search: search, Kind: kind, Status: status})
	if e != nil {
		contractFailure(w, r, e)
		return
	}
	next := ""
	if len(rows) > 25 {
		rows = rows[:25]
		next = strconv.FormatInt(rows[24].ContractID, 10)
	}
	items := make([]contractView, 0, len(rows))
	for _, row := range rows {
		v, e := viewContract(row)
		if e != nil {
			contractFailure(w, r, e)
			return
		}
		items = append(items, v)
	}
	h.decorate(r.Context(), o, items)
	h.summaries(r.Context(), o.Kind, o.ID, items)
	httpapi.Respond(w, r, 200, map[string]any{"items": items, "next_cursor": next})
}
func (h *ContractHTTP) record(w http.ResponseWriter, r *http.Request) (store.EveContract, bool) {
	o, ok := h.owned(w, r)
	if !ok {
		return store.EveContract{}, false
	}
	id, e := readCursor(chi.URLParam(r, "contract"), 0)
	if e != nil || id == 0 {
		contractFailure(w, r, pgx.ErrNoRows)
		return store.EveContract{}, false
	}
	row, e := store.New(h.pool).GetContract(r.Context(), store.GetContractParams{OwnerKind: o.Kind, OwnerID: o.ID, ContractID: id})
	if e != nil {
		contractFailure(w, r, e)
		return row, false
	}
	return row, true
}

type detailState struct {
	Part    string     `json:"part"`
	State   string     `json:"state"`
	Reason  string     `json:"reason"`
	Updated *time.Time `json:"updated_at"`
}

func (h *ContractHTTP) detail(w http.ResponseWriter, r *http.Request) {
	row, ok := h.record(w, r)
	if !ok {
		return
	}
	v, e := viewContract(row)
	if e != nil {
		contractFailure(w, r, e)
		return
	}
	states, e := store.New(h.pool).ReadContractDetailStates(r.Context(), store.ReadContractDetailStatesParams{OwnerKind: row.OwnerKind, OwnerID: row.OwnerID, ContractID: row.ContractID})
	if e != nil {
		contractFailure(w, r, e)
		return
	}
	ds := make([]detailState, 0, len(states))
	for _, s := range states {
		ds = append(ds, detailState{s.Part, s.State, s.Reason, optionalTime(s.LastSuccessAt)})
	}
	values := []contractView{v}
	h.decorate(r.Context(), ContractOwner{Kind: row.OwnerKind, ID: row.OwnerID}, values)
	h.summaries(r.Context(), row.OwnerKind, row.OwnerID, values)
	httpapi.Respond(w, r, 200, map[string]any{"contract": values[0], "details": ds})
}
func (h *ContractHTTP) items(w http.ResponseWriter, r *http.Request) {
	row, ok := h.record(w, r)
	if !ok {
		return
	}
	after, e := readCursor(r.URL.Query().Get("after"), 0)
	if e != nil {
		httpapi.Failure(w, r, 400, "invalid_cursor", "分页参数无效")
		return
	}
	rows, e := store.New(h.pool).ReadContractItems(r.Context(), store.ReadContractItemsParams{OwnerKind: row.OwnerKind, OwnerID: row.OwnerID, ContractID: row.ContractID, RecordID: after})
	if e != nil {
		contractFailure(w, r, e)
		return
	}
	next := ""
	if len(rows) > 100 {
		rows = rows[:100]
		next = strconv.FormatInt(rows[99].RecordID, 10)
	}
	type item struct {
		ID          string         `json:"id"`
		Type        contractEntity `json:"type"`
		Quantity    string         `json:"quantity"`
		Included    bool           `json:"included"`
		Singleton   bool           `json:"singleton"`
		RawQuantity *int64         `json:"raw_quantity"`
	}
	items := make([]item, 0, len(rows))
	for _, v := range rows {
		var raw *int64
		if v.RawQuantity.Valid {
			x := v.RawQuantity.Int64
			raw = &x
		}
		items = append(items, item{strconv.FormatInt(v.RecordID, 10), entity(v.TypeID, "inventory_type"), strconv.FormatInt(v.Quantity, 10), v.IsIncluded, v.IsSingleton, raw})
	}
	refs := []*contractEntity{}
	for i := range items {
		refs = append(refs, &items[i].Type)
	}
	h.names(r.Context(), refs...)
	httpapi.Respond(w, r, 200, map[string]any{"items": items, "next_cursor": next})
}
func (h *ContractHTTP) bids(w http.ResponseWriter, r *http.Request) {
	row, ok := h.record(w, r)
	if !ok {
		return
	}
	before, e := readCursor(r.URL.Query().Get("before"), math.MaxInt64)
	if e != nil {
		httpapi.Failure(w, r, 400, "invalid_cursor", "分页参数无效")
		return
	}
	rows, e := store.New(h.pool).ReadContractBids(r.Context(), store.ReadContractBidsParams{OwnerKind: row.OwnerKind, OwnerID: row.OwnerID, ContractID: row.ContractID, BidID: before})
	if e != nil {
		contractFailure(w, r, e)
		return
	}
	next := ""
	if len(rows) > 100 {
		rows = rows[:100]
		next = strconv.FormatInt(rows[99].BidID, 10)
	}
	type bid struct {
		ID     string         `json:"id"`
		Bidder contractEntity `json:"bidder"`
		Amount string         `json:"amount"`
		Date   time.Time      `json:"date"`
	}
	items := make([]bid, 0, len(rows))
	for _, v := range rows {
		items = append(items, bid{strconv.FormatInt(v.BidID, 10), entity(v.BidderID, "character"), v.Amount, v.DateBid.Time})
	}
	refs := []*contractEntity{}
	for i := range items {
		refs = append(refs, &items[i].Bidder)
	}
	h.names(r.Context(), refs...)
	httpapi.Respond(w, r, 200, map[string]any{"items": items, "next_cursor": next})
}

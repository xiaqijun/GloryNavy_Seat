package eve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
	"math/big"
	"slices"
	"strconv"
	"strings"
	"time"
)

const CharacterWalletScope = "esi-wallet.read_character_wallet.v1"
const CorporationWalletScope = "esi-wallet.read_corporation_wallets.v1"
const WalletDivisionsScope = "esi-corporations.read_divisions.v1"

func isWallet(r string) bool {
	return slices.Contains([]string{"wallet_balance", "wallet_journal", "wallet_transactions", "corporation_wallet_balance", "corporation_wallet_journal", "corporation_wallet_transactions", "corporation_wallet_divisions"}, r)
}
func walletPart(r string) string { a := strings.Split(r, "_"); return a[len(a)-1] }
func walletScope(r string) string {
	if r == "corporation_wallet_divisions" {
		return WalletDivisionsScope
	}
	if strings.HasPrefix(r, "corporation_") {
		return CorporationWalletScope
	}
	return CharacterWalletScope
}

type walletArgs struct {
	TargetID   int64  `json:"target_id"`
	Generation int64  `json:"generation"`
	Resource   string `json:"resource"`
}

func (walletArgs) Kind() string { return "eve.wallet-resource.v1" }

type walletWorker struct {
	river.WorkerDefaults[walletArgs]
	s *SyncService
}

func (w *walletWorker) Work(ctx context.Context, j *river.Job[walletArgs]) error {
	if !isWallet(j.Args.Resource) {
		return river.JobCancel(errors.New("invalid wallet resource"))
	}
	if !w.s.walletEnabled {
		return river.JobSnooze(time.Hour)
	}
	return w.s.work(ctx, syncArgs{j.Args.TargetID, j.Args.Generation}, j.ID, j.Args.Resource)
}
func (s *SyncService) SetWalletEnabled(v bool) { s.walletEnabled = v }

type walletCursor struct {
	Division, Page, Pages, Steps int
	From                         int64
	Expires                      time.Time
}
type walletRecord struct {
	ID       int64
	Division int
	At       *time.Time
	Raw      json.RawMessage
}
type walletBatch struct {
	Kind, Part string
	Owner      int64
	Cursor     walletCursor
	Records    []walletRecord
	Observed   time.Time
	Done       bool
}

func walletCorporation(ctx context.Context, db store.DBTX, c store.EveCredential, r string) (int64, error) {
	f, e := store.New(db).GetAuthorization(ctx, c.CharacterID)
	if e != nil {
		return 0, e
	}
	if !f.CorporationID.Valid || !f.ValidUntil.Time.After(time.Now()) || c.RolesNotBefore.Time.After(time.Now()) {
		return 0, syncFault{Reason: "authorization_pending", Temporary: true}
	}
	if f.CeoID.Int64 != c.CharacterID && !slices.Contains(f.Roles, "Director") && (walletPart(r) == "divisions" || (!slices.Contains(f.Roles, "Accountant") && !slices.Contains(f.Roles, "Junior_Accountant"))) {
		return 0, syncFault{Reason: "missing_role", Temporary: true}
	}
	source, e := store.WalletSource(ctx, db, f.CorporationID.Int64, r, walletScope(r), walletPart(r) == "divisions")
	if errors.Is(e, pgx.ErrNoRows) {
		return 0, syncFault{Reason: "authorization_pending", Temporary: true}
	}
	if e != nil {
		return 0, e
	}
	if source != c.CharacterID {
		return 0, syncFault{Reason: "shared_source", Temporary: true}
	}
	return f.CorporationID.Int64, nil
}

// Preserve decimal text end-to-end. No binary-float rounding of ISK or journal IDs.
func validWalletNumber(raw json.RawMessage) bool {
	if len(raw) == 0 || string(raw) == "null" || raw[0] == '"' {
		return false
	}
	var n json.Number
	if json.Unmarshal(raw, &n) != nil || len(n.String()) > 60 {
		return false
	}
	v, ok := new(big.Rat).SetString(n.String())
	return ok && v.Num().BitLen() < 128 && v.Denom().BitLen() < 80
}
func walletRecordID(m map[string]json.RawMessage, key string) (int64, error) {
	var n int64
	e := json.Unmarshal(m[key], &n)
	if e != nil || n <= 0 {
		return 0, errESI
	}
	return n, nil
}
func validateWalletRecord(raw json.RawMessage, part string) (walletRecord, error) {
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil || m == nil {
		return walletRecord{}, errESI
	}
	key := "id"
	if part == "transactions" {
		key = "transaction_id"
	}
	id, e := walletRecordID(m, key)
	if e != nil {
		return walletRecord{}, e
	}
	var at time.Time
	if json.Unmarshal(m["date"], &at) != nil || at.IsZero() {
		return walletRecord{}, errESI
	}
	if part == "journal" {
		var ref, description string
		if json.Unmarshal(m["ref_type"], &ref) != nil || ref == "" || json.Unmarshal(m["description"], &description) != nil {
			return walletRecord{}, errESI
		}
		for _, k := range []string{"amount", "balance", "tax"} {
			if b, ok := m[k]; ok && string(b) != "null" && !validWalletNumber(b) {
				return walletRecord{}, errESI
			}
		}
		for _, k := range []string{"first_party_id", "second_party_id", "context_id", "tax_receiver_id"} {
			if b, ok := m[k]; ok && string(b) != "null" {
				var n int64
				if json.Unmarshal(b, &n) != nil || n < 0 {
					return walletRecord{}, errESI
				}
			}
		}
		for _, k := range []string{"reason", "context_id_type"} {
			if v, ok := m[k]; ok && string(v) != "null" {
				var value string
				if json.Unmarshal(v, &value) != nil {
					return walletRecord{}, errESI
				}
			}
		}
	} else {
		for _, k := range []string{"type_id", "quantity", "client_id", "location_id"} {
			if _, e = walletRecordID(m, k); e != nil {
				return walletRecord{}, e
			}
		}
		var buy bool
		var journal int64
		if (string(m["is_buy"]) != "true" && string(m["is_buy"]) != "false") || json.Unmarshal(m["is_buy"], &buy) != nil || string(m["journal_ref_id"]) == "null" || json.Unmarshal(m["journal_ref_id"], &journal) != nil || journal < -1 || !validWalletNumber(m["unit_price"]) {
			return walletRecord{}, errESI
		}
		p, _ := new(big.Rat).SetString(string(m["unit_price"]))
		if p.Sign() < 0 {
			return walletRecord{}, errESI
		}
	}
	return walletRecord{ID: id, At: &at, Raw: raw}, nil
}
func (s *SyncService) collectWallet(ctx context.Context, t store.EveSyncTarget, c store.EveCredential) (syncResult, error) {
	out := syncResult{}
	scope := walletScope(t.Resource)
	if !slices.Contains(c.Scopes, scope) {
		return out, syncFault{Reason: "missing_scope"}
	}
	b := &walletBatch{Kind: "character", Owner: c.CharacterID, Part: walletPart(t.Resource), Cursor: walletCursor{Page: 1}}
	if strings.HasPrefix(t.Resource, "corporation_") {
		b.Kind = "corporation"
		var e error
		b.Owner, e = walletCorporation(ctx, s.pool, c, t.Resource)
		if e != nil {
			return out, e
		}
		b.Cursor.Division = 1
	}
	raw, e := store.WalletCursor(ctx, s.pool, t.ID, c.GrantGeneration, b.Owner)
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return out, e
	}
	if e == nil && json.Unmarshal(raw, &b.Cursor) != nil {
		return out, errESI
	}
	cur := &b.Cursor
	if cur.Page < 1 || cur.Page > 1000 || cur.Steps > 2000 || cur.Division < 0 || cur.Division > 7 {
		return out, errESI
	}
	base := fmt.Sprintf("/%ss/%d/wallet", b.Kind, b.Owner)
	if b.Kind == "corporation" {
		base += "s"
	}
	path := base + "/"
	if b.Part == "divisions" {
		path = fmt.Sprintf("/corporations/%d/divisions/", b.Owner)
	} else if b.Part != "balance" {
		if b.Kind == "corporation" {
			path = fmt.Sprintf("%s/%d/", base, cur.Division)
		}
		path += b.Part + "/"
		if b.Part == "journal" {
			path += fmt.Sprintf("?page=%d", cur.Page)
		} else if cur.From > 0 {
			path += fmt.Sprintf("?from_id=%d", cur.From)
		}
	}
	var payload json.RawMessage
	response, e := s.auth.esi.Request(ctx, ESIRequest{Method: "GET", Path: path, CharacterID: c.CharacterID, Generation: c.GrantGeneration, Scopes: []string{scope}, ExpectPages: b.Part == "journal"}, &payload)
	if e != nil {
		var f syncFault
		if errors.As(e, &f) && f.Status == 404 && cur.Page > 1 {
			return out, syncFault{Reason: "pagination_changed", Temporary: true}
		}
		return out, e
	}
	if response.ValidatedAt.IsZero() {
		return out, errESI
	}
	b.Observed = response.ValidatedAt
	if cur.Expires.IsZero() || response.ExpiresAt.Before(cur.Expires) {
		cur.Expires = response.ExpiresAt
	}
	cur.Steps++
	switch b.Part {
	case "balance":
		if b.Kind == "character" {
			if !validWalletNumber(payload) {
				return out, errESI
			}
			b.Records = []walletRecord{{Raw: json.RawMessage(`{"balance":` + string(payload) + `}`)}}
		} else {
			var values []struct {
				Division int             `json:"division"`
				Balance  json.RawMessage `json:"balance"`
			}
			if json.Unmarshal(payload, &values) != nil || len(values) != 7 {
				return out, errESI
			}
			seen := map[int]bool{}
			for _, v := range values {
				if v.Division < 1 || v.Division > 7 || seen[v.Division] || !validWalletNumber(v.Balance) {
					return out, errESI
				}
				seen[v.Division] = true
				b.Records = append(b.Records, walletRecord{Division: v.Division, Raw: json.RawMessage(`{"balance":` + string(v.Balance) + `}`)})
			}
		}
		b.Done = true
	case "divisions":
		var names struct {
			Wallet []struct {
				Division int    `json:"division"`
				Name     string `json:"name"`
			} `json:"wallet"`
		}
		if len(payload) == 0 || payload[0] != '{' || json.Unmarshal(payload, &names) != nil || len(names.Wallet) > 7 {
			return out, errESI
		}
		seen := map[int]bool{}
		for _, v := range names.Wallet {
			if v.Division < 1 || v.Division > 7 || seen[v.Division] || len(v.Name) > 500 {
				return out, errESI
			}
			seen[v.Division] = true
		}
		b.Records = []walletRecord{{Raw: payload}}
		b.Done = true
	default:
		var records []json.RawMessage
		if json.Unmarshal(payload, &records) != nil || records == nil || len(records) > 3000 {
			return out, errESI
		}
		minID := int64(0)
		for _, raw := range records {
			if b.Kind == "character" && b.Part == "transactions" {
				var fields map[string]json.RawMessage
				_ = json.Unmarshal(raw, &fields)
				if string(fields["is_personal"]) != "true" && string(fields["is_personal"]) != "false" {
					return out, errESI
				}
			}
			r, e := validateWalletRecord(raw, b.Part)
			if e != nil || (cur.From > 0 && r.ID > cur.From) {
				return out, errESI
			}
			// ESI may include the boundary entry despite the documented "before" wording.
			if cur.From > 0 && r.ID == cur.From {
				continue
			}
			r.Division = cur.Division
			b.Records = append(b.Records, r)
			if minID == 0 || r.ID < minID {
				minID = r.ID
			}
		}
		divisionDone := false
		if b.Part == "journal" {
			if response.Pages < 1 || response.Pages > 1000 {
				return out, errESI
			}
			if cur.Page > 1 && cur.Pages != response.Pages {
				return out, syncFault{Reason: "pagination_changed", Temporary: true}
			}
			cur.Pages = response.Pages
			divisionDone = cur.Page >= cur.Pages
			cur.Page++
		} else {
			divisionDone = minID == 0
			cur.From = minID
		}
		if divisionDone {
			if b.Kind == "character" || cur.Division == 7 {
				b.Done = true
			} else {
				cur.Division++
				cur.Page = 1
				cur.Pages = 0
				cur.From = 0
			}
		}
	}
	out.wallet = b
	out.next = maxTime(cur.Expires, time.Now().Add(time.Second))
	out.content = response.ContentUpdatedAt
	return out, nil
}
func (s *SyncService) saveWalletBatch(ctx context.Context, tx pgx.Tx, t store.EveSyncTarget, c store.EveCredential, b *walletBatch) error {
	for _, r := range b.Records {
		if e := store.SaveWalletObservation(ctx, tx, b.Kind, b.Owner, r.Division, b.Part, r.ID, c.CharacterID, c.OwnerHash, b.Observed, r.At, r.Raw); e != nil {
			return e
		}
	}
	if b.Done {
		return store.DeleteWalletCursor(ctx, tx, t.ID)
	}
	raw, _ := json.Marshal(b.Cursor)
	return store.SaveWalletCursor(ctx, tx, t.ID, c.GrantGeneration, b.Owner, raw)
}

// WalletData is a local-read boundary. The wallet module supplies object/division authorization.
type WalletFilter struct {
	Kind, Part, Search, Direction, Ref string
	BeforeEntry                        string
	Owner, Before, Party, ID           int64
	Division                           int
	From, Until                        *time.Time
}

func (s *AuthorizationService) WalletCorporations(ctx context.Context) ([]ContractOwner, error) {
	return NewContractHTTP(s.pool, nil, nil).Corporations(ctx)
}
func (s *AuthorizationService) WalletNames(ctx context.Context, ids []int64) (map[string]string, error) {
	h := NewContractHTTP(s.pool, s, nil)
	refs := []*contractEntity{}
	for _, id := range ids {
		refs = append(refs, &contractEntity{ID: strconv.FormatInt(id, 10)})
	}
	h.names(ctx, refs...)
	out := map[string]string{}
	for _, r := range refs {
		if r.Name != "" {
			out[r.ID] = r.Name
		}
	}
	return out, nil
}
func (s *AuthorizationService) WalletData(ctx context.Context, f WalletFilter) ([]map[string]any, error) {
	rows, e := store.ReadWallet(ctx, s.pool, store.WalletFilter{Kind: f.Kind, Part: f.Part, Search: f.Search, Direction: f.Direction, Ref: f.Ref, Owner: f.Owner, Before: f.Before, BeforeEntry: f.BeforeEntry, Party: f.Party, ID: f.ID, Division: f.Division, From: f.From, Until: f.Until})
	if e != nil {
		return nil, e
	}
	out := []map[string]any{}
	for _, r := range rows {
		var raw map[string]json.RawMessage
		if json.Unmarshal(r.Payload, &raw) != nil {
			return nil, errESI
		}
		item := map[string]any{}
		for k, v := range raw {
			if k == "id" || strings.HasSuffix(k, "_id") || slices.Contains([]string{"amount", "balance", "tax", "unit_price", "quantity"}, k) {
				if string(v) == "null" {
					item[k] = nil
				} else {
					item[k] = string(v)
				}
			} else {
				item[k] = v
			}
		}
		item["id"] = strconv.FormatInt(r.ID, 10)
		item["division"] = r.Division
		item["entry_key"] = r.EntryKey
		item["observed_at"] = r.Observed
		out = append(out, item)
	}
	return out, nil
}

type WalletSummary struct {
	OwnerID  int64      `json:"owner_id,string"`
	Balance  *string    `json:"balance"`
	Observed *time.Time `json:"observed_at"`
	Income   string     `json:"income"`
	Expense  string     `json:"expense"`
}

type WalletIncomeTrend struct {
	Period        string `json:"period"`
	Income        string `json:"income"`
	Expense       string `json:"expense"`
	ActiveMembers int    `json:"active_members"`
}

type WalletFinanceTrend struct {
	Period  string `json:"period"`
	Income  string `json:"income"`
	Expense string `json:"expense"`
	Tax     string `json:"tax"`
	Net     string `json:"net"`
}

func (s *AuthorizationService) WalletSummaries(ctx context.Context, ownerIDs []int64, from time.Time) ([]WalletSummary, error) {
	rows, e := store.ReadCharacterWalletSummaries(ctx, s.pool, ownerIDs, from)
	if e != nil {
		return nil, e
	}
	out := make([]WalletSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, WalletSummary{
			OwnerID: row.OwnerID, Balance: row.Balance, Observed: row.Observed,
			Income: row.Income, Expense: row.Expense,
		})
	}
	return out, nil
}

func (s *AuthorizationService) WalletCorporationSummary(ctx context.Context, ownerID int64, divisions []int, from time.Time) (WalletSummary, error) {
	row, e := store.ReadCorporationWalletSummary(ctx, s.pool, ownerID, divisions, from)
	if e != nil {
		return WalletSummary{}, e
	}
	return WalletSummary{
		OwnerID: row.OwnerID, Balance: row.Balance, Observed: row.Observed,
		Income: row.Income, Expense: row.Expense,
	}, nil
}

func (s *AuthorizationService) WalletCorporationFinanceTrend(ctx context.Context, ownerID int64, divisions []int, from, until time.Time) ([]WalletFinanceTrend, error) {
	rows, e := store.ReadCorporationWalletFinanceTrend(ctx, s.pool, ownerID, divisions, from, until)
	if e != nil {
		return nil, e
	}
	out := make([]WalletFinanceTrend, 0, len(rows))
	for _, row := range rows {
		out = append(out, WalletFinanceTrend{Period: row.Period, Income: row.Income, Expense: row.Expense, Tax: row.Tax, Net: row.Net})
	}
	return out, nil
}

func (s *AuthorizationService) WalletCorporationPersonalSummary(ctx context.Context, corporationID int64, from time.Time, characterIDs []int64) (WalletSummary, error) {
	row, e := store.ReadCorporationPersonalWalletSummary(ctx, s.pool, corporationID, from, characterIDs)
	if e != nil {
		return WalletSummary{}, e
	}
	return WalletSummary{
		OwnerID: row.OwnerID, Balance: row.Balance, Observed: row.Observed,
		Income: row.Income, Expense: row.Expense,
	}, nil
}

func (s *AuthorizationService) WalletCorporationPersonalIncomeTrend(ctx context.Context, corporationID int64, from, until time.Time, characterIDs []int64) ([]WalletIncomeTrend, error) {
	rows, e := store.ReadCorporationPersonalWalletIncomeTrend(ctx, s.pool, corporationID, from, until, characterIDs)
	if e != nil {
		return nil, e
	}
	out := make([]WalletIncomeTrend, 0, len(rows))
	for _, row := range rows {
		out = append(out, WalletIncomeTrend{Period: row.Period, Income: row.Income, Expense: row.Expense, ActiveMembers: row.ActiveMembers})
	}
	return out, nil
}

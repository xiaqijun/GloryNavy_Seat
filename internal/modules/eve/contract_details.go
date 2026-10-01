package eve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
)

type contractDetailArgs struct {
	OwnerKind   string `json:"owner_kind"`
	OwnerID     int64  `json:"owner_id"`
	ContractID  int64  `json:"contract_id"`
	Part        string `json:"part"`
	CharacterID int64  `json:"character_id"`
	Generation  int64  `json:"generation"`
}

func (contractDetailArgs) Kind() string { return "eve.contract-detail.v1" }

type contractDetailWorker struct {
	river.WorkerDefaults[contractDetailArgs]
	s *SyncService
}

func (w *contractDetailWorker) Work(parent context.Context, j *river.Job[contractDetailArgs]) error {
	return w.s.workContractDetail(parent, j.Args, j.Attempt)
}

func (s *SyncService) workContractDetail(parent context.Context, a contractDetailArgs, attempt int) error {
	ctx, cancel := context.WithTimeout(parent, 55*time.Second)
	defer cancel()
	if (a.OwnerKind != "character" && a.OwnerKind != "corporation") || (a.Part != "items" && a.Part != "bids") {
		return river.JobCancel(errors.New("invalid detail resource"))
	}
	q := store.New(s.pool)
	// Old queued jobs must retire before touching credentials or making ESI calls.
	if a.OwnerKind == "corporation" {
		_, err := q.GetContract(ctx, store.GetContractParams{OwnerKind: a.OwnerKind, OwnerID: a.OwnerID, ContractID: a.ContractID})
		if errors.Is(err, pgx.ErrNoRows) {
			return river.JobCancel(errors.New("corporation contract outside business scope"))
		}
		if err != nil {
			return err
		}
	}
	c, e := q.GetCredential(ctx, a.CharacterID)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	if c.GrantGeneration != a.Generation {
		return nil
	}
	previous, e := q.GetContractDetail(ctx, store.GetContractDetailParams{OwnerKind: a.OwnerKind, OwnerID: a.OwnerID, ContractID: a.ContractID, Part: a.Part})
	if errors.Is(e, pgx.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	if previous.Generation != a.Generation || previous.SourceCharacterID != a.CharacterID || previous.State == "blocked" {
		return nil
	}
	if previous.State == "ready" && (a.Part == "items" || previous.NextDueAt.Time.After(time.Now())) {
		return nil
	}
	d, e := q.ClaimContractDetail(ctx, store.ClaimContractDetailParams{OwnerKind: a.OwnerKind, OwnerID: a.OwnerID, ContractID: a.ContractID, Part: a.Part, Generation: a.Generation, SourceCharacterID: a.CharacterID})
	if errors.Is(e, pgx.ErrNoRows) {
		// A removed/reassigned detail is terminal for this old delivery.
		row, err := q.LockContractDetail(ctx, store.LockContractDetailParams{OwnerKind: a.OwnerKind, OwnerID: a.OwnerID, ContractID: a.ContractID, Part: a.Part})
		if errors.Is(err, pgx.ErrNoRows) || err == nil && (row.Generation != a.Generation || row.SourceCharacterID != a.CharacterID) {
			return nil
		}
		if err != nil {
			return err
		}
		return river.JobSnooze(30 * time.Second)
	}
	if e != nil {
		return e
	}
	var fetchErr error
	if d.NextDueAt.Time.After(time.Now()) {
		fetchErr = retryError{Until: d.NextDueAt.Time}
	}
	if s.validBinding != nil && fetchErr == nil {
		ok, err := s.validBinding(ctx, a.CharacterID, c.OwnerHash)
		if err != nil {
			fetchErr = err
		} else if !ok {
			fetchErr = syncFault{Reason: "identity_changed", Status: 0, Temporary: false}
		}
	}
	if a.OwnerKind == "corporation" && fetchErr == nil {
		fetchErr = contractSource(ctx, q, c, a.OwnerID)
	}
	contract, err := q.GetContract(ctx, store.GetContractParams{OwnerKind: a.OwnerKind, OwnerID: a.OwnerID, ContractID: a.ContractID})
	if err != nil {
		fetchErr = err
	}
	if err == nil && (contract.Status == "deleted" || contract.SourceCharacterID != a.CharacterID || contract.SourceGeneration != a.Generation) {
		fetchErr = syncFault{Reason: "resource_unavailable", Status: 0, Temporary: false}
	}
	scope := characterContractsScope
	if a.OwnerKind == "corporation" {
		scope = corporationContractsScope
	}
	paged := a.OwnerKind == "corporation" && a.Part == "bids"
	path := fmt.Sprintf("/%ss/%d/contracts/%d/%s/", a.OwnerKind, a.OwnerID, a.ContractID, a.Part)
	if paged {
		path += fmt.Sprintf("?page=%d", d.Page)
	}
	var raw json.RawMessage
	var response ESIResponse
	expires := time.Now().Add(5 * time.Minute)
	if fetchErr == nil {
		response, fetchErr = s.auth.esi.Request(ctx, ESIRequest{Method: "GET", Path: path, CharacterID: c.CharacterID, Generation: c.GrantGeneration, Scopes: []string{scope}, ExpectPages: paged}, &raw)
		expires = response.ExpiresAt
	}
	if fetchErr == nil {
		fetchErr = validateContractDetails(a.Part, raw)
	}
	pages := int32(1)
	if paged {
		pages = int32(response.Pages)
	}
	if fetchErr == nil && d.Page > 1 && d.Pages != pages {
		fetchErr = syncFault{Reason: "pagination_changed", Status: 200, Temporary: true}
	}

	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(context.Background())
	tq := store.New(tx)
	current, e := tq.GetCredentialForUpdate(ctx, a.CharacterID)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	if current.GrantGeneration != a.Generation {
		return nil
	}
	if current.State == "reauthorize" {
		fetchErr = ErrReauthorize
	}
	locked, e := tq.LockContractDetail(ctx, store.LockContractDetailParams{OwnerKind: a.OwnerKind, OwnerID: a.OwnerID, ContractID: a.ContractID, Part: a.Part})
	if errors.Is(e, pgx.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	if locked.Fence != d.Fence || locked.Generation != a.Generation || locked.SourceCharacterID != a.CharacterID || !locked.LeaseUntil.Time.After(time.Now()) {
		return nil
	}
	if a.OwnerKind == "corporation" {
		_, err := tq.GetContract(ctx, store.GetContractParams{OwnerKind: a.OwnerKind, OwnerID: a.OwnerID, ContractID: a.ContractID})
		if errors.Is(err, pgx.ErrNoRows) {
			return river.JobCancel(errors.New("corporation contract outside business scope"))
		}
		if err != nil {
			return err
		}
	}
	if fetchErr == nil && a.OwnerKind == "corporation" {
		fetchErr = contractSource(ctx, tq, current, a.OwnerID)
	}
	state, reason := "ready", ""
	next := expires
	page := int32(1)
	snooze := time.Duration(0)
	reauthorize := false
	if fetchErr == nil {
		if a.Part == "items" {
			if e = tq.ReplaceContractItems(ctx, store.ReplaceContractItemsParams{OwnerKind: a.OwnerKind, OwnerID: a.OwnerID, ContractID: a.ContractID}); e != nil {
				return e
			}
			e = tq.InsertContractItems(ctx, store.InsertContractItemsParams{OwnerKind: a.OwnerKind, OwnerID: a.OwnerID, ContractID: a.ContractID, Items: raw})
		} else {
			e = tq.InsertContractBids(ctx, store.InsertContractBidsParams{OwnerKind: a.OwnerKind, OwnerID: a.OwnerID, ContractID: a.ContractID, Bids: raw})
		}
		if e != nil {
			return e
		}
		if d.Page < pages {
			state = "pending"
			reason = "pagination"
			page = d.Page + 1
			next = time.Now().Add(time.Second)
			snooze = time.Second
		}
	} else {
		state = "pending"
		reason = "upstream_unavailable"
		page = d.Page
		pages = d.Pages
		next = time.Now().Add(time.Duration(1<<min(attempt, 5)) * 15 * time.Second)
		var retry retryError
		var fault syncFault
		switch {
		case errors.Is(fetchErr, ErrReauthorize):
			state = "blocked"
			reason = "reauthorize"
			reauthorize = true
		case errors.As(fetchErr, &retry):
			reason = "rate_limited"
			next = retry.Until
		case errors.As(fetchErr, &fault):
			reason = fault.Reason
			if !fault.Temporary {
				state = "blocked"
			}
		}
		if reason == "pagination_changed" {
			page = 1
			pages = 0
		}
		if reason == "authorization_pending" || reason == "shared_source" {
			next = time.Now().Add(time.Minute)
		}
		if state != "blocked" {
			if attempt >= 5 && reason != "rate_limited" && reason != "shared_source" && reason != "authorization_pending" {
				state = "failed"
				next = time.Now().Add(time.Hour)
			} else {
				snooze = max(time.Second, time.Until(next))
			}
		}
	}
	if e = tq.FinishContractDetail(ctx, store.FinishContractDetailParams{OwnerKind: a.OwnerKind, OwnerID: a.OwnerID, ContractID: a.ContractID, Part: a.Part, State: state, Reason: reason, NextDueAt: timestamp(next), Page: page, Pages: pages}); e != nil {
		return e
	}
	if e = tx.Commit(ctx); e != nil {
		return e
	}
	if reauthorize {
		if e = s.auth.Revoke(ctx, a.CharacterID); e != nil {
			return e
		}
	}
	if snooze > 0 {
		// Snooze does not consume River attempts; temporary HTTP failures do.
		if reason != "rate_limited" && reason != "pagination" && reason != "shared_source" && reason != "authorization_pending" {
			return errors.New("contract detail temporarily unavailable")
		}
		return river.JobSnooze(snooze)
	}
	return nil
}

func validateContractDetails(part string, raw []byte) error {
	var rows []map[string]json.RawMessage
	if json.Unmarshal(raw, &rows) != nil || rows == nil || len(rows) > 10000 {
		return syncFault{Reason: "invalid_response", Status: 200, Temporary: false}
	}
	ids := map[int64]bool{}
	for _, r := range rows {
		key := "record_id"
		if part == "bids" {
			key = "bid_id"
		}
		var id int64
		if json.Unmarshal(r[key], &id) != nil || id <= 0 || ids[id] {
			return syncFault{Reason: "invalid_response", Status: 200, Temporary: false}
		}
		ids[id] = true
		if part == "items" {
			var typ, quantity int64
			var included, singleton bool
			if json.Unmarshal(r["type_id"], &typ) != nil || typ <= 0 || json.Unmarshal(r["quantity"], &quantity) != nil || quantity < 0 || json.Unmarshal(r["is_included"], &included) != nil || json.Unmarshal(r["is_singleton"], &singleton) != nil {
				return syncFault{Reason: "invalid_response", Status: 200, Temporary: false}
			}
			if value, ok := r["raw_quantity"]; ok {
				var n int64
				if json.Unmarshal(value, &n) != nil {
					return syncFault{Reason: "invalid_response", Status: 200, Temporary: false}
				}
			}
		} else {
			var bidder int64
			var date time.Time
			var amount json.Number
			if json.Unmarshal(r["bidder_id"], &bidder) != nil || bidder <= 0 || json.Unmarshal(r["date_bid"], &date) != nil || date.IsZero() || json.Unmarshal(r["amount"], &amount) != nil || amount.String() == "" {
				return syncFault{Reason: "invalid_response", Status: 200, Temporary: false}
			}
		}
	}
	return nil
}

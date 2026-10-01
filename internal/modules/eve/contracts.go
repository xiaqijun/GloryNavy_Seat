package eve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
	platformjobs "glorynavy.local/seat/internal/platform/jobs"
)

const characterContractsScope = "esi-contracts.read_character_contracts.v1"
const corporationContractsScope = "esi-contracts.read_corporation_contracts.v1"

func isContracts(resource string) bool {
	return resource == "character_contracts" || resource == "corporation_contracts"
}

type contractsArgs syncArgs

func (contractsArgs) Kind() string { return "eve.contract-list.v1" }

type contractsWorker struct {
	river.WorkerDefaults[contractsArgs]
	s *SyncService
}

func (w *contractsWorker) Work(ctx context.Context, j *river.Job[contractsArgs]) error {
	t, e := store.New(w.s.pool).GetSyncTarget(ctx, j.Args.TargetID)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	if !isContracts(t.Resource) {
		return river.JobCancel(errors.New("invalid resource"))
	}
	return w.s.work(ctx, syncArgs(j.Args), j.ID, t.Resource)
}

type contractRecord struct {
	ID          int64     `json:"contract_id"`
	Issuer      int64     `json:"issuer_id"`
	Corporation int64     `json:"issuer_corporation_id"`
	Type        string    `json:"type"`
	Status      string    `json:"status"`
	Issued      time.Time `json:"date_issued"`
	Expired     time.Time `json:"date_expired"`
	raw         []byte
}
type contractPage struct {
	kind        string
	owner       int64
	page, pages int32
	expires     time.Time
	records     []contractRecord
}

func contractSource(ctx context.Context, q *store.Queries, c store.EveCredential, owner int64) error {
	fact, e := q.GetAuthorization(ctx, c.CharacterID)
	if e != nil {
		return e
	}
	if !fact.CorporationID.Valid || fact.CorporationID.Int64 != owner || !fact.ValidUntil.Time.After(time.Now()) || c.RolesNotBefore.Time.After(time.Now()) {
		return syncFault{Reason: "authorization_pending", Status: 0, Temporary: true}
	}
	id, e := q.ContractCorporationSource(ctx, owner)
	if errors.Is(e, pgx.ErrNoRows) {
		return syncFault{Reason: "authorization_pending", Status: 0, Temporary: true}
	}
	if e != nil {
		return e
	}
	if id != c.CharacterID {
		return syncFault{Reason: "shared_source", Status: 0, Temporary: true}
	}
	return nil
}
func (s *SyncService) collectContracts(ctx context.Context, t store.EveSyncTarget, c store.EveCredential) (syncResult, error) {
	p := &contractPage{kind: "character", owner: c.CharacterID, page: 1}
	scope := characterContractsScope
	q := store.New(s.pool)
	if t.Resource == "corporation_contracts" {
		scope = corporationContractsScope
		p.kind = "corporation"
		fact, e := q.GetAuthorization(ctx, c.CharacterID)
		if e != nil {
			return syncResult{}, e
		}
		p.owner = fact.CorporationID.Int64
		if !slices.Contains(c.Scopes, scope) {
			return syncResult{}, syncFault{Reason: "missing_scope", Status: 0, Temporary: false}
		}
		if e = contractSource(ctx, q, c, p.owner); e != nil {
			return syncResult{}, e
		}
	}
	cursor, e := q.GetContractCursor(ctx, t.ID)
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return syncResult{}, e
	}
	if e == nil && cursor.Generation == c.GrantGeneration && cursor.OwnerID == p.owner {
		p.page = cursor.Page
		p.pages = cursor.Pages
		p.expires = cursor.ExpiresAt.Time
	}
	var raw []json.RawMessage
	path := fmt.Sprintf("/%ss/%d/contracts/?page=%d", p.kind, p.owner, p.page)
	response, e := s.auth.esi.Request(ctx, ESIRequest{Method: "GET", Path: path, CharacterID: c.CharacterID, Generation: c.GrantGeneration, Scopes: []string{scope}, ExpectPages: true}, &raw)
	if e != nil {
		return syncResult{}, e
	}
	if raw == nil || len(raw) > 1000 {
		return syncResult{}, syncFault{Reason: "invalid_response", Status: 200, Temporary: false}
	}
	pages := int32(response.Pages)
	if p.page > 1 && p.pages != pages {
		return syncResult{}, syncFault{Reason: "pagination_changed", Status: 200, Temporary: true}
	}
	p.pages = pages
	if p.expires.IsZero() || response.ExpiresAt.Before(p.expires) {
		p.expires = response.ExpiresAt
	}
	seen := map[int64]bool{}
	for _, data := range raw {
		var r contractRecord
		if json.Unmarshal(data, &r) != nil || r.ID <= 0 || r.Issuer <= 0 || r.Corporation <= 0 || r.Issued.IsZero() || r.Expired.IsZero() || r.Status == "" || !slices.Contains([]string{"unknown", "item_exchange", "auction", "courier", "loan"}, r.Type) || seen[r.ID] {
			return syncResult{}, syncFault{Reason: "invalid_response", Status: 200, Temporary: false}
		}
		seen[r.ID] = true
		r.raw = data
		p.records = append(p.records, r)
	}
	return syncResult{contracts: p, next: maxTime(p.expires, time.Now().Add(time.Second)), content: response.ContentUpdatedAt}, nil
}
func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
func (s *SyncService) saveContractPage(ctx context.Context, tx pgx.Tx, t store.EveSyncTarget, c store.EveCredential, p *contractPage) error {
	q := store.New(tx)
	for _, r := range p.records {
		inScope, e := q.UpsertContract(ctx, store.UpsertContractParams{OwnerKind: p.kind, OwnerID: p.owner, ContractID: r.ID, SourceCharacterID: c.CharacterID, SourceGeneration: c.GrantGeneration, ContractType: r.Type, Status: r.Status, Payload: r.raw})
		if e != nil {
			return e
		}
		if !inScope {
			if e = q.ExcludeContractDetails(ctx, store.ExcludeContractDetailsParams{OwnerKind: p.kind, OwnerID: p.owner, ContractID: r.ID}); e != nil {
				return e
			}
			continue
		}
		parts := []string{}
		if r.Status != "deleted" {
			if r.Type == "auction" {
				parts = append(parts, "bids")
			}
			if r.Type != "courier" && r.Type != "unknown" {
				parts = append(parts, "items")
			}
		}
		for _, part := range parts {
			d, e := q.PrepareContractDetail(ctx, store.PrepareContractDetailParams{OwnerKind: p.kind, OwnerID: p.owner, ContractID: r.ID, Part: part, TargetID: t.ID, SourceCharacterID: c.CharacterID, Generation: c.GrantGeneration})
			if e != nil {
				return e
			}
			if d.State == "blocked" || (d.Part == "items" && d.LastSuccessAt.Valid) || d.NextDueAt.Time.After(time.Now()) {
				continue
			}
			args := contractDetailArgs{p.kind, p.owner, r.ID, part, c.CharacterID, c.GrantGeneration}
			if _, e = s.queue.InsertTx(ctx, tx, args, &river.InsertOpts{Queue: "eve_contracts", Priority: 3, UniqueOpts: platformjobs.ActiveUnique(), MaxAttempts: 5}); e != nil {
				return e
			}
			if e = q.QueueContractDetail(ctx, store.QueueContractDetailParams{OwnerKind: p.kind, OwnerID: p.owner, ContractID: r.ID, Part: part}); e != nil {
				return e
			}
		}
	}
	if p.page < p.pages {
		return q.SaveContractCursor(ctx, store.SaveContractCursorParams{TargetID: t.ID, Generation: c.GrantGeneration, OwnerID: p.owner, Page: p.page + 1, Pages: p.pages, ExpiresAt: timestamp(p.expires)})
	}
	return q.DeleteContractCursor(ctx, t.ID)
}

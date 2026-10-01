package eve

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
)

const structureReadScope = "esi-universe.read_structures.v1"

type structureKey struct{ character, generation, location int64 }

// Only the personal owner or the corporation's current contract sync source is
// eligible. Never try unrelated members' tokens to work around a structure ACL.
func (h *ContractHTTP) locationSource(ctx context.Context, owner ContractOwner) (store.EveCredential, error) {
	if h.BindingOwner == nil {
		return store.EveCredential{}, pgx.ErrNoRows
	}
	q := store.New(h.pool)
	id := owner.ID
	if owner.Kind == "corporation" {
		var err error
		id, err = q.ContractCorporationSource(ctx, owner.ID)
		if err != nil {
			return store.EveCredential{}, err
		}
	} else if owner.Kind != "character" {
		return store.EveCredential{}, pgx.ErrNoRows
	}
	c, err := q.GetCredential(ctx, id)
	if err != nil {
		return c, err
	}
	if c.State == "reauthorize" || !slices.Contains(c.Scopes, structureReadScope) {
		return c, ErrReauthorize
	}
	bound, err := h.BindingOwner(ctx, id)
	if err != nil {
		return c, err
	}
	if subtle.ConstantTimeCompare(bound, c.OwnerHash) != 1 {
		return c, ErrReauthorize
	}
	if owner.Kind == "corporation" {
		err = contractSource(ctx, q, c, owner.ID)
	}
	return c, err
}

// This is a bounded retry cooldown for rejected lookups, not a name cache or an
// authorization grant. Successful private responses stay in the shared encrypted
// ESI cache, isolated by character and grant generation, with official freshness.
func (h *ContractHTTP) locationCooling(key structureKey, failed bool) bool {
	h.locationMu.Lock()
	defer h.locationMu.Unlock()
	now := time.Now()
	if !failed {
		return h.locationRetry[key].After(now)
	}
	if h.locationRetry == nil {
		h.locationRetry = make(map[structureKey]time.Time)
	}
	for k, until := range h.locationRetry {
		if !until.After(now) {
			delete(h.locationRetry, k)
		}
	}
	if len(h.locationRetry) < 512 {
		h.locationRetry[key] = now.Add(time.Minute)
	}
	return false
}

func (h *ContractHTTP) contractLocations(ctx context.Context, owner ContractOwner, refs []*contractEntity) {
	public := []*contractEntity{}
	private := map[int64][]*contractEntity{}
	for _, v := range refs {
		id, _ := strconv.ParseInt(v.ID, 10, 64)
		if id <= 0 {
			continue
		}
		if v.Category == "structure" {
			private[id] = append(private[id], v)
		} else {
			public = append(public, v)
		}
	}
	var wg sync.WaitGroup
	wg.Go(func() {
		h.names(ctx, public...)
		// A removed/invalid public ID can reject a batch. Resolve remaining NPC
		// stations independently; unrelated contracts still keep their names.
		pending := map[int64][]*contractEntity{}
		for _, v := range public {
			id, _ := strconv.ParseInt(v.ID, 10, 64)
			if v.Name == "" && id >= 60000000 && id < 64000000 {
				pending[id] = append(pending[id], v)
			}
		}
		h.resolveLocations(ctx, store.EveCredential{}, pending)
	})
	if len(private) > 0 && h.esi != nil {
		if source, err := h.locationSource(ctx, owner); err == nil {
			wg.Go(func() {
				// Reserve part of the page budget for the final ownership check even
				// when one location is slow or a page contains many unique structures.
				fetchCtx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
				h.resolveLocations(fetchCtx, source, private)
				cancel()
				// Discard results if binding, grant or corporation source changed in flight.
				current, err := h.locationSource(ctx, owner)
				if err != nil || current.CharacterID != source.CharacterID || current.GrantGeneration != source.GrantGeneration || subtle.ConstantTimeCompare(current.OwnerHash, source.OwnerHash) != 1 {
					for _, refs := range private {
						for _, v := range refs {
							v.Name, v.NameLanguage = "", ""
						}
					}
				}
			})
		}
	}
	wg.Wait()
}

func (h *ContractHTTP) resolveLocations(ctx context.Context, source store.EveCredential, locations map[int64][]*contractEntity) {
	if h.esi == nil {
		return
	}
	// Three workers bound fan-out. All work shares decorate's three-second budget.
	jobs := make(chan int64, len(locations))
	for id := range locations {
		jobs <- id
	}
	close(jobs)
	var wg sync.WaitGroup
	for range min(3, len(locations)) {
		wg.Go(func() {
			for id := range jobs {
				if ctx.Err() != nil {
					return
				}
				key := structureKey{source.CharacterID, source.GrantGeneration, id}
				if h.locationCooling(key, false) {
					continue
				}
				req := ESIRequest{Method: "GET", Path: fmt.Sprintf("/universe/stations/%d/", id)}
				if source.CharacterID > 0 {
					req.Path = fmt.Sprintf("/universe/structures/%d/", id)
					req.CharacterID, req.Generation, req.Scopes = source.CharacterID, source.GrantGeneration, []string{structureReadScope}
				}
				var result struct {
					Name string `json:"name"`
				}
				err := h.decorationRequest(ctx, req, &result)
				if err != nil {
					var fault syncFault
					if errors.As(err, &fault) && (fault.Status == 403 || fault.Status == 404) {
						h.locationCooling(key, true)
					}
					continue
				}
				if result.Name == "" {
					continue
				}
				for _, v := range locations[id] {
					v.Name, v.NameLanguage = result.Name, "en"
				}
				if source.CharacterID == 0 {
					_ = store.New(h.pool).SaveEntityName(ctx, store.SaveEntityNameParams{EntityID: id, Name: result.Name, Category: "station", Language: "en"})
				}
			}
		})
	}
	wg.Wait()
}

// Honor short pacing deferrals inside the existing decoration budget. Long quota
// waits and upstream outages return immediately instead of holding a page open.
func (h *ContractHTTP) decorationRequest(ctx context.Context, req ESIRequest, out any) error {
	for attempt := 0; ; attempt++ {
		_, err := h.esi.Request(ctx, req, out)
		var retry retryError
		if attempt >= 3 || !errors.As(err, &retry) {
			return err
		}
		delay := time.Until(retry.Until)
		if delay <= 0 || delay > 250*time.Millisecond {
			return err
		}
		timer := time.NewTimer(delay + time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

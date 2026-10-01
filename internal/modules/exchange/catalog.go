package exchange

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/exchange/internal/store"
)

type PhysicalFitting struct {
	ID            int64           `json:"fitting_id,string"`
	Quantity      int64           `json:"quantity"`
	Name          string          `json:"name,omitempty"`
	ShipTypeID    int64           `json:"ship_type_id,string,omitempty"`
	CorporationID int64           `json:"corporation_id,string,omitempty"`
	Version       int64           `json:"version,string,omitempty"`
	Fit           json.RawMessage `json:"fit,omitempty"`
}
type PhysicalItem struct {
	ID       int64  `json:"type_id,string"`
	Quantity int64  `json:"quantity"`
	Name     string `json:"name,omitempty"`
}
type PhysicalReward struct {
	ISKMinor int64             `json:"isk_minor,omitempty"`
	Fittings []PhysicalFitting `json:"fittings"`
	Items    []PhysicalItem    `json:"items"`
}
type CatalogEntry struct {
	ID       int64          `json:"id,string"`
	Version  int64          `json:"version,string"`
	Name     string         `json:"name"`
	Content  PhysicalReward `json:"content"`
	Archived bool           `json:"archived"`
}
type CatalogEdit struct {
	CatalogEntry
	RequestKey string `json:"request_key"`
}
type CatalogList struct {
	Items      []CatalogEntry `json:"items"`
	NextCursor string         `json:"next_cursor"`
}

func physicalValid(r PhysicalReward) bool {
	if len(r.Fittings) > 10 || len(r.Items) > 30 || r.ISKMinor < 0 || r.ISKMinor > 100000000000000 || (r.ISKMinor > 0 && r.ISKMinor < 100) || (len(r.Fittings)+len(r.Items) == 0 && r.ISKMinor == 0) {
		return false
	}
	seen := map[int64]bool{}
	for _, f := range r.Fittings {
		if f.ID < 1 || f.Quantity < 1 || f.Quantity > 100 || seen[f.ID] {
			return false
		}
		seen[f.ID] = true
	}
	seen = map[int64]bool{}
	for _, i := range r.Items {
		if i.ID < 1 || i.Quantity < 1 || i.Quantity > 1000000 || seen[i.ID] {
			return false
		}
		seen[i.ID] = true
	}
	return true
}
func catalogEntry(r store.Catalog) (CatalogEntry, error) {
	out := CatalogEntry{ID: r.ID, Version: r.Version, Name: r.Name, Archived: r.Archived}
	err := json.Unmarshal(r.Content, &out.Content)
	return out, err
}

// Library definitions are site-admin curated. Only published offers and owned order snapshots are member-visible.
func (s *Service) Catalog(ctx context.Context, user string, after int64) (CatalogList, error) {
	out := CatalogList{Items: []CatalogEntry{}}
	if err := s.shopAdmin(ctx, user); err != nil {
		return out, err
	}
	rows, err := store.New(s.Pool).CatalogList(ctx, after)
	if err != nil {
		return out, err
	}
	if len(rows) > 50 {
		rows = rows[:50]
		out.NextCursor = strconv.FormatInt(rows[49].ID, 10)
	}
	for _, r := range rows {
		entry, e := catalogEntry(r)
		if e != nil {
			return out, e
		}
		if e = s.presentPhysical(ctx, &entry.Content); e != nil {
			return out, e
		}
		if entry.Name == "" && len(entry.Content.Items) == 1 {
			entry.Name = entry.Content.Items[0].Name
		}
		out.Items = append(out.Items, entry)
	}
	return out, nil
}
func (s *Service) presentPhysical(ctx context.Context, c *PhysicalReward) error {
	if len(c.Items) == 0 {
		return nil
	}
	if s.Names == nil {
		return ErrUnavailable
	}
	ids := []int64{}
	for _, i := range c.Items {
		ids = append(ids, i.ID)
	}
	names, err := s.Names.TypeNames(ctx, ids)
	if err != nil {
		return err
	}
	for i := range c.Items {
		if n := names[c.Items[i].ID].Name; n != "" {
			c.Items[i].Name = n
		}
	}
	return nil
}
func (s *Service) SaveCatalog(ctx context.Context, user string, c CatalogEdit) (int64, error) {
	if err := s.shopAdmin(ctx, user); err != nil {
		return 0, err
	}
	actor, err := uuid(user)
	key, e := uuid(c.RequestKey)
	c.Name = strings.TrimSpace(c.Name)
	if err != nil || e != nil || c.ID < 0 || c.ID > 0 && c.Version < 1 || len([]rune(c.Name)) < 1 || len([]rune(c.Name)) > 100 {
		return 0, ErrInvalid
	}
	fp := shopFingerprint("catalog", c)
	// Fast replay happens before resolving possibly removed source fittings.
	q := store.New(s.Pool)
	if audit, e := q.FindShopAudit(ctx, store.FindShopAuditParams{ActorID: actor, RequestKey: key}); e == nil {
		if audit.Kind != "catalog" || audit.Fingerprint != fp {
			return 0, ErrConflict
		}
		return audit.TargetID, nil
	} else if e != pgx.ErrNoRows {
		return 0, e
	}
	content := PhysicalReward{ISKMinor: c.Content.ISKMinor, Fittings: []PhysicalFitting{}, Items: []PhysicalItem{}}
	var cover int64
	if !c.Archived {
		if !physicalValid(c.Content) {
			return 0, ErrInvalid
		}
		for _, f := range c.Content.Fittings {
			if s.RewardFitting == nil {
				return 0, ErrUnavailable
			}
			resolved, e := s.RewardFitting(ctx, user, f.ID)
			if e != nil {
				return 0, e
			}
			if resolved.ID != f.ID || resolved.ShipTypeID <= 0 || resolved.CorporationID <= 0 || len(resolved.Fit) == 0 {
				return 0, ErrInvalid
			}
			resolved.Quantity = f.Quantity
			content.Fittings = append(content.Fittings, resolved)
			if cover == 0 {
				cover = resolved.ShipTypeID
			}
		}
		if s.Names == nil {
			return 0, ErrUnavailable
		}
		ids := []int64{}
		for _, i := range c.Content.Items {
			ids = append(ids, i.ID)
		}
		names, e := s.Names.TypeNames(ctx, ids)
		if e != nil {
			return 0, e
		}
		for _, i := range c.Content.Items {
			n := names[i.ID]
			if n.Source != "sde" || n.Name == "" {
				return 0, ErrInvalid
			}
			content.Items = append(content.Items, PhysicalItem{ID: i.ID, Quantity: i.Quantity, Name: n.Name})
			if cover == 0 {
				cover = i.ID
			}
		}
	} else if c.ID == 0 {
		return 0, ErrInvalid
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(context.Background())
	q = store.New(tx)
	if _, err = q.LockShopSettings(ctx); err != nil {
		return 0, err
	}
	if audit, e := q.FindShopAudit(ctx, store.FindShopAuditParams{ActorID: actor, RequestKey: key}); e == nil {
		if audit.Kind != "catalog" || audit.Fingerprint != fp {
			return 0, ErrConflict
		}
		return audit.TargetID, nil
	} else if e != pgx.ErrNoRows {
		return 0, e
	}
	var previous any
	if c.ID > 0 {
		old, e := q.LockReward(ctx, c.ID)
		if e != nil {
			return 0, e
		}
		if old.CatalogVersion != c.Version {
			return 0, ErrConflict
		}
		previous = old
		if c.Archived {
			if e = json.Unmarshal(old.Content, &content); e != nil {
				return 0, e
			}
			cover = old.TypeID
			c.Name = old.Name
		}
	}
	raw, _ := json.Marshal(content)
	row := store.Catalog{ID: c.ID, Name: c.Name, Content: raw, Archived: c.Archived}
	id, err := q.CatalogSave(ctx, row, cover)
	if err != nil {
		return 0, err
	}
	if err = q.ResetRewardPricing(ctx, id); err != nil {
		return 0, err
	}
	payload, _ := json.Marshal(map[string]any{"before": previous, "after": row})
	if err = q.ShopAudit(ctx, store.ShopAuditParams{ActorID: actor, RequestKey: key, Kind: "catalog", TargetID: id, Fingerprint: fp, Payload: payload}); err != nil {
		return 0, err
	}
	return id, tx.Commit(ctx)
}

// A welfare configuration pins the library version. Stored case evidence remains usable after archival.
func (s *Service) WelfareReward(ctx context.Context, user string, corp, id, version int64) (PhysicalReward, error) {
	if err := s.shopAdmin(ctx, user); err != nil {
		return PhysicalReward{}, err
	}
	row, err := store.New(s.Pool).CatalogRead(ctx, id)
	if err != nil {
		return PhysicalReward{}, err
	}
	entry, err := catalogEntry(row)
	if err != nil {
		return PhysicalReward{}, err
	}
	if entry.Archived || entry.Version != version || !physicalValid(entry.Content) {
		return PhysicalReward{}, ErrConflict
	}
	for _, f := range entry.Content.Fittings {
		if f.CorporationID != corp {
			return PhysicalReward{}, pgx.ErrNoRows
		}
	}
	return entry.Content, nil
}
func (h Handler) catalog(w http.ResponseWriter, r *http.Request) {
	after, err := cursorQuery(r, "after")
	if err != nil {
		respond(w, r, nil, err)
		return
	}
	out, err := h.Service.Catalog(r.Context(), h.User(r), after)
	respond(w, r, out, err)
}
func (h Handler) saveCatalog(w http.ResponseWriter, r *http.Request) {
	var c CatalogEdit
	if !readBody(w, r, &c) {
		return
	}
	id, err := h.Service.SaveCatalog(r.Context(), h.User(r), c)
	respond(w, r, map[string]string{"id": strconv.FormatInt(id, 10)}, err)
}

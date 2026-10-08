package eve

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
)

type DeliveryItem struct {
	RecordID    int64  `json:"record_id,string"`
	Singleton   bool   `json:"singleton"`
	RawQuantity *int64 `json:"raw_quantity,string,omitempty"`
	TypeID      int64  `json:"type_id,string"`
	Quantity    int64  `json:"quantity,string"`
	Included    bool   `json:"included"`
	Name        string `json:"name"`
}
type DeliveryContract struct {
	IssuerName          string          `json:"issuer_name"`
	ID                  int64           `json:"id,string"`
	OwnerKind           string          `json:"owner_kind"`
	OwnerID             int64           `json:"owner_id,string"`
	Type                string          `json:"type"`
	Status              string          `json:"status"`
	CheckedAt           time.Time       `json:"checked_at"`
	Payload             json.RawMessage `json:"-"`
	Title               string          `json:"title"`
	IssuerID            int64           `json:"issuer_id,string"`
	IssuerCorporationID int64           `json:"issuer_corporation_id,string"`
	ForCorporation      bool            `json:"for_corporation"`
	AssigneeID          int64           `json:"assignee_id,string"`
	AcceptorID          int64           `json:"acceptor_id,string"`
	Price               string          `json:"price"`
	Reward              string          `json:"reward"`
	Issued              time.Time       `json:"issued"`
	Completed           string          `json:"completed"`
	Expired             time.Time       `json:"expired"`
	Accepted            string          `json:"accepted"`
	ItemsReady          bool            `json:"items_ready"`
	Items               []DeliveryItem  `json:"items"`
	ContentToken        string          `json:"content_token"`
}

// PurchaseContract reads a single personal contract under the existing object guard.
// It does not broaden a welfare reviewer's private-contract access.
func (h *ContractHTTP) PurchaseContract(ctx context.Context, actor string, character, id int64) (DeliveryContract, error) {
	return h.ReadContract(ctx, actor, "character", character, id)
}

// ReadContract returns a complete local snapshot after checking current object access.
func (h *ContractHTTP) ReadContract(ctx context.Context, actor, kind string, owner, id int64) (DeliveryContract, error) {
	if (kind != "character" && kind != "corporation") || owner <= 0 || id <= 0 || h.LookupOwner == nil {
		return DeliveryContract{}, pgx.ErrNoRows
	}
	if _, err := h.LookupOwner(ctx, actor, kind, owner); err != nil {
		return DeliveryContract{}, err
	}
	rows, err := store.DeliveryContracts(ctx, h.pool, kind, owner, 0, 0, id, time.Time{}, false)
	if err != nil {
		return DeliveryContract{}, err
	}
	if len(rows) != 1 {
		return DeliveryContract{}, pgx.ErrNoRows
	}
	c, err := decodeDelivery(rows[0])
	if err != nil {
		return c, err
	}
	out := []DeliveryContract{c}
	err = h.nameDelivery(ctx, out)
	return out[0], err
}

func decodeDelivery(b json.RawMessage) (DeliveryContract, error) {
	var raw struct {
		DeliveryContract
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return DeliveryContract{}, err
	}
	c := raw.DeliveryContract
	var p struct {
		Title     string
		Issuer    int64 `json:"issuer_id"`
		Corp      int64 `json:"issuer_corporation_id"`
		ForCorp   bool  `json:"for_corporation"`
		Assignee  int64 `json:"assignee_id"`
		Acceptor  int64 `json:"acceptor_id"`
		Price     json.Number
		Reward    json.Number
		Issued    time.Time `json:"date_issued"`
		Completed string    `json:"date_completed"`
		Expired   time.Time `json:"date_expired"`
		Accepted  string    `json:"date_accepted"`
	}
	if err := json.Unmarshal(raw.Payload, &p); err != nil {
		return c, err
	}
	c.Title = p.Title
	c.IssuerID = p.Issuer
	c.IssuerCorporationID = p.Corp
	c.ForCorporation = p.ForCorp
	c.AssigneeID = p.Assignee
	c.AcceptorID = p.Acceptor
	c.Price = p.Price.String()
	c.Reward = p.Reward.String()
	c.Issued = p.Issued
	c.Completed = p.Completed
	c.Expired = p.Expired
	c.Accepted = p.Accepted
	// Exclude mutable status/timestamps and display names; bind the exact exchange terms.
	var terms map[string]json.RawMessage
	if err := json.Unmarshal(raw.Payload, &terms); err != nil {
		return c, err
	}
	for _, key := range []string{"status", "acceptor_id", "date_accepted", "date_completed"} {
		delete(terms, key)
	}
	stable, _ := json.Marshal([]any{c.ID, c.Type, terms, c.Items})
	sum := sha256.Sum256(stable)
	c.ContentToken = hex.EncodeToString(sum[:])
	return c, nil
}
func (h *ContractHTTP) DeliveryContracts(ctx context.Context, actor string, corp, recipient, id int64, since time.Time) ([]DeliveryContract, error) {
	owners, err := h.Owners(ctx, actor)
	if err != nil {
		return nil, err
	}
	// Site administrators may additionally read the recipient's private contracts.
	if o, e := h.LookupOwner(ctx, actor, "character", recipient); e == nil {
		owners = append(owners, o)
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return nil, e
	}
	unique := map[string]bool{}
	byID := map[int64]DeliveryContract{}
	for _, o := range owners {
		if o.Kind == "corporation" && o.ID != corp {
			continue
		}
		key := o.Kind + strconv.FormatInt(o.ID, 10)
		if unique[key] {
			continue
		}
		unique[key] = true
		if _, err = h.LookupOwner(ctx, actor, o.Kind, o.ID); errors.Is(err, pgx.ErrNoRows) {
			continue
		} else if err != nil {
			return nil, err
		}
		rows, e := store.DeliveryContracts(ctx, h.pool, o.Kind, o.ID, recipient, corp, id, since, false)
		if e != nil {
			return nil, e
		}
		for _, b := range rows {
			c, e := decodeDelivery(b)
			if e != nil {
				return nil, e
			}
			old, ok := byID[c.ID]
			if !ok || c.CheckedAt.After(old.CheckedAt) {
				byID[c.ID] = c
			}
		}
	}
	out := []DeliveryContract{}
	for _, c := range byID {
		out = append(out, c)
	}
	slices.SortFunc(out, func(a, b DeliveryContract) int {
		if a.ID > b.ID {
			return -1
		}
		if a.ID < b.ID {
			return 1
		}
		return 0
	})
	if len(out) > 50 {
		out = out[:50]
	}
	if err = h.nameDelivery(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

// LoanCashContracts lists recent finished contract snapshots owned by the
// current account. The loan module still performs its own exact cash-party
// and amount validation before claiming any contract.
func (h *ContractHTTP) LoanCashContracts(ctx context.Context, actor string, since time.Time) ([]DeliveryContract, error) {
	owners, err := h.Owners(ctx, actor)
	if err != nil {
		return nil, err
	}
	unique := map[string]bool{}
	out := []DeliveryContract{}
	for _, owner := range owners {
		key := owner.Kind + strconv.FormatInt(owner.ID, 10)
		if unique[key] {
			continue
		}
		unique[key] = true
		rows, e := store.LoanCashContracts(ctx, h.pool, owner.Kind, owner.ID, since)
		if e != nil {
			return nil, e
		}
		for _, raw := range rows {
			contract, e := decodeDelivery(raw)
			if e != nil {
				return nil, e
			}
			out = append(out, contract)
		}
	}
	slices.SortFunc(out, func(a, b DeliveryContract) int {
		if a.ID > b.ID {
			return -1
		}
		if a.ID < b.ID {
			return 1
		}
		return 0
	})
	if len(out) > 100 {
		out = out[:100]
	}
	return out, nil
}

func (h *ContractHTTP) nameDelivery(ctx context.Context, out []DeliveryContract) error {
	ids := []int64{}
	for _, c := range out {
		if c.ForCorporation {
			ids = append(ids, c.IssuerCorporationID)
		} else {
			ids = append(ids, c.IssuerID)
		}
	}
	names, e := store.New(h.pool).ReadEntityNames(ctx, store.ReadEntityNamesParams{Ids: ids, Language: "zh"})
	if e != nil {
		return e
	}
	for x := range out {
		c := &out[x]
		id := c.IssuerID
		if c.ForCorporation {
			id = c.IssuerCorporationID
		}
		for _, n := range names {
			if n.EntityID == id {
				c.IssuerName = n.Name
				break
			}
		}
	}
	if h.StaticData != nil {
		ids := []int64{}
		for _, c := range out {
			for _, i := range c.Items {
				ids = append(ids, i.TypeID)
			}
		}
		names, e := h.StaticData.TypeNames(ctx, ids)
		if e != nil {
			return e
		}
		for x := range out {
			for y := range out[x].Items {
				i := &out[x].Items[y]
				i.Name = names[i.TypeID].Name
				if i.Name == "" {
					i.Name = strconv.FormatInt(i.TypeID, 10)
				}
			}
		}
	}
	return nil
}
func (h *ContractHTTP) DeliveryContractTx(ctx context.Context, tx pgx.Tx, actor, kind string, owner, id int64) (DeliveryContract, error) {
	if _, err := h.LookupOwner(ctx, actor, kind, owner); err != nil {
		return DeliveryContract{}, err
	}
	rows, err := store.DeliveryContracts(ctx, tx, kind, owner, 0, 0, id, time.Time{}, true)
	if err != nil {
		return DeliveryContract{}, err
	}
	if len(rows) != 1 {
		return DeliveryContract{}, pgx.ErrNoRows
	}
	c, e := decodeDelivery(rows[0])
	if e != nil {
		return c, e
	}
	values := []DeliveryContract{c}
	e = h.nameDelivery(ctx, values)
	return values[0], e
}

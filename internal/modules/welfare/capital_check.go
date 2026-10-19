package welfare

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
)

type CapitalCheck struct {
	State         string                  `json:"state"`
	Reason        string                  `json:"reason,omitempty"`
	PolicyVersion int64                   `json:"policy_version,string"`
	RateBPS       int64                   `json:"rate_bps"`
	CaseID        int64                   `json:"case_id,string,omitempty"`
	Purchase      *CapitalPurchasePreview `json:"purchase,omitempty"`
}

type CapitalPurchasePreview struct {
	ContractID    int64  `json:"contract_id,string"`
	CharacterID   int64  `json:"character_id,string"`
	CharacterName string `json:"character_name"`
	ShipTypeID    int64  `json:"ship_type_id,string"`
	ShipName      string `json:"ship_name"`
	PriceMinor    int64  `json:"price_minor"`
	AwardMinor    int64  `json:"award_minor"`
	CompletedAt   string `json:"completed_at"`
}

// CapitalCheck is an advisory, read-only preview. Execute remains authoritative
// and rechecks binding, policy, purchase fingerprint and claims under its locks.
func (s *Service) CapitalCheck(ctx context.Context, actor string, corp int64, kind string, contract int64) (CapitalCheck, error) {
	out := CapitalCheck{State: "closed"}
	if !validUUID(actor) || !isSuper(kind) || corp <= 0 || contract < 0 {
		return out, ErrInvalid
	}
	if err := s.allowed(ctx, actor, corp, false); err != nil {
		return out, err
	}
	chars, err := s.Characters(ctx, actor)
	if err != nil {
		return out, err
	}
	own := []Character{}
	for _, ch := range chars {
		if ch.AccountID == actor && ch.CorporationID == corp {
			own = append(own, ch)
		}
	}
	if len(own) == 0 {
		out.State = "unbound"
		return out, nil
	}
	p, cfg, err := s.rule(ctx, corp, kind)
	if errors.Is(err, ErrRule) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	out.PolicyVersion = p.Version
	out.RateBPS = 1000
	if kind == "titan" {
		out.RateBPS = 500
	}
	if cfg.SubsidyRateBPS != nil {
		out.RateBPS = *cfg.SubsidyRateBPS
	}
	if !cfg.Enabled {
		return out, nil
	}
	prior, err := store.CapitalApplication(ctx, s.Pool, actor, kind)
	if err == nil {
		out.State, out.CaseID = "pending", prior.ID
		if prior.State == "completed" {
			out.State = "claimed"
		}
		return out, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	profile, err := store.Profile(ctx, s.Pool, actor)
	if err != nil {
		return out, err
	}
	claimed, err := store.Claimed(ctx, s.Pool, "once:"+actor+":"+kind)
	if err != nil {
		return out, err
	}
	if profile.History[kind] == "used" || claimed {
		out.State = "claimed"
		return out, nil
	}
	out.State = "contract_required"
	if contract == 0 {
		return out, nil
	}
	ch, purchase, hull, price, err := s.findCapitalPurchase(ctx, actor, corp, kind, contract, own)
	if errors.Is(err, ErrCapitalPurchase) {
		out.State = "contract_invalid"
		var issue capitalPurchaseIssue
		if errors.As(err, &issue) {
			out.Reason = string(issue)
		}
		return out, nil
	}
	if err != nil {
		return out, err
	}
	occupied, err := store.CapitalOccupied(ctx, s.Pool, actor, kind, contract, 0)
	if err != nil {
		return out, err
	}
	if occupied {
		out.State = "contract_used"
		return out, nil
	}
	amount, err := capitalAward(kind, price, cfg)
	if err != nil {
		return out, err
	}
	name := ""
	for _, item := range purchase.Items {
		if item.TypeID == hull {
			name = item.Name
			break
		}
	}
	out.State = "ready"
	out.Purchase = &CapitalPurchasePreview{ContractID: contract, CharacterID: ch.ID, CharacterName: ch.Name, ShipTypeID: hull, ShipName: name, PriceMinor: price, AwardMinor: amount, CompletedAt: purchase.Completed}
	return out, nil
}

func (h Handler) capitalCheck(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	contract := int64(0)
	if value := q.Get("contract_id"); value != "" {
		var err error
		contract, err = strconv.ParseInt(value, 10, 64)
		if err != nil || contract <= 0 {
			respond(w, r, nil, ErrInvalid)
			return
		}
	}
	result, err := h.Service.CapitalCheck(r.Context(), h.User(r), number(q.Get("corporation_id")), q.Get("kind"), contract)
	respond(w, r, result, err)
}

package welfare

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
	"time"
)

type GrowthStatus struct {
	State       string     `json:"state"`
	RemainingSP *int64     `json:"remaining_sp"`
	ObservedAt  *time.Time `json:"observed_at"`
}

func (s *Service) GrowthStatus(ctx context.Context, actor string, corp, char int64, kind string) (GrowthStatus, error) {
	out := GrowthStatus{State: "unknown"}
	if !isGrowth(kind) || !validKind(kind) {
		return out, ErrInvalid
	}
	if err := s.allowed(ctx, actor, corp, false); err != nil {
		return out, err
	}
	chars, err := s.Characters(ctx, actor)
	if err != nil {
		return out, err
	}
	found := false
	for _, c := range chars {
		if c.ID == char && c.AccountID == actor && c.CorporationID == corp {
			found = true
		}
	}
	if !found {
		return out, pgx.ErrNoRows
	}
	_, cfg, err := s.rule(ctx, corp, kind)
	if err != nil {
		return out, err
	}
	state, err := store.GrowthState(ctx, s.Pool, actor, kind, cfg.ShipTypeID, 0)
	if err != nil {
		return out, err
	}
	if state != "" {
		out.State = state
		return out, nil
	}
	at, err := time.Parse(time.RFC3339, cfg.EffectiveAt)
	if err != nil || !cfg.Enabled || time.Now().Before(at) {
		out.State = "closed"
		return out, nil
	}
	if s.GrowthCheck == nil {
		return out, nil
	}
	_, check, err := s.GrowthCheck(ctx, actor, char, corp, cfg)
	if err != nil {
		return out, nil
	}
	if err = json.Unmarshal(check, &out); err != nil {
		return GrowthStatus{State: "unknown"}, err
	}
	if out.State != "met" && out.State != "missing" {
		out.State = "unknown"
	}
	return out, nil
}

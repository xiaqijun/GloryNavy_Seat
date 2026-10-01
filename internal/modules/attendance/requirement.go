package attendance

import (
	"context"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/attendance/internal/store"
	"net/http"
)

type PAPRequirement struct {
	// The requirement is intentionally scoped to alliance PAP. Local activity
	// awards are corporation PAP and are reported separately.
	Source        string `json:"source"`
	MonthlyPoints int32  `json:"monthly_points"`
	Version       int64  `json:"version,string"`
	CanManage     bool   `json:"can_manage"`
}
type PAPRequirementChange struct {
	MonthlyPoints int32 `json:"monthly_points"`
	Version       int64 `json:"version,string"`
}

func (s *Service) PAPRequirement(ctx context.Context, user string) (PAPRequirement, error) {
	out := PAPRequirement{Source: "alliance"}
	if _, err := uuid(user); err != nil {
		return out, ErrInvalid
	}
	var err error
	if s.Administrator != nil {
		out.CanManage, err = s.Administrator(ctx, user)
		if err != nil {
			return out, err
		}
	}
	row, err := store.New(s.Pool).PAPRequirement(ctx)
	out.MonthlyPoints, out.Version = row.MonthlyPoints, row.Version
	return out, err
}
func (s *Service) SetPAPRequirement(ctx context.Context, user string, c PAPRequirementChange) error {
	if s.Administrator == nil {
		return pgx.ErrNoRows
	}
	ok, err := s.Administrator(ctx, user)
	if err != nil {
		return err
	}
	if !ok {
		return pgx.ErrNoRows
	}
	if _, err = uuid(user); err != nil || c.Version < 1 || c.MonthlyPoints < 1 || c.MonthlyPoints > 100000 {
		return ErrInvalid
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	q := store.New(tx)
	old, err := q.LockPAPRequirement(ctx)
	if err != nil {
		return err
	}
	ok, err = s.Administrator(ctx, user)
	if err != nil {
		return err
	}
	if !ok {
		return pgx.ErrNoRows
	}
	if old.Version != c.Version {
		return ErrConflict
	}
	if old.MonthlyPoints == c.MonthlyPoints {
		return nil
	}
	if err = q.UpdatePAPRequirement(ctx, user, old, c.MonthlyPoints); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (h Handler) papRequirement(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		out, err := h.Service.PAPRequirement(r.Context(), h.User(r))
		respond(w, r, out, err)
		return
	}
	var c PAPRequirementChange
	if !readBody(w, r, &c) {
		return
	}
	err := h.Service.SetPAPRequirement(r.Context(), h.User(r), c)
	respond(w, r, map[string]bool{"saved": err == nil}, err)
}

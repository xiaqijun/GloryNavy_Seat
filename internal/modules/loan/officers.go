package loan

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/loan/internal/store"
)

var ErrOfficer = errors.New("loan officer unavailable")

func (s *Service) poolOfficers(ctx context.Context, p store.Pool) ([]Character, error) {
	if p.CorporationID == nil || s.LoanOfficers == nil {
		return []Character{}, nil
	}
	return s.LoanOfficers(ctx, *p.CorporationID)
}

func (s *Service) Officers(ctx context.Context) ([]Character, error) {
	p, err := store.SharedPool(ctx, s.db(), false)
	if errors.Is(err, pgx.ErrNoRows) {
		return []Character{}, nil
	}
	if err != nil {
		return nil, err
	}
	rows, err := s.poolOfficers(ctx, p)
	if err != nil {
		return nil, err
	}
	out := make([]Character, 0, len(rows))
	for _, c := range rows {
		out = append(out, Character{ID: c.ID, Name: c.Name, IsMain: c.IsMain})
	}
	return out, nil
}

func (s *Service) chooseOfficer(ctx context.Context, p store.Pool, selected int64) (Character, error) {
	rows, err := s.poolOfficers(ctx, p)
	if err != nil {
		return Character{}, err
	}
	if selected == 0 {
		for _, c := range rows {
			if c.IsMain {
				return c, nil
			}
		}
		if len(rows) == 1 {
			return rows[0], nil
		}
	}
	for _, c := range rows {
		if c.ID == selected {
			return c, nil
		}
	}
	return Character{}, ErrOfficer
}

func (s *Service) guardOfficer(ctx context.Context, tx pgx.Tx, p store.Pool, selected int64) error {
	if p.CorporationID == nil || s.GuardLoanOfficer == nil {
		return ErrOfficer
	}
	return s.GuardLoanOfficer(ctx, tx, *p.CorporationID, selected)
}

func (s *Service) cashOfficer(ctx context.Context, p store.Pool, contract Contract, other cashParty, amount int64, outgoing bool) (Character, error) {
	rows, err := s.poolOfficers(ctx, p)
	if err != nil {
		return Character{}, err
	}
	if len(rows) == 0 {
		return Character{}, ErrOfficer
	}
	for _, c := range rows {
		party := cashParty{"character", c.ID}
		if partyEqual(party, other) {
			continue
		}
		payer, receiver := other, party
		if outgoing {
			payer, receiver = party, other
		}
		if cashMatches(contract, payer, receiver, amount) {
			return c, nil
		}
	}
	return Character{}, ErrContract
}

// Only the completed recipient counts; an assignee alone is not delivery.
func (s *Service) collateralMatchesPool(ctx context.Context, p store.Pool, c Contract) bool {
	if c.ForCorporation || c.AcceptorID <= 0 {
		return false
	}
	officer, err := s.chooseOfficer(ctx, p, c.AcceptorID)
	return err == nil && officer.ID == c.AcceptorID
}

// Fence character ownership before account and business locks. Candidates are
// reauthorized by guardOfficer before publishing any contract claim or ledger.
func (s *Service) lockOfficerAccounts(ctx context.Context, tx pgx.Tx, p store.Pool, actors ...string) error {
	rows, err := s.poolOfficers(ctx, p)
	if err != nil {
		return err
	}
	if len(rows) == 0 || s.LockOfficerCharacters == nil {
		return ErrOfficer
	}
	ids := make([]int64, 0, len(rows))
	for _, c := range rows {
		ids = append(ids, c.ID)
		if c.AccountID != "" {
			actors = append(actors, c.AccountID)
		}
	}
	if err = s.LockOfficerCharacters(ctx, tx, ids); err != nil {
		return err
	}
	return s.guardAccounts(ctx, tx, actors...)
}

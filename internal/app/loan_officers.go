package app

import (
	"context"
	"sort"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/access"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/modules/loan"
)

func wireLoanOfficers(service *loan.Service, accounts *identity.Service, permissions *access.Service, reader *eve.AuthorizationService) {
	target := func(ctx context.Context, id int64) (access.Corporation, error) {
		c, err := reader.Corporation(ctx, id)
		return access.Corporation{ID: c.CorporationID, Name: c.CorporationName, AllianceID: c.AllianceID, CEOID: c.CEOID}, err
	}
	service.LoanOfficers = func(ctx context.Context, corporation int64) ([]loan.Character, error) {
		corp, err := target(ctx, corporation)
		if err != nil {
			return nil, err
		}
		users, err := permissions.AssignedPermissionMembers(ctx, "corporation.loan", corp)
		if err != nil {
			return nil, err
		}
		ids, err := accounts.MainCharacterIDs(ctx, users)
		if err != nil {
			return nil, err
		}
		out := []loan.Character{}
		for _, user := range users {
			characters, err := accounts.ActiveCharacters(ctx, user)
			if err != nil {
				return nil, err
			}
			for _, c := range characters {
				out = append(out, loan.Character{ID: c.ID, Name: c.Name, AccountID: user, IsMain: ids[user] == c.ID})
			}
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].IsMain != out[j].IsMain {
				return out[i].IsMain
			}
			if out[i].Name == out[j].Name {
				return out[i].ID < out[j].ID
			}
			return out[i].Name < out[j].Name
		})
		return out, nil
	}
	service.LockOfficerCharacters = func(ctx context.Context, tx pgx.Tx, ids []int64) error {
		_, err := accounts.Bindings(ctx, tx, ids)
		return err
	}
	service.GuardLoanOfficer = func(ctx context.Context, tx pgx.Tx, corporation, character int64) error {
		bindings, err := accounts.Bindings(ctx, tx, []int64{character})
		if err != nil {
			return err
		}
		if len(bindings) != 1 {
			return loan.ErrOfficer
		}
		user, err := accounts.UserForCharacter(ctx, character)
		if err != nil {
			return loan.ErrOfficer
		}
		if user != bindings[0].UserID {
			return loan.ErrOfficer
		}
		if tx != nil {
			if err := accounts.LockActiveAccounts(ctx, tx, []string{user}); err != nil {
				return loan.ErrOfficer
			}
		}
		characters, err := accounts.ActiveCharacters(ctx, user)
		if err != nil {
			return err
		}
		active := false
		for _, c := range characters {
			if c.ID == character {
				active = true
			}
		}
		if !active {
			return loan.ErrOfficer
		}
		corp, err := target(ctx, corporation)
		if err != nil {
			return err
		}
		ok, err := permissions.AssignedPermission(ctx, tx, user, "corporation.loan", corp)
		if err != nil {
			return err
		}
		if !ok {
			return loan.ErrOfficer
		}
		return nil
	}
}

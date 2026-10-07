package app

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/modules/access"
	"glorynavy.local/seat/internal/modules/attendance"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/platform/locale"
	"net/http"
	"slices"
)

func attendanceHandler(pool *pgxpool.Pool, accounts *identity.Service, policy *access.Service, reader *eve.AuthorizationService) attendance.Handler {
	s := &attendance.Service{Pool: pool, EVE: reader, Administrator: policy.IsAdministrator}
	s.AlliancePAPMemberNames = accounts.MainCharacterNames
	s.Bindings = func(ctx context.Context, tx pgx.Tx, ids []int64) ([]attendance.Binding, error) {
		rows, err := accounts.Bindings(ctx, tx, ids)
		if err != nil {
			return nil, err
		}
		result := make([]attendance.Binding, 0, len(rows))
		for _, b := range rows {
			result = append(result, attendance.Binding{ID: b.ID, Name: b.Name, UserID: b.UserID, OwnerHash: b.OwnerHash})
		}
		return result, nil
	}
	s.Own = func(ctx context.Context, user string) ([]attendance.Binding, error) {
		rows, err := accounts.ActiveCharacters(ctx, user)
		if err != nil {
			return nil, err
		}
		result := make([]attendance.Binding, 0, len(rows))
		for _, b := range rows {
			result = append(result, attendance.Binding{ID: b.ID, Name: b.Name, UserID: user, OwnerHash: b.OwnerHash})
		}
		return result, nil
	}
	s.Manage = func(ctx context.Context, user string, id int64) (bool, error) {
		corp, err := reader.Corporation(ctx, id)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return false, err
		}

		return policy.Can(ctx, user, "corporation.attendance", access.Corporation{ID: id, Name: corp.CorporationName, AllianceID: corp.AllianceID, CEOID: corp.CEOID})
	}
	s.Corporations = func(ctx context.Context, user string) ([]attendance.Corporation, error) {
		ids, err := reader.ActivityCorporations(ctx)
		if err != nil {
			return nil, err
		}
		history, err := s.EventCorporations(ctx)
		if err != nil {
			return nil, err
		}
		for _, id := range history {
			if !slices.Contains(ids, id) {
				ids = append(ids, id)
			}
		}
		result := []attendance.Corporation{}
		for _, id := range ids {
			ok, err := s.Manage(ctx, user, id)
			if err != nil {
				return nil, err
			}
			if ok {
				corp, err := reader.Corporation(ctx, id)
				if err != nil && !errors.Is(err, pgx.ErrNoRows) {
					return nil, err
				}
				name := corp.CorporationName
				if name == "" {
					name = fmt.Sprintf(locale.Choose(ctx, "军团 #%d", "Corporation #%d"), id)
				}
				result = append(result, attendance.Corporation{ID: id, Name: name})
			}
		}
		return result, nil
	}
	s.ReportBindings = func(ctx context.Context, actor, member string, corp int64) ([]attendance.Binding, error) {
		if corp > 0 && member != "" {
			return nil, attendance.ErrInvalid
		}
		var bindings []attendance.Binding
		var err error
		if corp > 0 {
			known, err := s.Corporations(ctx, actor)
			if err != nil {
				return nil, err
			}
			found := false
			for _, c := range known {
				if c.ID == corp {
					found = true
				}
			}
			if !found {
				return nil, pgx.ErrNoRows
			}
			ok, err := s.Manage(ctx, actor, corp)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, pgx.ErrNoRows
			}
			ids, err := reader.ActivityCharacters(ctx, corp)
			if err != nil {
				return nil, err
			}
			bindings, err = s.Bindings(ctx, nil, ids)
			if err != nil {
				return nil, err
			}
		} else {
			target := actor
			if member != "" && member != actor {
				admin, err := policy.IsAdministrator(ctx, actor)
				if err != nil {
					return nil, err
				}
				if !admin {
					return nil, pgx.ErrNoRows
				}
				target = member
			}
			bindings, err = s.Own(ctx, target)
			if err != nil {
				return nil, err
			}
			if target != actor && len(bindings) == 0 {
				return nil, pgx.ErrNoRows
			}
		}
		// The EVE owner hash must still match the active site's binding.
		result := []attendance.Binding{}
		for _, b := range bindings {
			a, err := reader.Get(ctx, b.ID)
			if err != nil {
				return nil, err
			}
			if len(a.OwnerHash) > 0 && subtle.ConstantTimeCompare(a.OwnerHash, b.OwnerHash) != 1 {
				continue
			}
			result = append(result, b)
		}
		slices.SortFunc(result, func(a, b attendance.Binding) int {
			if a.ID < b.ID {
				return -1
			}
			if a.ID > b.ID {
				return 1
			}
			return 0
		})
		return result, nil
	}
	return attendance.Handler{Service: s, User: func(r *http.Request) string { return identity.Principal(r.Context()).UserID }}
}

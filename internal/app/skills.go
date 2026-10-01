package app

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/modules/access"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/modules/skills"
	"glorynavy.local/seat/internal/platform/locale"
	"net/http"
	"time"
)

func skillsHandler(pool *pgxpool.Pool, accounts *identity.Service, policy *access.Service, reader *eve.AuthorizationService, names *eve.StaticDataService, canRead func(context.Context, string, int64) (bool, error)) skills.Handler {
	if reader == nil {
		reader = eve.ReadAuthorization(pool)
	}
	s := &skills.Service{Pool: pool, CanRead: canRead, Snapshot: reader.SkillSnapshot, Names: names}
	s.Characters = func(ctx context.Context, actor, member string) ([]skills.Character, error) {
		subject := actor
		if member != "" && member != actor {
			ok, err := policy.IsAdministrator(ctx, actor)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, pgx.ErrNoRows
			}
			subject = member
		}
		rows, err := accounts.ActiveCharacters(ctx, subject)
		if err != nil {
			return nil, err
		}
		if subject != actor && len(rows) == 0 {
			return nil, pgx.ErrNoRows
		}
		out := []skills.Character{}
		for _, c := range rows {
			a, err := reader.Get(ctx, c.ID)
			if err != nil {
				return nil, err
			}
			var corp int64
			if subtle.ConstantTimeCompare(a.OwnerHash, c.OwnerHash) == 1 && (a.State == "ready" || a.State == "retry") && a.ValidUntil.After(time.Now()) {
				corp = a.CorporationID
			}
			out = append(out, skills.Character{ID: c.ID, Name: c.Name, CorporationID: corp})
		}
		return out, nil
	}
	s.Manage = func(ctx context.Context, user string, id int64) (bool, error) {
		c, err := reader.Corporation(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return policy.Can(ctx, user, "corporation.skills", access.Corporation{ID: id, Name: c.CorporationName, AllianceID: c.AllianceID, CEOID: c.CEOID})
	}
	s.Corporations = func(ctx context.Context, user string) ([]skills.Corporation, error) {
		own, err := s.Characters(ctx, user, "")
		if err != nil {
			return nil, err
		}
		ids, err := reader.ActivityCorporations(ctx)
		if err != nil {
			return nil, err
		}
		out := []skills.Corporation{}
		admin, err := policy.IsAdministrator(ctx, user)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			manage, err := s.Manage(ctx, user, id)
			if err != nil {
				return nil, err
			}
			member := false
			for _, c := range own {
				if c.CorporationID == id {
					member = true
				}
			}
			if !member && !manage && !admin {
				continue
			}
			c, err := reader.Corporation(ctx, id)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return nil, err
			}
			name := c.CorporationName
			if name == "" {
				name = fmt.Sprintf(locale.Choose(ctx, "军团 #%d", "Corporation #%d"), id)
			}
			out = append(out, skills.Corporation{ID: id, Name: name, CanManage: manage})
		}
		return out, nil
	}
	s.Members = func(ctx context.Context, user string, corp int64) ([]skills.Character, error) {
		ok, err := s.Manage(ctx, user, corp)
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
		rows, err := accounts.Bindings(ctx, nil, ids)
		if err != nil {
			return nil, err
		}
		out := []skills.Character{}
		for _, c := range rows {
			a, err := reader.Get(ctx, c.ID)
			if err != nil {
				return nil, err
			}
			if subtle.ConstantTimeCompare(a.OwnerHash, c.OwnerHash) != 1 || a.CorporationID != corp || !a.ValidUntil.After(time.Now()) || (a.State != "ready" && a.State != "retry") {
				continue
			}
			out = append(out, skills.Character{ID: c.ID, Name: c.Name, CorporationID: corp})
		}
		return out, nil
	}
	return skills.Handler{Service: s, User: func(r *http.Request) string { return identity.Principal(r.Context()).UserID }}
}

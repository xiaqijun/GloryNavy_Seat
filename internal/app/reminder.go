package app

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/modules/access"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/modules/reminder"
	"net/http"
)

func reminderHandler(pool *pgxpool.Pool, accounts *identity.Service, policy *access.Service, authorization *eve.AuthorizationService, enabled bool, origin string) reminder.Handler {
	s := &reminder.Service{Pool: pool, Enabled: enabled, Origin: origin, Administrator: policy.IsAdministrator, Users: accounts.AllUserIDs, LockAccounts: accounts.LockActiveAccounts}
	s.Own = func(ctx context.Context, user string) ([]reminder.Character, error) {
		rows, err := accounts.ActiveCharacters(ctx, user)
		if err != nil {
			return nil, err
		}
		out := []reminder.Character{}
		for _, r := range rows {
			generation, ready, e := authorization.MailGrant(ctx, r.ID, r.OwnerHash)
			if e != nil {
				return nil, e
			}
			out = append(out, reminder.Character{ID: r.ID, Name: r.Name, Owner: r.OwnerHash, Generation: generation, Ready: ready})
		}
		return out, nil
	}
	s.Main = func(ctx context.Context, user string) (reminder.Character, error) {
		ids, err := accounts.MainCharacterIDs(ctx, []string{user})
		if err != nil {
			return reminder.Character{}, err
		}
		rows, err := accounts.ActiveCharacters(ctx, user)
		if err != nil {
			return reminder.Character{}, err
		}
		for _, r := range rows {
			if r.ID == ids[user] {
				return reminder.Character{ID: r.ID, Name: r.Name, Owner: r.OwnerHash}, nil
			}
		}
		return reminder.Character{}, pgx.ErrNoRows
	}
	s.Send = func(ctx context.Context, ch reminder.Character, recipient int64, subject, body string) reminder.Result {
		r := authorization.SendMail(ctx, ch.ID, ch.Owner, ch.Generation, recipient, subject, body)
		return reminder.Result{ID: r.ID, State: r.State, Reason: r.Reason}
	}
	s.SendCorporation = func(ctx context.Context, ch reminder.Character, corporation int64, subject, body string) reminder.Result {
		r := authorization.SendCorporationMail(ctx, ch.ID, ch.Owner, ch.Generation, corporation, subject, body)
		return reminder.Result{ID: r.ID, State: r.State, Reason: r.Reason}
	}
	return reminder.Handler{Service: s, User: func(r *http.Request) string { return identity.Principal(r.Context()).UserID }}
}

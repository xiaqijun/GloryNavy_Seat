package app

import (
	"context"
	"crypto/subtle"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/access"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/modules/wallet"
	"net/http"
	"slices"
	"time"
)

func walletHandler(accounts *identity.Service, acl *access.Service, reader *eve.AuthorizationService, static *eve.StaticDataService, canRead func(context.Context, string, int64) (bool, error)) wallet.Handler {
	h := wallet.Handler{User: func(r *http.Request) string { return identity.Principal(r.Context()).UserID }, Data: reader.WalletData, Summaries: reader.WalletSummaries, CorporationSummary: reader.WalletCorporationSummary, CorporationFinanceTrend: reader.WalletCorporationFinanceTrend, Names: reader.WalletNames, Types: static}
	h.CorporationPersonalSummary = func(ctx context.Context, actor string, corporationID int64, from time.Time) (eve.WalletSummary, error) {
		allowed, e := acl.Can(ctx, actor, "access.members.read", access.Corporation{ID: corporationID})
		if e != nil {
			return eve.WalletSummary{}, e
		}
		if !allowed {
			return eve.WalletSummary{}, pgx.ErrNoRows
		}
		characters, e := reader.ActivityCharacters(ctx, corporationID)
		if e != nil {
			return eve.WalletSummary{}, e
		}
		bindings, e := accounts.Bindings(ctx, nil, characters)
		if e != nil {
			return eve.WalletSummary{}, e
		}
		bound := make(map[int64]identity.Binding, len(bindings))
		for _, binding := range bindings {
			bound[binding.ID] = binding
		}
		ids := make([]int64, 0, len(characters))
		for _, characterID := range characters {
			binding, ok := bound[characterID]
			if !ok {
				continue
			}
			credential, e := reader.Get(ctx, characterID)
			if e != nil {
				return eve.WalletSummary{}, e
			}
			if credential.State == "reauthorize" || !slices.Contains(credential.Scopes, eve.CharacterWalletScope) || subtle.ConstantTimeCompare(credential.OwnerHash, binding.OwnerHash) != 1 {
				continue
			}
			ids = append(ids, characterID)
		}
		return reader.WalletCorporationPersonalSummary(ctx, corporationID, from, ids)
	}
	h.CorporationPersonalIncomeTrend = func(ctx context.Context, actor string, corporationID int64, from, until time.Time) ([]eve.WalletIncomeTrend, error) {
		allowed, e := acl.Can(ctx, actor, "access.members.read", access.Corporation{ID: corporationID})
		if e != nil {
			return nil, e
		}
		if !allowed {
			return nil, pgx.ErrNoRows
		}
		characters, e := reader.ActivityCharacters(ctx, corporationID)
		if e != nil {
			return nil, e
		}
		bindings, e := accounts.Bindings(ctx, nil, characters)
		if e != nil {
			return nil, e
		}
		bound := make(map[int64]identity.Binding, len(bindings))
		for _, binding := range bindings {
			bound[binding.ID] = binding
		}
		ids := make([]int64, 0, len(characters))
		for _, characterID := range characters {
			binding, ok := bound[characterID]
			if !ok {
				continue
			}
			credential, e := reader.Get(ctx, characterID)
			if e != nil {
				return nil, e
			}
			if credential.State == "reauthorize" || !slices.Contains(credential.Scopes, eve.CharacterWalletScope) || subtle.ConstantTimeCompare(credential.OwnerHash, binding.OwnerHash) != 1 {
				continue
			}
			ids = append(ids, characterID)
		}
		return reader.WalletCorporationPersonalIncomeTrend(ctx, corporationID, from, until, ids)
	}
	personalOwners := func(ctx context.Context, actor, member string) ([]wallet.Owner, error) {
		subject := actor
		if member != "" && member != actor {
			ok, e := acl.IsAdministrator(ctx, actor)
			if e != nil {
				return nil, e
			}
			if !ok {
				return nil, pgx.ErrNoRows
			}
			subject = member
		}
		chars, e := accounts.ActiveCharacters(ctx, subject)
		if e != nil {
			return nil, e
		}
		if subject != actor && len(chars) == 0 {
			return nil, pgx.ErrNoRows
		}
		ids := make([]int64, 0, len(chars))
		for _, ch := range chars {
			ids = append(ids, ch.ID)
		}
		bindings, e := accounts.Bindings(ctx, nil, ids)
		if e != nil {
			return nil, e
		}
		bound := make(map[int64]identity.Binding, len(bindings))
		for _, binding := range bindings {
			if binding.UserID == subject {
				bound[binding.ID] = binding
			}
		}
		out := []wallet.Owner{}
		for _, ch := range chars {
			binding, ok := bound[ch.ID]
			if !ok {
				continue
			}
			a, e := reader.Get(ctx, ch.ID)
			if e != nil {
				return nil, e
			}
			if a.State == "reauthorize" || !slices.Contains(a.Scopes, eve.CharacterWalletScope) || subtle.ConstantTimeCompare(a.OwnerHash, binding.OwnerHash) != 1 {
				continue
			}
			out = append(out, wallet.Owner{Kind: "character", ID: ch.ID, Name: ch.Name, Divisions: []int{0}, Journal: true, Transactions: true})
		}
		return out, nil
	}
	h.PersonalOwners = func(ctx context.Context, actor string) ([]wallet.Owner, error) {
		return personalOwners(ctx, actor, "")
	}
	h.Owners = func(ctx context.Context, actor, member string) ([]wallet.Owner, error) {
		out, e := personalOwners(ctx, actor, member)
		if e != nil {
			return nil, e
		}
		account, e := acl.Account(ctx, actor)
		if e != nil {
			return nil, e
		}
		corps, e := reader.WalletCorporations(ctx)
		if e != nil {
			return nil, e
		}
		for _, c := range corps {
			target := access.Corporation{ID: c.ID, Name: c.Name, AllianceID: c.AllianceID, CEOID: c.CEOID}
			journal, transactions := account.Can("corporation.journal", target), account.Can("corporation.transaction", target)
			if !journal && !transactions {
				continue
			}
			divs := []int{}
			for i, n := range []string{"first", "second", "third", "fourth", "fifth", "sixth", "seventh"} {
				if account.Can("corporation.wallet_"+n+"_division", target) {
					divs = append(divs, i+1)
				}
			}
			if len(divs) > 0 {
				out = append(out, wallet.Owner{Kind: "corporation", ID: c.ID, Name: c.Name, Divisions: divs, Journal: journal, Transactions: transactions})
			}
		}
		return out, nil
	}
	h.Authorize = func(ctx context.Context, actor, kind string, id int64) (wallet.Owner, error) {
		member := ""
		if kind == "character" {
			ok, e := canRead(ctx, actor, id)
			if e != nil {
				return wallet.Owner{}, e
			}
			if !ok {
				return wallet.Owner{}, pgx.ErrNoRows
			}
			member, e = accounts.UserForCharacter(ctx, id)
			if e != nil {
				return wallet.Owner{}, e
			}
		}
		rows, e := h.Owners(ctx, actor, member)
		if e != nil {
			return wallet.Owner{}, e
		}
		for _, o := range rows {
			if o.Kind == kind && o.ID == id {
				return o, nil
			}
		}
		return wallet.Owner{}, pgx.ErrNoRows
	}
	return h
}

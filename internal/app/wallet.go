package app

import (
	"context"
	"crypto/subtle"
	"math/big"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/access"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/modules/wallet"
	"net/http"
	"slices"
	"strings"
	"time"
)

func walletHandler(accounts *identity.Service, acl *access.Service, reader *eve.AuthorizationService, static *eve.StaticDataService, canRead func(context.Context, string, int64) (bool, error), nickname func(context.Context, []string) (map[string]string, error)) wallet.Handler {
	h := wallet.Handler{User: func(r *http.Request) string { return identity.Principal(r.Context()).UserID }, Data: reader.WalletData, Summaries: reader.WalletSummaries, CorporationSummary: reader.WalletCorporationSummary, CorporationFinanceTrend: reader.WalletCorporationFinanceTrend, Names: reader.WalletNames, Types: static}
	readableCharacters := func(ctx context.Context, actor string, corporationID int64) ([]identity.Binding, error) {
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
		out := make([]identity.Binding, 0, len(characters))
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
			out = append(out, binding)
		}
		return out, nil
	}
	h.CorporationPersonalSummary = func(ctx context.Context, actor string, corporationID int64, from time.Time) (eve.WalletSummary, error) {
		bindings, e := readableCharacters(ctx, actor, corporationID)
		if e != nil {
			return eve.WalletSummary{}, e
		}
		ids := make([]int64, 0, len(bindings))
		for _, binding := range bindings {
			ids = append(ids, binding.ID)
		}
		return reader.WalletCorporationPersonalSummary(ctx, corporationID, from, ids)
	}
	h.CorporationPersonalIncomeTrend = func(ctx context.Context, actor string, corporationID int64, from, until time.Time) ([]eve.WalletIncomeTrend, error) {
		bindings, e := readableCharacters(ctx, actor, corporationID)
		if e != nil {
			return nil, e
		}
		ids := make([]int64, 0, len(bindings))
		for _, binding := range bindings {
			ids = append(ids, binding.ID)
		}
		return reader.WalletCorporationPersonalIncomeTrend(ctx, corporationID, from, until, ids)
	}
	h.CorporationPersonalTaxRanking = func(ctx context.Context, actor string, corporationID int64, from, until time.Time) ([]wallet.TaxRankingRow, error) {
		bindings, e := readableCharacters(ctx, actor, corporationID)
		if e != nil {
			return nil, e
		}
		ids := make([]int64, 0, len(bindings))
		byCharacter := make(map[int64]identity.Binding, len(bindings))
		for _, binding := range bindings {
			ids = append(ids, binding.ID)
			byCharacter[binding.ID] = binding
		}
		rows, e := reader.WalletCorporationPersonalTaxRanking(ctx, corporationID, from, until, ids)
		if e != nil {
			return nil, e
		}
		users := uniqueBindingUsers(bindings)
		accountNames := map[string]string{}
		if nickname != nil {
			accountNames, _ = nickname(ctx, users)
		}
		mainNames, _ := accounts.MainCharacterNames(ctx, users)
		type aggregate struct {
			name string
			tax  *big.Rat
		}
		byAccount := map[string]*aggregate{}
		for _, row := range rows {
			binding, ok := byCharacter[row.CharacterID]
			if !ok {
				continue
			}
			value, ok := new(big.Rat).SetString(row.Tax)
			if !ok || value.Sign() <= 0 {
				continue
			}
			entry := byAccount[binding.UserID]
			if entry == nil {
				entry = &aggregate{name: binding.Name, tax: new(big.Rat)}
				byAccount[binding.UserID] = entry
			}
			entry.tax.Add(entry.tax, value)
		}
		out := make([]wallet.TaxRankingRow, 0, len(byAccount))
		for userID, entry := range byAccount {
			name := accountNames[userID]
			if name == "" {
				name = mainNames[userID]
			}
			if name == "" {
				name = entry.name
			}
			out = append(out, wallet.TaxRankingRow{Nickname: name, Tax: entry.tax.FloatString(2)})
		}
		slices.SortFunc(out, func(a, b wallet.TaxRankingRow) int {
			av, _ := new(big.Rat).SetString(a.Tax)
			bv, _ := new(big.Rat).SetString(b.Tax)
			if cmp := bv.Cmp(av); cmp != 0 {
				return cmp
			}
			return strings.Compare(a.Nickname, b.Nickname)
		})
		if len(out) > 10 {
			out = out[:10]
		}
		return out, nil
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

func uniqueBindingUsers(bindings []identity.Binding) []string {
	seen := make(map[string]struct{}, len(bindings))
	out := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		if binding.UserID == "" {
			continue
		}
		if _, ok := seen[binding.UserID]; ok {
			continue
		}
		seen[binding.UserID] = struct{}{}
		out = append(out, binding.UserID)
	}
	return out
}

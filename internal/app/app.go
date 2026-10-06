// Package app is the composition root: it is the only place assembling modules.
package app

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/module"
	"glorynavy.local/seat/internal/modules/access"
	"glorynavy.local/seat/internal/modules/attendance"
	"glorynavy.local/seat/internal/modules/community"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/exchange"
	"glorynavy.local/seat/internal/modules/fittings"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/modules/loan"
	"glorynavy.local/seat/internal/modules/market"
	"glorynavy.local/seat/internal/modules/sentry"
	"glorynavy.local/seat/internal/modules/system"
	"glorynavy.local/seat/internal/modules/welfare"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"
)

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

type AuthConfig struct {
	Origin, ClientID, ClientSecret, TokenKey, ESIUserAgent string
	SDEAutoUpdate                                          bool
	SDEWorkDir                                             string
	SDECheckInterval                                       time.Duration
	WinterCoPAPURL                                         string
	WinterCoPAPAuthFile                                    string
	QQBotAppID                                             string
	QQBotAppSecret                                         string
	QQBotAPIBase                                           string
	QQBotGroupOpenIDs                                      []string
	SentryIntegrationURL                                   string
	SentryIntegrationToken                                 string
	AlertConsumptionEnabled                                bool
	AlertPriceVersion                                      string
	AlertUnitSeconds                                       int64
	AlertUnitPriceMinor                                    int64
	AlertMaxGrantSeconds                                   int64
	AlertGrantTTL                                          time.Duration
}
type Application struct {
	http.Handler
	background func(context.Context)
}

func (a *Application) Run(ctx context.Context) {
	if a.background != nil {
		a.background(ctx)
	}
}

func New(pool *pgxpool.Pool, logger *slog.Logger, version string, enabled []string, auth AuthConfig) (*Application, error) {
	if !slices.Contains(enabled, "system") {
		return nil, fmt.Errorf("required module system cannot be disabled")
	}
	status := system.StatusHandler(system.New(pool, version), logger)
	identityService := identity.New(pool)
	communityService := community.New(pool)
	if err := communityService.SetQQBotConfigKey(auth.TokenKey); err != nil {
		return nil, err
	}
	communityService.SetQQBotOfficial(auth.QQBotAppID, auth.QQBotAppSecret, auth.QQBotAPIBase)
	communityService.SetQQBotGroups(auth.QQBotGroupOpenIDs)
	if pool != nil {
		if err := communityService.LoadQQBotSettings(context.Background()); err != nil {
			return nil, fmt.Errorf("load QQ Bot settings: %w", err)
		}
		if err := communityService.LoadQQBotGroups(context.Background()); err != nil {
			return nil, fmt.Errorf("load QQ group settings: %w", err)
		}
	}
	identityHandler := identity.Handler{Service: identityService, Origin: auth.Origin, Secure: strings.HasPrefix(auth.Origin, "https://")}
	client := eve.NewClient(auth.ClientID, auth.ClientSecret, auth.Origin+"/api/v1/eve/callback")
	reader := eve.ReadAuthorization(pool)
	var syncService *eve.AuthorizationService
	if slices.Contains(enabled, "eve") && client.Configured() {
		client.EnableSeatDefaultScopes()
		if slices.Contains(enabled, "fittings") {
			client.EnableFittingsWriteScope()
		}
		var err error
		syncService, err = eve.NewAuthorization(pool, client, auth.TokenKey, logger)
		if err != nil {
			return nil, err
		}
		reader = syncService
	}
	var esiSync *eve.SyncService
	staticData := eve.NewStaticData(pool, auth.SDEWorkDir, auth.SDECheckInterval)

	corpDTO := func(a eve.Authorization) access.Corporation {
		return access.Corporation{ID: a.CorporationID, Name: a.CorporationName, AllianceID: a.AllianceID, CEOID: a.CEOID}
	}
	accessService := access.New(pool, func(ctx context.Context, user string) ([]access.Fact, error) {
		owned, err := identityService.ActiveCharacters(ctx, user)
		if err != nil {
			return nil, err
		}
		facts := make([]access.Fact, 0, len(owned))
		for _, ch := range owned {
			a, err := reader.Get(ctx, ch.ID)
			if err != nil {
				return nil, err
			}
			if len(a.OwnerHash) > 0 && subtle.ConstantTimeCompare(a.OwnerHash, ch.OwnerHash) != 1 {
				a = eve.Authorization{CharacterID: ch.ID, State: "reauthorize", Roles: []string{}, HQ: []string{}, Base: []string{}, Other: []string{}}
			}
			needsAuthorization := false
			for _, scope := range client.RequestedScopes() {
				if !slices.Contains(a.Scopes, scope) {
					needsAuthorization = true
					break
				}
			}
			facts = append(facts, access.Fact{NeedsAuthorization: needsAuthorization, CharacterID: ch.ID, State: a.State, Corporation: corpDTO(a), Roles: a.Roles, RolesAtHQ: a.HQ, RolesAtBase: a.Base, RolesAtOther: a.Other, SyncedAt: a.SyncedAt, ValidUntil: a.ValidUntil})
		}
		return facts, nil
	}, func(ctx context.Context, id int64) (access.Corporation, error) {
		a, err := reader.Corporation(ctx, id)
		return corpDTO(a), err
	})
	structureHandler := structuresHandler(identityService, accessService, syncService, staticData)
	attendanceModule := attendanceHandler(pool, identityService, accessService, reader)
	attendanceModule.Service.AlliancePAP = attendance.AlliancePAPConfig{URL: auth.WinterCoPAPURL, AuthFile: auth.WinterCoPAPAuthFile}
	attendanceModule.Service.Battle = reader
	attendanceModule.Service.Names = staticData
	alertConsumptionCapability := slices.Contains(enabled, "sentry") && slices.Contains(enabled, "exchange") && slices.Contains(enabled, "eve") && auth.SentryIntegrationURL != "" && auth.SentryIntegrationToken != ""
	exchangeService := &exchange.Service{Pool: pool, Administrator: accessService.IsAdministrator, Names: staticData, SearchTypes: staticData.SearchTypes, Sources: map[string]string{}, SourceScales: map[string]int64{}, AllowNew: slices.Contains(enabled, "exchange"), AllowAlertConsumption: alertConsumptionCapability}
	if slices.Contains(enabled, "attendance") {
		exchangeService.Sources["pap"] = "PAP 集结分"
		exchangeService.Sources["alliance_pap"] = "联盟 PAP"
		exchangeService.SourceScales["alliance_pap"] = 100
	}
	exchangeService.MemberExists = func(ctx context.Context, user string) (bool, error) {
		chars, err := identityService.ActiveCharacters(ctx, user)
		return len(chars) > 0, err
	}
	exchangeService.Own = func(ctx context.Context, user string) ([]exchange.Binding, error) {
		rows, err := identityService.ActiveCharacters(ctx, user)
		out := []exchange.Binding{}
		for _, r := range rows {
			out = append(out, exchange.Binding{ID: r.ID, Name: r.Name, UserID: user})
		}
		return out, err
	}
	exchangeService.Bindings = func(ctx context.Context, tx pgx.Tx, ids []int64) ([]exchange.Binding, error) {
		rows, err := identityService.Bindings(ctx, tx, ids)
		out := []exchange.Binding{}
		for _, r := range rows {
			out = append(out, exchange.Binding{ID: r.ID, Name: r.Name, UserID: r.UserID})
		}
		return out, err
	}
	exchangeModule := exchange.Handler{Service: exchangeService, User: func(r *http.Request) string { return identity.Principal(r.Context()).UserID }}
	// Keep existing currency corrections active when the exchange UI is disabled.
	attendanceModule.Service.CoinAwards = func(ctx context.Context, tx pgx.Tx, key, reason string, rows []attendance.PAPCoinAward) error {
		awards := []exchange.Award{}
		for _, r := range rows {
			awards = append(awards, exchange.Award{Reference: r.Reference, AccountID: r.AccountID, Previous: r.Previous, Units: r.Units})
		}
		err := exchangeService.ReconcileTx(ctx, tx, "pap", key, reason, awards)
		if errors.Is(err, exchange.ErrRateRequired) {
			return attendance.ErrCoinRateRequired
		}
		return err
	}
	attendanceModule.Service.AllianceLockAccounts = identityService.LockActiveAccounts
	attendanceModule.Service.AllianceCoinAwards = func(ctx context.Context, tx pgx.Tx, key, reason string, rows []attendance.PAPCoinAward) error {
		awards := []exchange.Award{}
		for _, r := range rows {
			awards = append(awards, exchange.Award{Reference: r.Reference, AccountID: r.AccountID, Previous: r.Previous, Units: r.Units})
		}
		return exchangeService.ReconcileTx(ctx, tx, "alliance_pap", key, reason, awards)
	}
	if slices.Contains(enabled, "exchange") {
		attendanceModule.Service.CoinConversion = func(ctx context.Context, tx pgx.Tx, user, key, reason, token string, rows []attendance.PAPCoinAward) (attendance.PAPCoinQuote, error) {
			awards := []exchange.Award{}
			for _, r := range rows {
				awards = append(awards, exchange.Award{Reference: r.Reference, AccountID: r.AccountID, Previous: r.Previous, Units: r.Units})
			}
			q, err := exchangeService.ConversionTx(ctx, tx, user, "pap", key, reason, token, awards)
			switch {
			case errors.Is(err, exchange.ErrRateRequired):
				err = attendance.ErrCoinRateRequired
			case errors.Is(err, exchange.ErrConflict):
				err = attendance.ErrConflict
			case errors.Is(err, exchange.ErrInvalid):
				err = attendance.ErrInvalid
			}
			return attendance.PAPCoinQuote{Token: q.Token, Mode: q.Mode, Points: q.Points, Converted: q.Converted, Pending: q.Pending, CoinsMinor: q.CoinsMinor, Characters: q.Characters, UnitScale: q.UnitScale}, err
		}
		attendanceModule.Service.AllianceCoinConversion = func(ctx context.Context, tx pgx.Tx, user, key, reason, token string, rows []attendance.PAPCoinAward) (attendance.PAPCoinQuote, error) {
			awards := make([]exchange.Award, 0, len(rows))
			for _, r := range rows {
				awards = append(awards, exchange.Award{Reference: r.Reference, AccountID: r.AccountID, Previous: r.Previous, Units: r.Units})
			}
			q, err := exchangeService.ConversionTx(ctx, tx, user, "alliance_pap", key, reason, token, awards)
			switch {
			case errors.Is(err, exchange.ErrRateRequired):
				err = attendance.ErrCoinRateRequired
			case errors.Is(err, exchange.ErrConflict):
				err = attendance.ErrConflict
			case errors.Is(err, exchange.ErrInvalid):
				err = attendance.ErrInvalid
			}
			return attendance.PAPCoinQuote{Token: q.Token, Mode: q.Mode, Points: q.Points, Converted: q.Converted, Pending: q.Pending, CoinsMinor: q.CoinsMinor, Characters: q.Characters, UnitScale: q.UnitScale}, err
		}
	}
	exchangeService.LockAccounts = identityService.LockActiveAccounts
	exchangeJobs := &exchange.DeliveryJobs{Service: exchangeService, Enabled: slices.Contains(enabled, "exchange")}
	deliveryJobs := &welfare.DeliveryJobs{Enabled: slices.Contains(enabled, "welfare")}
	var sentryRemote sentry.Remote
	if auth.SentryIntegrationURL != "" && auth.SentryIntegrationToken != "" {
		remote, remoteErr := sentry.NewHTTPRemote(auth.SentryIntegrationURL, auth.SentryIntegrationToken)
		if remoteErr != nil {
			return nil, remoteErr
		}
		sentryRemote = remote
	}
	sentryService := sentry.New(pool, sentryRemote)
	if err := sentryService.SetSecretKey(auth.TokenKey); err != nil {
		return nil, err
	}
	sentryService.Administrator = accessService.IsAdministrator
	// The legacy environment value is still parsed for configuration
	// compatibility; the actual on/off state is persisted in
	// sentry_alert_pricing and changed by an administrator from the Sentry page.
	sentryService.AlertEnabled = alertConsumptionCapability && sentryService.ClientUsageRemote != nil
	sentryService.AlertPolicy = sentry.AlertGrantPolicy{PriceVersion: auth.AlertPriceVersion, UnitSeconds: auth.AlertUnitSeconds, UnitPriceMinor: auth.AlertUnitPriceMinor, MaxGrantSeconds: auth.AlertMaxGrantSeconds, GrantTTL: auth.AlertGrantTTL}
	if pool != nil {
		if err := sentryService.LoadAlertPricing(context.Background(), sentryService.AlertPolicy); err != nil {
			return nil, fmt.Errorf("load Sentry alert pricing: %w", err)
		}
	}
	sentryBoundary := &sentryAlertSettlement{exchange: exchangeService}
	sentryService.SetAlertFunding(sentryBoundary)
	sentryService.SetAlertSettlement(sentryBoundary)
	sentryService.SetAlertUsageReader(&sentryAlertUsageReader{exchange: exchangeService})
	sentryService.MonitorRewardFunding = exchangeService
	if pool != nil && slices.Contains(enabled, "eve") {
		var err error
		esiSync, err = eve.NewSync(pool, syncService, logger, identityService.ValidESIIdentity, attendanceModule.Service.Extension(slices.Contains(enabled, "attendance")), deliveryJobs.Extension(), exchangeJobs.Extension(), sentryService.Extension(sentryService.AlertEnabled))
		if err != nil {
			return nil, err
		}
		esiSync.SetUserAgent(auth.ESIUserAgent)
		esiSync.SetOnlineEnabled(slices.Contains(enabled, "attendance"))
		esiSync.SetFittingsEnabled(slices.Contains(enabled, "fittings"))
		esiSync.SetSkillsEnabled(slices.Contains(enabled, "skills"))
		esiSync.SetLossesEnabled(slices.Contains(enabled, "welfare"))
		esiSync.SetWalletEnabled(slices.Contains(enabled, "wallet"))
		if auth.SDEAutoUpdate {
			esiSync.SetStaticData(staticData)
		}
	}
	identityHandler.Cleanup = eve.RemoveCharacterTx
	if slices.Contains(enabled, "community") {
		identityHandler.ProfileGate = func(ctx context.Context, user, permission string) (bool, error) {
			switch permission {
			case "identity.session.read", "identity.session.logout", "identity.characters.read", "identity.characters.manage", "eve.characters.manage", "eve.sync.self", "structures.self", "community.profile.read", "community.profile.update", "community.group.apply", "community.group.manage", "access.self":
				return true, nil
			default:
				return communityService.Complete(ctx, user)
			}
		}
	}
	identityHandler.Check = func(ctx context.Context, s *identity.Session, permission string, _ *http.Request) (bool, error) {
		if permission == "system.status.read" {
			return accessService.IsAdministrator(ctx, s.UserID)
		}
		if permission == "community.group.manage" {
			return accessService.IsAdministrator(ctx, s.UserID)
		}
		if permission == "approval.self" || permission == "market.self" || permission == "wallet.self" || permission == "welfare.self" || permission == "loan.self" || permission == "skills.self" || permission == "fittings.self" || permission == "exchange.self" || permission == "attendance.self" || permission == "sentry.self" || permission == "structures.self" || permission == "eve.characters.manage" || permission == "eve.sync.self" || permission == "eve.contracts.read" || permission == "community.profile.read" || permission == "community.profile.update" {
			return true, nil
		}
		if permission == "sentry.manage" {
			return accessService.IsAdministrator(ctx, s.UserID)
		}
		if !slices.Contains(enabled, "access") {
			return false, nil
		}
		if permission == "access.self" {
			return true, nil
		}
		return accessService.Can(ctx, s.UserID, permission, access.Corporation{})
	}
	accessHandler := access.Handler{Service: accessService, User: func(r *http.Request) string { return identity.Principal(r.Context()).UserID }}
	accessHandler.Directory = func(ctx context.Context, search, after string) ([]access.Member, string, error) {
		rows, next, err := identityService.SearchMembers(ctx, search, after)
		if err != nil {
			return nil, "", err
		}
		items := make([]access.Member, 0, len(rows))
		for _, r := range rows {
			items = append(items, access.Member{UserID: r.UserID, CharacterID: r.Main.ID, Name: r.Main.Name, CharacterCount: r.CharacterCount})
		}
		return items, next, nil
	}
	accessHandler.MemberData = func(ctx context.Context, user string) (access.MemberData, error) {
		data := access.MemberData{UserID: user, Characters: []access.MemberCharacter{}}
		characters, err := identityService.Characters(ctx, user)
		if err != nil {
			return data, err
		}
		if len(characters) == 0 {
			return data, pgx.ErrNoRows
		}
		for _, ch := range characters {
			data.Characters = append(data.Characters, access.MemberCharacter{ID: ch.ID, Name: ch.Name, Status: ch.Status, IsMain: ch.IsMain})
		}
		data.Access, err = accessService.Account(ctx, user)
		if err != nil {
			return data, err
		}
		if slices.Contains(enabled, "community") {
			profile, err := communityService.Get(ctx, user)
			if err != nil {
				return data, err
			}
			data.Community = &access.MemberCommunity{QQ: access.MemberBinding{Value: profile.QQ.Value, Confirmation: profile.QQ.Confirmation}, KOOK: access.MemberBinding{Value: profile.KOOK.Value, Confirmation: profile.KOOK.Confirmation}}
		}
		return data, nil
	}
	canReadCharacter := func(ctx context.Context, actor string, id int64) (bool, error) {
		owner, err := identityService.UserForCharacter(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if owner == actor {
			return true, nil
		}
		if !slices.Contains(enabled, "access") {
			return false, nil
		}
		return accessService.IsAdministrator(ctx, actor)
	}
	fittingService := &fittings.Service{Pool: pool, Administrator: accessService.IsAdministrator, CanReadCharacter: canReadCharacter, Names: staticData, Search: staticData.SearchTypes}
	fittingService.Own = func(ctx context.Context, user string) ([]fittings.Character, error) {
		rows, err := identityService.ActiveCharacters(ctx, user)
		out := []fittings.Character{}
		for _, r := range rows {
			out = append(out, fittings.Character{ID: r.ID, Name: r.Name})
		}
		return out, err
	}
	fittingService.Snapshot = func(ctx context.Context, id int64, resource string) (json.RawMessage, *time.Time, error) {
		if syncService == nil {
			return nil, nil, pgx.ErrNoRows
		}
		return syncService.FittingSnapshot(ctx, id, resource)
	}
	skillModule := skillsHandler(pool, identityService, accessService, syncService, staticData, canReadCharacter)
	fittingService.LibraryCorporations = func(ctx context.Context, user string) ([]fittings.LibraryCorporation, error) {
		rows, err := skillModule.Service.Corporations(ctx, user)
		out := []fittings.LibraryCorporation{}
		for _, c := range rows {
			out = append(out, fittings.LibraryCorporation{ID: c.ID, Name: c.Name, CanCreateSkills: slices.Contains(enabled, "skills") && c.CanManage})
		}
		return out, err
	}
	fittingService.LibraryCharacters = func(ctx context.Context, user string) ([]fittings.LibraryCharacter, error) {
		rows, err := identityService.ActiveCharacters(ctx, user)
		out := []fittings.LibraryCharacter{}
		if err != nil {
			return out, err
		}
		for _, c := range rows {
			a, e := reader.Get(ctx, c.ID)
			if e != nil {
				return nil, e
			}
			out = append(out, fittings.LibraryCharacter{ID: c.ID, Name: c.Name, CanSave: a.State != "reauthorize" && slices.Contains(a.Scopes, eve.FittingsWriteScope)})
		}
		return out, nil
	}
	fittingService.GameSave = func(ctx context.Context, user string, id int64, f eve.GameFitting) eve.FittingWriteResult {
		rows, err := identityService.Bindings(ctx, nil, []int64{id})
		if err != nil || len(rows) != 1 || rows[0].UserID != user {
			return eve.FittingWriteResult{State: "failed", Reason: "identity_changed"}
		}
		return syncService.SaveGameFitting(ctx, id, rows[0].OwnerHash, f)
	}
	fittingModule := fittings.Handler{Service: fittingService, User: func(r *http.Request) string { return identity.Principal(r.Context()).UserID }}
	exchangeService.RewardFitting = func(ctx context.Context, user string, id int64) (exchange.PhysicalFitting, error) {
		f, err := fittingService.LibraryRead(ctx, user, id)
		if err != nil {
			return exchange.PhysicalFitting{}, err
		}
		raw, err := json.Marshal(f.Fit)
		return exchange.PhysicalFitting{ID: f.ID, Name: f.Name, CorporationID: f.CorporationID, ShipTypeID: f.Fit.ShipTypeID, Version: f.Version, Fit: raw}, err
	}
	communityHandler := community.Handler{Service: communityService, User: func(r *http.Request) string { return identity.Principal(r.Context()).UserID }}
	eveHandler := eve.NewHandler(pool, client, nil, auth.Origin, identityHandler.Secure, logger)
	if syncService != nil {
		eveHandler.PublicCorporation = eve.NewPublicCorporation(syncService.ESI())
	}
	eveHandler.Accounts = identityService
	if pool != nil {
		eveHandler.PublicActivity = publicActivity(reader, identityService)
	}
	if pool != nil {
		eveHandler.Contracts = eve.NewContractHTTP(pool, syncService, func(r *http.Request) string { return identity.Principal(r.Context()).UserID })
		eveHandler.Contracts.StaticData = staticData
		eveHandler.Contracts.BindingOwner = func(ctx context.Context, id int64) ([]byte, error) {
			bindings, err := identityService.Bindings(ctx, nil, []int64{id})
			if err != nil {
				return nil, err
			}
			if len(bindings) != 1 {
				return nil, pgx.ErrNoRows
			}
			return bindings[0].OwnerHash, nil
		}
		eveHandler.Contracts.Owners = func(ctx context.Context, user string) ([]eve.ContractOwner, error) {
			characters, err := identityService.ActiveCharacters(ctx, user)
			if err != nil {
				return nil, err
			}
			owners := make([]eve.ContractOwner, 0, len(characters))
			for _, ch := range characters {
				owners = append(owners, eve.ContractOwner{Kind: "character", ID: ch.ID, Name: ch.Name})
			}
			if !slices.Contains(enabled, "access") {
				return owners, nil
			}
			account, err := accessService.Account(ctx, user)
			if err != nil {
				return nil, err
			}
			corporations, err := eveHandler.Contracts.Corporations(ctx)
			if err != nil {
				return nil, err
			}
			for _, c := range corporations {
				if account.Can("corporation.contract", access.Corporation{ID: c.ID, Name: c.Name, AllianceID: c.AllianceID, CEOID: c.CEOID}) {
					owners = append(owners, c)
				}
			}
			return owners, nil
		}
	}
	if eveHandler.Contracts != nil {
		contracts := eveHandler.Contracts
		contracts.MemberOwners = func(ctx context.Context, actor, member string) ([]eve.ContractOwner, error) {
			if !slices.Contains(enabled, "access") {
				return nil, pgx.ErrNoRows
			}
			admin, err := accessService.IsAdministrator(ctx, actor)
			if err != nil {
				return nil, err
			}
			if !admin {
				return nil, pgx.ErrNoRows
			}
			var memberID pgtype.UUID
			if memberID.Scan(member) != nil || !memberID.Valid {
				return nil, pgx.ErrNoRows
			}
			chars, err := identityService.Characters(ctx, member)
			if err != nil {
				return nil, err
			}
			if len(chars) == 0 {
				return nil, pgx.ErrNoRows
			}
			active, err := identityService.ActiveCharacters(ctx, member)
			if err != nil {
				return nil, err
			}
			owners := make([]eve.ContractOwner, 0, len(active))
			for _, ch := range active {
				owners = append(owners, eve.ContractOwner{Kind: "character", ID: ch.ID, Name: ch.Name})
			}
			visible, err := contracts.Owners(ctx, actor)
			if err != nil {
				return nil, err
			}
			for _, o := range visible {
				if o.Kind == "corporation" {
					owners = append(owners, o)
				}
			}
			return owners, nil
		}
		contracts.LookupOwner = func(ctx context.Context, actor, kind string, id int64) (eve.ContractOwner, error) {
			if kind == "character" {
				allowed, err := canReadCharacter(ctx, actor, id)
				if err != nil {
					return eve.ContractOwner{}, err
				}
				if !allowed {
					return eve.ContractOwner{}, pgx.ErrNoRows
				}
				owner, err := identityService.UserForCharacter(ctx, id)
				if err != nil {
					return eve.ContractOwner{}, err
				}
				chars, err := identityService.ActiveCharacters(ctx, owner)
				if err != nil {
					return eve.ContractOwner{}, err
				}
				for _, ch := range chars {
					if ch.ID == id {
						return eve.ContractOwner{Kind: kind, ID: id, Name: ch.Name}, nil
					}
				}
				return eve.ContractOwner{}, pgx.ErrNoRows
			}
			owners, err := contracts.Owners(ctx, actor)
			if err != nil {
				return eve.ContractOwner{}, err
			}
			for _, o := range owners {
				if o.Kind == kind && o.ID == id {
					return o, nil
				}
			}
			return eve.ContractOwner{}, pgx.ErrNoRows
		}
	}
	if esiSync != nil {
		eveHandler.Sync = &eve.SyncHTTP{Service: esiSync, CanRead: canReadCharacter, User: func(r *http.Request) string { return identity.Principal(r.Context()).UserID }, Owns: func(ctx context.Context, user string, id int64) (bool, error) {
			characters, err := identityService.ActiveCharacters(ctx, user)
			if err != nil {
				return false, err
			}
			for _, ch := range characters {
				if ch.ID == id {
					return true, nil
				}
			}
			return false, nil
		}}
	}
	eveHandler.Complete = func(ctx context.Context, character eve.Character, previous string, intent identity.LoginIntent) (string, error) {
		var save identity.CredentialWrite
		if syncService != nil {
			save = func(ctx context.Context, tx pgx.Tx) error { return syncService.SaveTx(ctx, tx, character) }
		}
		if intent.Kind == "merge" {
			return identityService.ProveMerge(ctx, character.ID, character.Owner, previous, intent.UserID, save)
		}
		token, err := identityService.Complete(ctx, character.ID, character.Name, character.Owner, previous, intent, save)
		if syncService != nil && errors.Is(err, identity.ErrOwnership) {
			_ = syncService.Revoke(ctx, character.ID)
		}
		return token, err
	}
	// Include persisted data even when its UI module is disabled.
	welfareModule := welfareHandler(pool, identityService, accessService, syncService, exchangeService, fittingService, skillModule.Service)
	wireWelfareLosses(welfareModule.Service, identityService, reader, staticData)
	wireWelfareAttendance(welfareModule.Service, attendanceModule.Service, identityService)
	if eveHandler.Contracts != nil {
		exchangeService.Contracts = eveHandler.Contracts.RedemptionContracts
		exchangeService.ClaimDelivery = eve.ClaimDeliveryTx
		welfareModule.Service.ClaimDelivery = eve.ClaimDeliveryTx
		welfareModule.Service.PaymentContracts = eveHandler.Contracts.RedemptionContracts
		welfareModule.Service.PaymentBindings = func(ctx context.Context, tx pgx.Tx, ids []int64) (map[int64]string, error) {
			rows, err := identityService.Bindings(ctx, tx, ids)
			out := map[int64]string{}
			for _, row := range rows {
				out[row.ID] = row.UserID
			}
			return out, err
		}
		welfareModule.Service.Contracts = eveHandler.Contracts.DeliveryContracts
		welfareModule.Service.Contract = eveHandler.Contracts.DeliveryContractTx
		welfareModule.Service.PurchaseContract = eveHandler.Contracts.PurchaseContract
	}
	// Batch settlement keeps its orchestration in welfare while reusing the
	// exchange module's single-order verifier through an injected service.
	welfareModule.Service.ExchangeDelivery = exchangeService.CheckDelivery
	welfareModule.Service.ExchangeSettlementComplete = exchangeService.SettlementComplete
	welfareModule.Service.ExchangeSettlementAccount = exchangeService.SettlementAccount
	welfareModule.Service.ExchangeSettlementEligible = exchangeService.SettlementEligible
	welfareModule.Service.ExchangeSettlementCompleteTx = exchangeService.CompleteMergedDeliveryTx
	welfareModule.Service.SettlementCompleteTx = welfareModule.Service.CompleteMergedDeliveryTx
	welfareModule.Service.ExchangeSettlementReward = func(ctx context.Context, id int64) (welfare.SettlementReward, error) {
		content, recipient, account, _, err := exchangeService.SettlementReward(ctx, id)
		if err != nil {
			return welfare.SettlementReward{}, err
		}
		if content.ISKMinor < 0 {
			return welfare.SettlementReward{}, welfare.ErrSettlementUnsupported
		}
		out := welfare.SettlementReward{ISKMinor: content.ISKMinor, AccountID: account, RecipientID: recipient, RecipientIDs: []int64{recipient}, Items: []welfare.SettlementItemSummary{}}
		items, err := exchangeRewardItems(content)
		if err != nil {
			return welfare.SettlementReward{}, err
		}
		for _, item := range items {
			out.Items = append(out.Items, welfare.SettlementItemSummary{TypeID: item.TypeID, Quantity: item.Quantity})
		}
		return out, nil
	}
	deliveryJobs.Service = welfareModule.Service
	welfareModule.Service.EnqueueValuationTx = deliveryJobs.EnqueueValuationTx
	identityService.MergeParticipants = map[string]identity.MergeParticipant{
		"community":  communityService.MergeAccountTx,
		"welfare":    welfareModule.Service.MergeAccountTx,
		"attendance": attendanceModule.Service.MergeAccountTx,
		"exchange":   exchangeService.MergeAccountTx,
		"fittings":   fittingService.MergeAccountTx,
		"eve":        eve.MergeAccountTx,
		"sentry":     sentryService.MergeAccountTx,
	}
	var authorize module.Authorizer = func(_ string, _ http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			httpapi.Failure(w, r, 401, "unauthenticated", "请先登录")
		})
	}
	if slices.Contains(enabled, "identity") {
		authorize = identityHandler.Authorize
	}
	walletModule := walletHandler(identityService, accessService, reader, staticData, canReadCharacter)
	if syncService != nil {
		walletModule.Names = syncService.WalletNames
	}
	marketModule := market.Handler{Service: &market.Service{Pool: pool, Administrator: accessService.IsAdministrator, Resolve: staticData.ResolveTypeNames}, User: func(r *http.Request) string { return identity.Principal(r.Context()).UserID }}
	if syncService != nil {
		marketModule.Service.Prices = syncService.ESI().JitaPrices
	}
	if eveHandler.Contracts != nil {
		marketModule.Service.Contract = eveHandler.Contracts.ReadContract
	}
	if slices.Contains(enabled, "market") {
		welfareModule.Service.EstimateLoss = marketModule.Service.EstimateItems
		exchangeService.EstimateReward = marketModule.Service.EstimateItems
	}
	sentryHandler := sentry.Handler{Service: sentryService, User: func(r *http.Request) string { return identity.Principal(r.Context()).UserID }}
	loanService := &loan.Service{Pool: pool, Characters: func(ctx context.Context, user string) ([]loan.Character, error) {
		rows, err := identityService.ActiveCharacters(ctx, user)
		out := make([]loan.Character, 0, len(rows))
		for _, row := range rows {
			out = append(out, loan.Character{ID: row.ID, Name: row.Name})
		}
		return out, err
	}, IsAdministrator: accessService.IsAdministrator, LockAccounts: identityService.LockActiveAccounts,
		AccountForCharacter: func(ctx context.Context, characterID int64) (string, error) {
			// Resolve only an active binding. A historical or revoked character
			// must not receive a guarantee invitation or reserve responsibility.
			owner, err := identityService.UserForCharacter(ctx, characterID)
			if err != nil {
				return "", err
			}
			chars, err := identityService.ActiveCharacters(ctx, owner)
			if err != nil {
				return "", err
			}
			for _, character := range chars {
				if character.ID == characterID {
					return owner, nil
				}
			}
			return "", pgx.ErrNoRows
		},
		CreditEvidence: func(ctx context.Context, user string) (loan.CreditEvidence, error) {
			var out loan.CreditEvidence
			if !slices.Contains(enabled, "attendance") {
				return out, nil
			}
			pap, err := attendanceModule.Service.CreditPAP(ctx, user)
			if err != nil {
				return out, nil
			}
			// PAP is a capped activity signal, never a repayment proxy.
			out.PAPPoints = minInt(5, int(pap.Points/4))
			out.PAPAvailable = pap.Complete
			out.PAPSnapshotID = pap.SnapshotID
			out.EvidenceCutoff = pap.ObservedAt
			return out, nil
		},
	}
	loanService.CanManagePool = func(ctx context.Context, user, kind string, id int64) (bool, error) {
		if kind != "corporation" {
			return false, nil
		}
		c, err := reader.Corporation(ctx, id)
		if err != nil {
			return false, err
		}
		return accessService.Can(ctx, user, "corporation.loan", access.Corporation{ID: c.CorporationID, Name: c.CorporationName, AllianceID: c.AllianceID, CEOID: c.CEOID})
	}
	if eveHandler.Contracts != nil {
		loanService.Contracts = loan.ContractAdapter{
			ReadFunc: func(ctx context.Context, actor, kind string, owner, id int64) (loan.Contract, error) {
				c, err := eveHandler.Contracts.ReadContract(ctx, actor, kind, owner, id)
				if err != nil {
					return loan.Contract{}, err
				}
				items, _ := json.Marshal(c.Items)
				return loan.Contract{ID: c.ID, OwnerKind: c.OwnerKind, OwnerID: c.OwnerID, Type: c.Type, Status: c.Status, Price: c.Price, Reward: c.Reward, IssuerID: c.IssuerID, AssigneeID: c.AssigneeID, AcceptorID: c.AcceptorID, ForCorporation: c.ForCorporation, IssuerCorporationID: c.IssuerCorporationID, Items: items, ItemsReady: c.ItemsReady, Completed: c.Completed}, nil
			},
			ClaimFunc: eve.ClaimDeliveryTx,
		}
		loanService.ReadContractTx = func(ctx context.Context, tx pgx.Tx, actor, kind string, owner, id int64) (loan.Contract, error) {
			c, err := eveHandler.Contracts.DeliveryContractTx(ctx, tx, actor, kind, owner, id)
			if err != nil {
				return loan.Contract{}, err
			}
			items, _ := json.Marshal(c.Items)
			return loan.Contract{ID: c.ID, OwnerKind: c.OwnerKind, OwnerID: c.OwnerID, Type: c.Type, Status: c.Status, Price: c.Price, Reward: c.Reward, IssuerID: c.IssuerID, AssigneeID: c.AssigneeID, AcceptorID: c.AcceptorID, ForCorporation: c.ForCorporation, IssuerCorporationID: c.IssuerCorporationID, Items: items, ItemsReady: c.ItemsReady, Completed: c.Completed}, nil
		}
	}
	identityService.MergeParticipants["loan"] = loanService.MergeAccountTx
	loanHandler := loan.Handler{Service: loanService, User: func(r *http.Request) string { return identity.Principal(r.Context()).UserID }}
	registry, err := module.New([]module.Definition{system.Module(status), identityHandler.Module(), eveHandler.Module(), accessHandler.Module(), communityHandler.Module(), attendanceModule.Module(), exchangeModule.Module(), fittingModule.Module(), skillModule.Module(), welfareModule.Module(), walletModule.Module(), marketModule.Module(), sentryHandler.Module(), structureHandler.Module(), loanHandler.Module(), approvalHandler(enabled, identityService, welfareModule.Service, exchangeService, loanService).Module()}, enabled, authorize)
	if err != nil {
		return nil, err
	}
	application := &Application{Handler: httpapi.New(logger, registry, system.ReadyHandler(system.New(pool, version)), authorize)}
	if esiSync != nil {
		application.background = esiSync.Run
	}
	return application, nil
}

package app

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/modules/access"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/exchange"
	"glorynavy.local/seat/internal/modules/fittings"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/modules/skills"
	"glorynavy.local/seat/internal/modules/welfare"
	"net/http"
	"time"
)

func welfareHandler(pool *pgxpool.Pool, accounts *identity.Service, policy *access.Service, reader *eve.AuthorizationService, coins *exchange.Service, fits *fittings.Service, skill *skills.Service) welfare.Handler {
	if reader == nil {
		reader = eve.ReadAuthorization(pool)
	}
	s := &welfare.Service{Pool: pool, Administrator: policy.IsAdministrator, LockAccounts: accounts.LockActiveAccounts, Credit: coins.WelfareGrantTx}
	s.MainCharacterID = func(ctx context.Context, account string) (int64, error) {
		ids, err := accounts.MainCharacterIDs(ctx, []string{account})
		if err != nil {
			return 0, err
		}
		return ids[account], nil
	}
	s.MatchReward = exchange.MatchRewardDelivery
	s.IsShip = fittings.IsShipType
	s.LibraryReward = func(ctx context.Context, user string, corp, id, version int64) (*welfare.GrowthRewards, error) {
		physical, err := coins.WelfareReward(ctx, user, corp, id, version)
		if err != nil {
			return nil, welfare.ErrRule
		}
		raw, err := json.Marshal(physical)
		if err != nil {
			return nil, err
		}
		var out welfare.GrowthRewards
		err = json.Unmarshal(raw, &out)
		return &out, err
	}
	s.ShipGroup = fittings.ShipGroup
	s.GrowthFitting = func(ctx context.Context, user string, corp, id int64) (welfare.GrowthFitting, error) {
		f, e := fits.LibraryRead(ctx, user, id)
		if e != nil {
			return welfare.GrowthFitting{}, e
		}
		if f.CorporationID != corp {
			return welfare.GrowthFitting{}, welfare.ErrRule
		}
		body, e := json.Marshal(f.Fit)
		return welfare.GrowthFitting{ID: f.ID, Name: f.Name, ShipTypeID: f.Fit.ShipTypeID, Version: f.Version, Fit: body}, e
	}
	s.ValidateConfig = func(ctx context.Context, user string, corp int64, kind string, c welfare.Config) error {
		if kind == "srp" || kind == "solo" {
			return nil
		}
		if !c.Enabled {
			return nil
		}
		growth := map[string]int64{"growth_gila": 17715, "growth_ishtar": 12005, "growth_loki": 29990, "growth_absolution": 22448}
		if expected, ok := growth[kind]; ok && c.ShipTypeID != expected {
			return welfare.ErrRule
		}
		if c.FittingID > 0 {
			f, e := fits.LibraryRead(ctx, user, c.FittingID)
			if e != nil {
				return e
			}
			if f.CorporationID != corp || f.Fit.ShipTypeID != c.ShipTypeID {
				return welfare.ErrRule
			}
		}
		if c.SkillPlanID > 0 {
			plans, e := skill.Plans(ctx, user, corp)
			if e != nil {
				return e
			}
			found := false
			for _, p := range plans {
				if p.ID == c.SkillPlanID {
					found = true
				}
			}
			if !found {
				return welfare.ErrRule
			}
		}
		return nil
	}
	s.SearchShips = func(ctx context.Context, q string) ([]welfare.Ship, error) {
		rows, e := fits.SearchShips(ctx, q)
		out := []welfare.Ship{}
		for _, v := range rows {
			out = append(out, welfare.Ship{ID: v.ID, Name: v.Name})
		}
		return out, e
	}
	s.Characters = func(ctx context.Context, user string) ([]welfare.Character, error) {
		rows, e := accounts.ActiveCharacters(ctx, user)
		if e != nil {
			return nil, e
		}
		out := []welfare.Character{}
		for _, c := range rows {
			a, e := reader.Get(ctx, c.ID)
			if e != nil {
				return nil, e
			}
			corp := int64(0)
			if subtle.ConstantTimeCompare(a.OwnerHash, c.OwnerHash) == 1 && (a.State == "ready" || a.State == "retry") && a.ValidUntil.After(time.Now()) {
				corp = a.CorporationID
			}
			out = append(out, welfare.Character{ID: c.ID, Name: c.Name, AccountID: user, CorporationID: corp})
		}
		return out, nil
	}
	s.Scope = func(ctx context.Context, user string, corp int64, manage bool) (bool, error) {
		c, e := reader.Corporation(ctx, corp)
		if e != nil {
			return false, e
		}
		allowed, e := policy.Can(ctx, user, "corporation.welfare", access.Corporation{ID: corp, Name: c.CorporationName, AllianceID: c.AllianceID, CEOID: c.CEOID})
		if e != nil || allowed || manage {
			return allowed, e
		}
		rows, e := s.Characters(ctx, user)
		for _, r := range rows {
			if r.CorporationID == corp {
				return true, e
			}
		}
		return false, e
	}
	s.Corporations = func(ctx context.Context, user string) ([]welfare.Corporation, error) {
		ids, e := reader.ActivityCorporations(ctx)
		if e != nil {
			return nil, e
		}
		out := []welfare.Corporation{}
		for _, id := range ids {
			ok, e := s.Scope(ctx, user, id, false)
			if e != nil {
				return nil, e
			}
			if !ok {
				continue
			}
			c, e := reader.Corporation(ctx, id)
			if e != nil {
				return nil, e
			}
			manage, e := s.Scope(ctx, user, id, true)
			if e != nil {
				return nil, e
			}
			out = append(out, welfare.Corporation{ID: id, Name: c.CorporationName, Manage: manage})
		}
		return out, nil
	}
	s.Members = func(ctx context.Context, user string, corp int64) ([]welfare.Character, error) {
		ok, e := s.Scope(ctx, user, corp, true)
		if e != nil {
			return nil, e
		}
		if !ok {
			return nil, pgx.ErrNoRows
		}
		ids, e := reader.ActivityCharacters(ctx, corp)
		if e != nil {
			return nil, e
		}
		rows, e := accounts.Bindings(ctx, nil, ids)
		if e != nil {
			return nil, e
		}
		authorizations, e := reader.MemberAuthorizations(ctx, ids)
		if e != nil {
			return nil, e
		}
		out := []welfare.Character{}
		for _, c := range rows {
			a, ok := authorizations[c.ID]
			if ok && subtle.ConstantTimeCompare(a.OwnerHash, c.OwnerHash) == 1 && (a.State == "ready" || a.State == "retry") && a.ValidUntil.After(time.Now()) && a.CorporationID == corp {
				out = append(out, welfare.Character{ID: c.ID, Name: c.Name, AccountID: c.UserID, CorporationID: corp})
			}
		}
		accountIDs := make([]string, 0, len(out))
		seen := make(map[string]bool)
		for _, c := range out {
			if !seen[c.AccountID] {
				seen[c.AccountID] = true
				accountIDs = append(accountIDs, c.AccountID)
			}
		}
		mainNames, e := accounts.MainCharacterNames(ctx, accountIDs)
		if e != nil {
			return nil, e
		}
		for i := range out {
			out[i].MainCharacterName = mainNames[out[i].AccountID]
		}
		return out, nil
	}
	s.LockCharacter = func(ctx context.Context, tx pgx.Tx, user string, id int64) error {
		rows, e := accounts.Bindings(ctx, tx, []int64{id})
		if e != nil {
			return e
		}
		if len(rows) != 1 || rows[0].UserID != user {
			return pgx.ErrNoRows
		}
		return nil
	}
	s.GrowthCheck = func(ctx context.Context, user string, char, corp int64, c welfare.Config) (json.RawMessage, json.RawMessage, error) {
		if fits == nil || skill == nil || c.FittingID <= 0 {
			return nil, nil, welfare.ErrGrowthUnmet
		}
		f, err := fits.LibraryRead(ctx, user, c.FittingID)
		if err != nil {
			return nil, nil, err
		}
		if f.CorporationID != corp || f.Fit.ShipTypeID != c.ShipTypeID {
			return nil, nil, welfare.ErrRule
		}
		required, err := fittings.RequiredSkills(f.Fit)
		if err != nil {
			return nil, nil, err
		}
		base := []skills.Requirement{}
		for _, r := range required {
			base = append(base, skills.Requirement{ID: r.ID, Level: r.Level})
		}
		result, version, err := skill.CheckRequirements(ctx, user, char, corp, c.SkillPlanID, base)
		if err != nil {
			return nil, nil, err
		}
		fit, _ := json.Marshal(f)
		check, _ := json.Marshal(struct {
			skills.Result
			PlanVersion int64 `json:"plan_version,string"`
		}{result, version})
		return fit, check, nil
	}
	s.GuardGrowth = func(ctx context.Context, tx pgx.Tx, corp int64, c welfare.Config, fit, check json.RawMessage) error {
		var f struct {
			Version int64 `json:"version,string"`
		}
		var r struct {
			PlanVersion int64 `json:"plan_version,string"`
		}
		if json.Unmarshal(fit, &f) != nil || json.Unmarshal(check, &r) != nil {
			return welfare.ErrConflict
		}
		if err := fits.GuardLibraryVersion(ctx, tx, corp, c.FittingID, f.Version); err != nil {
			return err
		}
		if c.SkillPlanID > 0 {
			return skill.GuardPlanVersion(ctx, tx, corp, c.SkillPlanID, r.PlanVersion)
		}
		return nil
	}
	s.Evidence = func(ctx context.Context, user string, char int64, c welfare.Config) (json.RawMessage, json.RawMessage, error) {
		var fit, check json.RawMessage
		if c.FittingID > 0 {
			f, e := fits.LibraryRead(ctx, user, c.FittingID)
			if e != nil {
				return nil, nil, e
			}
			if f.Fit.ShipTypeID != c.ShipTypeID {
				return nil, nil, welfare.ErrRule
			}
			fit, _ = json.Marshal(f)
		}
		if c.SkillPlanID > 0 {
			report, e := skill.CheckPlan(ctx, user, c.SkillPlanID, "", false, 0)
			if e != nil {
				return nil, nil, e
			}
			for _, r := range report.Items {
				if r.Character.ID == char {
					check, _ = json.Marshal(r)
					break
				}
			}
			if check == nil {
				check = json.RawMessage(`{"state":"unknown"}`)
			}
		}
		return fit, check, nil
	}
	return welfare.Handler{Service: s, User: func(r *http.Request) string { return identity.Principal(r.Context()).UserID }}
}

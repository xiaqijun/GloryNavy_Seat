package welfare

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/market"
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
	"slices"
	"strconv"
	"strings"
	"time"
)

type Service struct {
	MatchReward      func(json.RawMessage, eve.DeliveryContract) (bool, error)
	PaymentContracts func(context.Context, pgx.Tx, string, int64, string, time.Time) ([]eve.DeliveryContract, error)
	PaymentBindings  func(context.Context, pgx.Tx, []int64) (map[int64]string, error)
	GrowthCheck      func(context.Context, string, int64, int64, Config) (json.RawMessage, json.RawMessage, error)
	GuardGrowth      func(context.Context, pgx.Tx, int64, Config, json.RawMessage, json.RawMessage) error
	ClaimDelivery    func(context.Context, pgx.Tx, int64, string, int64) error
	LibraryReward    func(context.Context, string, int64, int64, int64) (*GrowthRewards, error)
	GrowthFitting    func(context.Context, string, int64, int64) (GrowthFitting, error)
	SearchItems      func(context.Context, string) ([]eve.StaticTypeName, error)
	Names            interface {
		TypeNames(context.Context, []int64) (map[int64]eve.StaticTypeName, error)
		SolarSystemNames(context.Context, []int64) (map[int64]eve.StaticTypeName, error)
	}
	EstimateLoss        func(context.Context, []market.Item) (market.Appraisal, int64, error)
	EnqueueValuationTx  func(context.Context, pgx.Tx, int64) error
	PurchaseContract    func(context.Context, string, int64, int64) (eve.DeliveryContract, error)
	Contracts           func(context.Context, string, int64, int64, int64, time.Time) ([]eve.DeliveryContract, error)
	Contract            func(context.Context, pgx.Tx, string, string, int64, int64) (eve.DeliveryContract, error)
	AttendanceLosses    func(context.Context, int64, int64, []int64) (map[int64]int64, error)
	GuardAttendanceLoss func(context.Context, pgx.Tx, string, int64, int64, int64) (int64, error)
	Losses              func(context.Context, string, int64, int64, int64, int64) ([]Loss, error)
	LatestLoss          func(context.Context, string, int64, int64) (*Loss, error)
	GuardLoss           func(context.Context, pgx.Tx, string, int64, int64) error
	ValidateConfig      func(context.Context, string, int64, string, Config) error
	ShipGroup           func(int64) string
	SearchShips         func(context.Context, string) ([]Ship, error)
	IsShip              func(int64) bool
	Pool                *pgxpool.Pool
	Administrator       func(context.Context, string) (bool, error)
	MainCharacterID     func(context.Context, string) (int64, error)
	MainCharacterName   func(context.Context, string) (string, error)
	Corporations        func(context.Context, string) ([]Corporation, error)
	Characters          func(context.Context, string) ([]Character, error)
	Members             func(context.Context, string, int64) ([]Character, error)
	Scope               func(context.Context, string, int64, bool) (bool, error)
	// CompensationScope grants additional management access for loss cases
	// only. It never replaces Scope, so the account keeps ordinary member access.
	CompensationScope func(context.Context, string, int64) (bool, error)
	LockAccounts      func(context.Context, pgx.Tx, []string) error
	LockCharacter     func(context.Context, pgx.Tx, string, int64) error
	Evidence          func(context.Context, string, int64, Config) (json.RawMessage, json.RawMessage, error)
	Credit            func(context.Context, pgx.Tx, string, int64, int64, int64, string, string) error
	// ExchangeDelivery is injected by the host so batch settlement can reuse
	// exchange's single-order contract verifier without importing its store.
	ExchangeDelivery           func(context.Context, int64) error
	ExchangeSettlementComplete func(context.Context, int64) (bool, error)
	ExchangeSettlementAccount  func(context.Context, int64) (string, error)
	ExchangeSettlementEligible func(context.Context, int64) error
	// ExchangeSettlementReward returns the frozen ISK/items projection for an
	// exchange order. The host adapts the exchange module so welfare never
	// imports its private store.
	ExchangeSettlementReward     func(context.Context, int64) (SettlementReward, error)
	ExchangeSettlementCompleteTx func(context.Context, pgx.Tx, int64, eve.DeliveryContract, string) error
	SettlementCompleteTx         func(context.Context, pgx.Tx, int64, eve.DeliveryContract, string) error
}

// SettlementAccount returns the owning Seat account for batch grouping.
func (s *Service) SettlementAccount(ctx context.Context, id int64) (string, error) {
	row, err := store.Read(ctx, s.Pool, id)
	if err != nil {
		return "", err
	}
	return row.AccountID, nil
}

func hash(v any) string { h := sha256.Sum256(raw(v)); return hex.EncodeToString(h[:]) }
func (s *Service) allowed(ctx context.Context, actor string, corp int64, manage bool) error {
	if corp <= 0 {
		return ErrInvalid
	}
	ok, e := s.Scope(ctx, actor, corp, manage)
	if e != nil {
		return e
	}
	if !ok {
		return pgx.ErrNoRows
	}
	return nil
}
func (s *Service) allowedLoss(ctx context.Context, actor string, corp int64, manage bool) error {
	if corp <= 0 {
		return ErrInvalid
	}
	if manage && s.CompensationScope != nil {
		ok, err := s.CompensationScope(ctx, actor, corp)
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
	}
	return s.allowed(ctx, actor, corp, manage)
}
func (s *Service) allowedCase(ctx context.Context, actor string, corp int64, kind string, manage bool) error {
	if isLoss(kind) {
		return s.allowedLoss(ctx, actor, corp, manage)
	}
	return s.allowed(ctx, actor, corp, manage)
}
func (s *Service) admin(ctx context.Context, user string) error {
	ok, e := s.Administrator(ctx, user)
	if e != nil {
		return e
	}
	if !ok {
		return pgx.ErrNoRows
	}
	return nil
}
func (s *Service) member(ctx context.Context, actor string, corp int64, target string) error {
	if !validUUID(target) {
		return ErrInvalid
	}
	rows, e := s.Members(ctx, actor, corp)
	if e != nil {
		return e
	}
	for _, r := range rows {
		if r.AccountID == target {
			return nil
		}
	}
	return pgx.ErrNoRows
}
func (s *Service) Read(ctx context.Context, actor string, id int64) (Case, error) {
	c, e := store.Read(ctx, s.Pool, id)
	if e != nil {
		return c, e
	}
	if c.AccountID == actor {
		return c, nil
	}
	if e = s.allowedCase(ctx, actor, c.CorporationID, c.Kind, true); e != nil {
		return Case{}, e
	}
	if e = s.member(ctx, actor, c.CorporationID, c.AccountID); e != nil {
		return Case{}, e
	}
	return c, nil
}
func (s *Service) List(ctx context.Context, actor string, corp int64, all bool, kind string, before int64) ([]Case, error) {
	if all && (kind == "loss" || isLoss(kind)) {
		if e := s.allowedLoss(ctx, actor, corp, true); e != nil {
			return nil, e
		}
	} else if e := s.allowed(ctx, actor, corp, all); e != nil {
		return nil, e
	}
	owner := actor
	allowed := []string{}
	if all {
		owner = ""
		rows, e := s.Members(ctx, actor, corp)
		if e != nil {
			return nil, e
		}
		for _, r := range rows {
			if !slices.Contains(allowed, r.AccountID) {
				allowed = append(allowed, r.AccountID)
			}
		}
	}
	if before == 0 {
		before = 9223372036854775807
	}
	return store.List(ctx, s.Pool, corp, owner, kind, before, allowed)
}
func (s *Service) rule(ctx context.Context, corp int64, kind string) (Policy, Config, error) {
	rows, e := store.Policies(ctx, s.Pool, corp)
	if e != nil {
		return Policy{}, Config{}, e
	}
	for _, p := range rows {
		if p.Kind == kind {
			var c Config
			e = json.Unmarshal(p.Config, &c)
			return p, c, e
		}
	}
	return Policy{}, Config{}, ErrRule
}

type Quote struct {
	Token string      `json:"token"`
	Lines []GrantLine `json:"lines"`
	Total int64       `json:"total_minor"`
}

func (s *Service) Preview(ctx context.Context, actor string, c Command) (Quote, error) {
	var q Quote
	if e := s.admin(ctx, actor); e != nil {
		return q, e
	}
	if e := s.allowed(ctx, actor, c.CorporationID, true); e != nil {
		return q, e
	}
	if !validText(c.Note, 500) || len(c.Lines) == 0 || len(c.Lines) > 100 {
		return q, ErrInvalid
	}
	members, e := s.Members(ctx, actor, c.CorporationID)
	if e != nil {
		return q, e
	}
	available := map[string]bool{}
	for _, m := range members {
		available[m.AccountID] = true
	}
	q.Lines = slices.Clone(c.Lines)
	slices.SortFunc(q.Lines, func(a, b GrantLine) int { return strings.Compare(a.AccountID, b.AccountID) })
	seen := map[string]bool{}
	for _, line := range q.Lines {
		if !validUUID(line.AccountID) || !available[line.AccountID] || seen[line.AccountID] || line.Amount <= 0 || line.Amount > 1000000000000 {
			return q, ErrInvalid
		}
		seen[line.AccountID] = true
		q.Total += line.Amount
	}
	q.Token = hash(struct {
		Actor  string
		Corp   int64
		Reason string
		Lines  []GrantLine
	}{actor, c.CorporationID, c.Note, q.Lines})
	return q, nil
}
func (s *Service) Execute(ctx context.Context, actor string, c Command) (out json.RawMessage, err error) {
	if !validUUID(actor) || !validUUID(c.RequestKey) || !slices.Contains([]string{"configure", "profile", "apply", "resubmit", "approve", "information", "external", "reject", "cancel", "execute", "complete", "grant", "reverse", "void", "link_delivery", "appraise", "request_cancel", "approve_cancel", "reject_cancel", "release_coins"}, c.Action) {
		return nil, ErrInvalid
	}
	newAction := slices.Contains([]string{"configure", "profile", "apply", "grant"}, c.Action)
	// Preserve legacy cases; new applications currently exclude alliance SRP and ordinary capitals.
	if c.Action == "apply" && (c.Kind == "alliance" || c.Kind == "capital") {
		return nil, ErrInvalid
	}
	manualLoss := c.Action == "apply" && isLoss(c.Kind) && !c.Detail.SyncedLoss
	if (newAction && c.ID != 0) || (!newAction && c.ID <= 0) {
		return nil, ErrInvalid
	}
	if len([]rune(c.Note)) > 1000 {
		return nil, ErrInvalid
	}
	// Resolve immutable ownership before locks; re-read after acquiring the publication fence.
	// Fingerprint only the submitted command; server-resolved names can change independently.
	commandFingerprint := hash(c)
	if c.Action == "apply" && isActivity(c.Kind) {
		commandFingerprint = activityFingerprint(c)
		oldFP, old, replayErr := store.Replay(ctx, s.Pool, actor, c.RequestKey)
		if replayErr == nil {
			if oldFP != commandFingerprint {
				return nil, ErrConflict
			}
			return old, nil
		}
		if replayErr != pgx.ErrNoRows {
			return nil, replayErr
		}
	}
	var previous Case
	var d Detail
	owner := actor
	charID := int64(0)
	manage := c.Action != "request_cancel" && c.Action != "apply" && c.Action != "cancel" && c.Action != "resubmit"
	if c.ID > 0 {
		previous, err = s.Read(ctx, actor, c.ID)
		if err != nil {
			return nil, err
		}
		owner = previous.AccountID
		c.CorporationID = previous.CorporationID
		if err = json.Unmarshal(previous.Detail, &d); err != nil {
			return nil, err
		}
		if c.Action == "approve_cancel" || c.Action == "execute" || c.Action == "complete" || c.Action == "link_delivery" || c.Action == "release_coins" || c.Action == "approve" && automaticFulfillment(previous.Kind) {
			charID = d.CharacterID
		}
		if c.Action == "appraise" {
			manage = owner != actor
			charID = d.CharacterID
		}
	}
	if manualLoss || c.Action == "configure" || c.Action == "profile" || c.Action == "grant" || c.Action == "reverse" {
		if err = s.admin(ctx, actor); err != nil {
			return nil, err
		}
	}
	if c.Action == "configure" {
		if isActivity(c.Kind) {
			return nil, ErrInvalid
		}
		if isSuper(c.Kind) {
			c.Config.EffectiveAt = ""
		}
		c.Config, err = s.prepareGrowth(ctx, actor, c.CorporationID, c.Kind, c.Config)
		if err != nil {
			return nil, err
		}
		if s.ValidateConfig != nil {
			if err = s.ValidateConfig(ctx, actor, c.CorporationID, c.Kind, c.Config); err != nil {
				return nil, err
			}
		}
	}
	// Members may maintain their existing applications even if live ESI metadata is temporarily missing.
	if !(c.ID > 0 && !manage && owner == actor) {
		if err = s.allowedCase(ctx, actor, c.CorporationID, c.Kind, manage); err != nil {
			return nil, err
		}
	}
	accounts := []string{actor, owner}
	var quote Quote
	if c.Action == "grant" {
		quote, err = s.Preview(ctx, actor, c)
		if err != nil {
			return nil, err
		}
		if c.Token != quote.Token {
			return nil, ErrConflict
		}
		for _, l := range quote.Lines {
			accounts = append(accounts, l.AccountID)
		}
	}
	if c.Action == "profile" {
		owner = c.AccountID
		if err = s.member(ctx, actor, c.CorporationID, owner); err != nil {
			return nil, err
		}
		accounts = append(accounts, owner)
	}
	if c.Action == "apply" {
		if !validKind(c.Kind) || c.Kind == "grant" {
			return nil, ErrInvalid
		}
		if isActivity(c.Kind) {
			if err = validateActivityImages(c.Images); err != nil {
				return nil, err
			}
		}
		rows, e := s.Characters(ctx, actor)
		if e != nil {
			return nil, e
		}
		var ch Character
		var purchase *eve.DeliveryContract
		var purchasePrice int64
		if isSuper(c.Kind) {
			var evidence eve.DeliveryContract
			var hull int64
			ch, evidence, hull, purchasePrice, e = s.findCapitalPurchase(ctx, actor, c.CorporationID, c.Kind, c.Detail.ContractID, rows)
			if e != nil {
				return nil, e
			}
			purchase = &evidence
			c.Detail.CharacterID, c.Detail.ShipTypeID = ch.ID, hull
		}
		for _, v := range rows {
			if v.ID == c.Detail.CharacterID && v.CorporationID == c.CorporationID {
				ch = v
			}
		}
		if ch.ID == 0 {
			return nil, pgx.ErrNoRows
		}
		charID = ch.ID
		var p Policy
		var cfg Config
		if !isLoss(c.Kind) {
			p, cfg, e = s.rule(ctx, c.CorporationID, c.Kind)
			if e != nil {
				return nil, e
			}
			at, parseErr := time.Parse(time.RFC3339, cfg.EffectiveAt)
			if !cfg.Enabled || !isSuper(c.Kind) && !isActivity(c.Kind) && (parseErr != nil || time.Now().Before(at)) {
				return nil, ErrRule
			}
		}
		d = Detail{CharacterID: ch.ID, CharacterName: ch.Name, ShipTypeID: c.Detail.ShipTypeID, KillmailID: c.Detail.KillmailID, ContractID: c.Detail.ContractID, SupportingContractID: c.Detail.SupportingContractID, EventID: c.Detail.EventID, OccurredAt: c.Detail.OccurredAt, Description: strings.TrimSpace(c.Detail.Description), Evidence: strings.TrimSpace(c.Detail.Evidence), Alliance: c.Detail.Alliance, ActivityBatchKey: c.Detail.ActivityBatchKey, PolicyVersion: p.Version, Rule: cfg}
		if isActivity(c.Kind) {
			d.Rewards = cfg.Rewards
			d.ImageCount = len(c.Images)
			if err = validateGrowthRewards(d.Rewards); err != nil {
				return nil, ErrRule
			}
		}
		if purchase != nil {
			d = Detail{CharacterID: ch.ID, CharacterName: ch.Name, ShipTypeID: c.Detail.ShipTypeID, ContractID: purchase.ID, Purchase: purchase, BaseMinor: purchasePrice, PolicyVersion: p.Version, Rule: cfg}
		}
		if isLoss(c.Kind) && s.Losses != nil && d.KillmailID > 0 {
			losses, e := s.Losses(ctx, actor, c.CorporationID, charID, 0, d.KillmailID)
			if e != nil {
				return nil, e
			}
			if len(losses) == 1 {
				l := losses[0]
				if l.CharacterID != charID {
					return nil, ErrRule
				}
				d.ShipTypeID = l.ShipTypeID
				d.OccurredAt = l.At.Format(time.RFC3339)
				d.LossEvidence = &l
				d.SyncedLoss = true
			} else if c.Detail.SyncedLoss {
				return nil, ErrConflict
			}
		} else if c.Detail.SyncedLoss {
			return nil, ErrRule
		}
		// Growth applications use server-owned project, rewards and skill evidence.
		// Free-text explanations are optional; other benefit kinds still require them.
		if len([]rune(d.Description)) > 1000 || len([]rune(d.Evidence)) > 3000 ||
			(!isGrowth(c.Kind) && !isSuper(c.Kind) && !isActivity(c.Kind) && (!validText(d.Description, 1000) || !validText(d.Evidence, 3000))) {
			return nil, ErrInvalid
		}
		if isLoss(c.Kind) {
			lost, e := time.Parse(time.RFC3339, d.OccurredAt)
			if e != nil || lost.After(time.Now()) || d.KillmailID <= 0 || d.ShipTypeID <= 0 {
				return nil, ErrRule
			}
		}
		if isGrowth(c.Kind) {
			d.ShipTypeID = cfg.ShipTypeID
			if growthFittingID(c.Kind) > 0 {
				if cfg.RewardID > 0 {
					d.Rewards = cfg.Rewards
					err = validateGrowthRewards(d.Rewards)
				} else {
					d.Rewards, err = s.growthRewards(ctx, actor, c.CorporationID, cfg.Rewards, true)
				}
				if err != nil {
					return nil, err
				}
			}
		}
		if !isActivity(c.Kind) && (d.ShipTypeID <= 0 || (s.IsShip != nil && !s.IsShip(d.ShipTypeID))) {
			return nil, ErrInvalid
		}
		if s.ShipGroup != nil {
			g := s.ShipGroup(d.ShipTypeID)
			if (c.Kind == "titan" && g != "泰坦") || (c.Kind == "supercarrier" && g != "超级航母") || (c.Kind == "capital" && (g == "泰坦" || g == "超级航母")) {
				return nil, ErrInvalid
			}
		}
		if c.Kind == "alliance" && !slices.Contains([]string{"pending", "not_paid", "paid", "unknown"}, d.Alliance) {
			return nil, ErrInvalid
		}
		if isGrowth(c.Kind) {
			if s.GrowthCheck == nil {
				return nil, ErrGrowthUnmet
			}
			d.FittingEvidence, d.SkillEvidence, err = s.GrowthCheck(ctx, actor, charID, c.CorporationID, cfg)
			if err != nil {
				return nil, err
			}
		} else if s.Evidence != nil && (cfg.FittingID > 0 || cfg.SkillPlanID > 0) {
			d.FittingEvidence, d.SkillEvidence, err = s.Evidence(ctx, actor, charID, cfg)
			if err != nil {
				return nil, err
			}
		}
	}
	// A PVP application publishes its validated loss first. Market/contract
	// quotes may take seconds and are resolved by the welfare queue after commit.
	// Approval still requires a complete quote or an explicit manual override.
	var valuation *Valuation
	if c.Action == "apply" && c.Kind == "solo" {
		d.Valuation = &Valuation{Source: "market", State: "pending", Reason: "核价中", At: time.Now().UTC()}
		if d.ContractID > 0 {
			d.Valuation.Source = "purchase_contract"
		}
	}
	if c.Action == "appraise" {
		valuation = s.valuate(ctx, actor, d)
	}
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(context.Background())
	if charID > 0 && s.LockCharacter != nil {
		if e = s.LockCharacter(ctx, tx, owner, charID); e != nil {
			return nil, e
		}
	}
	if c.Action == "apply" && d.LossEvidence != nil && s.GuardLoss != nil {
		if e = s.GuardLoss(ctx, tx, actor, charID, d.KillmailID); e != nil {
			return nil, e
		}
	}
	if e = s.LockAccounts(ctx, tx, accounts); e != nil {
		return nil, e
	}
	if c.Action == "apply" && c.Kind == "srp" || c.Action == "approve" && previous.Kind == "srp" {
		if s.GuardAttendanceLoss == nil {
			return nil, ErrAttendanceRequired
		}
		event, e := s.GuardAttendanceLoss(ctx, tx, owner, c.CorporationID, d.CharacterID, d.KillmailID)
		if errors.Is(e, pgx.ErrNoRows) || e == nil && event <= 0 {
			return nil, ErrAttendanceRequired
		}
		if e != nil {
			return nil, e
		}
		if c.Action == "approve" && d.EventID != event {
			return nil, ErrAttendanceRequired
		}
		d.EventID = event
	}
	if c.Action == "apply" && isActivity(c.Kind) {
		e = store.LockActivity(ctx, tx)
	} else {
		e = store.Lock(ctx, tx)
	}
	if e != nil {
		return nil, e
	}
	// Recheck mutable privileges immediately before publication.
	if manage {
		kind := c.Kind
		if previous.Kind != "" {
			kind = previous.Kind
		}
		if e = s.allowedCase(ctx, actor, c.CorporationID, kind, true); e != nil {
			return nil, e
		}
	}
	if manualLoss || c.Action == "configure" || c.Action == "profile" || c.Action == "grant" || c.Action == "reverse" {
		if e = s.admin(ctx, actor); e != nil {
			return nil, e
		}
	}
	fp := hash(c)
	if c.Action == "configure" || c.Action == "apply" && (isSuper(c.Kind) || isActivity(c.Kind)) {
		fp = commandFingerprint
	}
	oldFP, old, e := store.Replay(ctx, tx, actor, c.RequestKey)
	if e == nil {
		if oldFP != fp {
			return nil, ErrConflict
		}
		return old, nil
	}
	if e != pgx.ErrNoRows {
		return nil, e
	}
	result := any(nil)
	auditID := c.ID
	switch c.Action {
	case "configure":
		if !validKind(c.Kind) || c.Kind == "grant" {
			return nil, ErrInvalid
		}
		if e = validateConfig(c.Kind, c.Config); e != nil {
			return nil, e
		}
		p, e := store.SavePolicy(ctx, tx, Policy{CorporationID: c.CorporationID, Kind: c.Kind, Version: c.Version, Config: raw(c.Config)})
		if e != nil {
			return nil, normalize(e)
		}
		result = p
	case "profile":
		old, e := store.Profile(ctx, tx, owner)
		if e != nil {
			return nil, e
		}
		m := c.Profile
		m.AccountID = owner
		if m.Version != old.Version || !validText(c.Note, 500) {
			return nil, ErrConflict
		}
		if m.History == nil {
			m.History = map[string]string{}
		}
		if m.Months == nil {
			m.Months = []string{}
		}
		for k, v := range m.History {
			if !(isGrowth(k) && validKind(k) || k == "supercarrier" || k == "titan") || !slices.Contains([]string{"unknown", "unused", "used"}, v) {
				return nil, ErrInvalid
			}
		}
		for k, v := range old.History {
			if v == "used" && m.History[k] != "used" {
				return nil, ErrConflict
			}
		}
		if len(m.Months) > 1200 {
			return nil, ErrInvalid
		}
		slices.Sort(m.Months)
		m.Months = slices.Compact(m.Months)
		for _, v := range m.Months {
			t, e := time.Parse("2006-01", v)
			if e != nil || t.After(time.Now()) {
				return nil, ErrInvalid
			}
		}
		for _, v := range old.Months {
			if !slices.Contains(m.Months, v) {
				held, e := store.Claimed(ctx, tx, "month:"+owner+":"+v)
				if e != nil {
					return nil, e
				}
				if held {
					return nil, ErrConflict
				}
			}
		}
		if e = store.SaveProfile(ctx, tx, m); e != nil {
			return nil, e
		}
		m.Version++
		result = m
	case "apply":
		if isSuper(c.Kind) {
			if e = s.guardCapitalPurchase(ctx, tx, actor, c.Kind, d); e != nil {
				return nil, e
			}
			occupied, err := store.CapitalOccupied(ctx, tx, actor, c.Kind, d.ContractID, 0)
			if err != nil {
				return nil, err
			}
			if occupied {
				return nil, ErrConflict
			}
		}
		if isGrowth(c.Kind) {
			occupied, err := store.GrowthOccupied(ctx, tx, actor, c.Kind, d.ShipTypeID, 0)
			if err != nil {
				return nil, err
			}
			if occupied {
				return nil, ErrGrowthClaimed
			}
			if !growthMet(d.SkillEvidence) {
				return nil, ErrGrowthUnmet
			}
			if s.GuardGrowth == nil {
				return nil, ErrGrowthUnmet
			}
			if err = s.GuardGrowth(ctx, tx, c.CorporationID, d.Rule, d.FittingEvidence, d.SkillEvidence); err != nil {
				return nil, ErrConflict
			}
		}
		// A rule edit while evidence was collected invalidates the submission snapshot.
		if !isLoss(c.Kind) {
			ps, e := store.Policies(ctx, tx, c.CorporationID)
			if e != nil {
				return nil, e
			}
			found := false
			for _, p := range ps {
				if p.Kind == c.Kind && p.Version == d.PolicyVersion {
					found = true
				}
			}
			if !found {
				return nil, ErrConflict
			}
		}
		if isActivity(c.Kind) && d.Rule.ActivityClaimLimit != nil {
			claims, e := store.ActivityClaimCount(ctx, tx, actor, c.CorporationID, d.CharacterID, c.Kind, d.ActivityBatchKey)
			if e != nil {
				return nil, e
			}
			if claims >= *d.Rule.ActivityClaimLimit {
				return nil, ErrActivityClaimed
			}
		}
		v, e := store.Save(ctx, tx, Case{AccountID: actor, CorporationID: c.CorporationID, Kind: c.Kind, State: "submitted", Detail: raw(d), Keys: []string{}})
		if e != nil {
			return nil, e
		}
		if c.Kind == "solo" && s.EnqueueValuationTx != nil {
			if e = s.EnqueueValuationTx(ctx, tx, v.ID); e != nil {
				return nil, e
			}
		}
		if isActivity(c.Kind) {
			for i, image := range c.Images {
				if e = store.SaveActivityImage(ctx, tx, v.ID, i+1, image.MIME, image.Content); e != nil {
					return nil, e
				}
			}
		}
		result = v
		auditID = v.ID
	case "grant":
		if s.Credit == nil {
			return nil, ErrRule
		}
		items := []Case{}
		for _, line := range quote.Lines {
			if e = s.member(ctx, actor, c.CorporationID, line.AccountID); e != nil {
				return nil, e
			}
			v, e := store.Save(ctx, tx, Case{AccountID: line.AccountID, CorporationID: c.CorporationID, Kind: "grant", State: "completed", Award: line.Amount, Detail: raw(Detail{Description: c.Note, Reviewer: actor}), Keys: []string{}})
			if e != nil {
				return nil, e
			}
			if e = s.Credit(ctx, tx, line.AccountID, v.ID, 0, line.Amount, c.RequestKey, c.Note); e != nil {
				return nil, e
			}
			// Each member receives only their own immutable grant snapshot in the detail history.
			if e = store.GrantAudit(ctx, tx, actor, v.ID, c.Note, raw(v)); e != nil {
				return nil, e
			}
			items = append(items, v)
		}
		result = items
	default:
		v, e := store.Read(ctx, tx, c.ID)
		if e != nil {
			return nil, e
		}
		if v.Version != c.Version || v.AccountID != owner {
			return nil, ErrConflict
		}
		if e = json.Unmarshal(v.Detail, &d); e != nil {
			return nil, e
		}
		// Legacy approvals may predate hull-level reservations. Recheck before
		// starting or recording delivery so they cannot bypass a prior receipt.
		if isGrowth(v.Kind) && (c.Action == "execute" || c.Action == "complete" || c.Action == "release_coins") {
			occupied, err := store.GrowthOccupied(ctx, tx, v.AccountID, v.Kind, d.ShipTypeID, v.ID)
			if err != nil {
				return nil, err
			}
			if occupied {
				return nil, ErrGrowthClaimed
			}
		}
		if c.Action == "cancel" || c.Action == "resubmit" || c.Action == "request_cancel" {
			if owner != actor {
				return nil, pgx.ErrNoRows
			}
		}
		if c.Action != "cancel" && !validText(c.Note, 1000) {
			return nil, ErrInvalid
		}
		switch c.Action {
		case "request_cancel", "approve_cancel", "reject_cancel":
			if e = s.cancelLoss(ctx, tx, actor, &v, &d, c); e != nil {
				return nil, e
			}
		case "appraise":
			if !pendingValuation(v) || valuation == nil {
				return nil, ErrConflict
			}
			d.Valuation = valuation
		case "resubmit":
			if v.State != "information" {
				return nil, ErrConflict
			}
			if !validText(c.Detail.Evidence, 3000) {
				return nil, ErrInvalid
			}
			d.Evidence = c.Detail.Evidence
			d.Description = c.Note
			v.State = "submitted"
		case "information", "external", "reject":
			if !slices.Contains([]string{"submitted", "information", "external"}, v.State) {
				return nil, ErrConflict
			}
			if c.Action == "external" && v.Kind != "alliance" {
				return nil, ErrInvalid
			}
			v.State = map[string]string{"information": "information", "external": "external", "reject": "rejected"}[c.Action]
			d.ReviewNote = c.Note
		case "cancel":
			if !slices.Contains([]string{"submitted", "information", "external"}, v.State) {
				return nil, ErrConflict
			}
			v.State = "cancelled"
		case "release_coins":
			if !isGrowth(v.Kind) || !coinOnly(d) || d.Delivery != nil || (v.State != "approved" && v.State != "executing") || actor == owner {
				return nil, ErrConflict
			}
			if e = s.prepareFulfillment(v.Kind, &d); e != nil {
				return nil, e
			}
			if e = s.startFulfillment(ctx, tx, &v, &d); e != nil {
				return nil, e
			}
		case "approve":
			if e = s.prepareFulfillment(v.Kind, &d); e != nil {
				return nil, e
			}
			if growthFittingID(v.Kind) > 0 && validateGrowthRewards(d.Rewards) != nil {
				return nil, ErrRule
			}
			if !slices.Contains([]string{"submitted", "information", "external"}, v.State) || v.Kind == "grant" {
				return nil, ErrConflict
			}
			if actor == owner {
				return nil, pgx.ErrNoRows
			}
			if v.Kind == "solo" {
				if e = approveValuation(&d, c); e != nil {
					return nil, e
				}
			}
			if isSuper(v.Kind) {
				if e = s.guardCapitalPurchase(ctx, tx, actor, v.Kind, d); e != nil {
					return nil, e
				}
				occupied, err := store.CapitalOccupied(ctx, tx, owner, v.Kind, d.ContractID, v.ID)
				if err != nil {
					return nil, err
				}
				if occupied {
					return nil, ErrConflict
				}
			} else {
				d.BaseMinor = c.Detail.BaseMinor
			}
			d.Discipline = c.Detail.Discipline
			d.Reviewer = actor
			d.ReviewNote = c.Note
			if isLoss(v.Kind) {
				if d.BaseMinor <= 0 || d.BaseMinor > 100000000000000 {
					return nil, ErrInvalid
				}
				v.Award = d.BaseMinor
				if cashLoss(v.Kind) {
					p, cfg, err := s.lossPolicy(ctx, tx, v.CorporationID, v.Kind)
					if err != nil {
						return nil, err
					}
					if c.LossPolicyVersion != nil && *c.LossPolicyVersion != p.Version {
						return nil, ErrConflict
					}
					d.Rule, d.PolicyVersion = cfg, p.Version
					v.Award, e = lossAward(d.BaseMinor, cfg)
					if e != nil {
						return nil, e
					}
					if e = s.checkLossQuota(ctx, tx, v, cfg); e != nil {
						return nil, e
					}
					d.PaymentStatus = "waiting_contract"
					if e = store.ScheduleDelivery(ctx, tx, v.ID, time.Now()); e != nil {
						return nil, e
					}
				}
			} else if isSuper(v.Kind) {
				v.Award, e = capitalAward(v.Kind, d.BaseMinor, d.Rule)
				if e != nil {
					return nil, e
				}
			} else if !isGrowth(v.Kind) && !isActivity(v.Kind) {
				v.Award, e = Calculate(v.Kind, d.BaseMinor, d.Discipline)
				if e != nil {
					return nil, e
				}
			}
			keys, e := s.qualify(ctx, tx, v, d)
			if e != nil {
				return nil, e
			}
			v.Keys = keys
			for _, key := range keys {
				if e = store.Claim(ctx, tx, key, v.ID); e != nil {
					return nil, normalize(e)
				}
			}
			v.State = "approved"
			if e = s.startFulfillment(ctx, tx, &v, &d); e != nil {
				return nil, e
			}
		case "void":
			if automaticFulfillment(v.Kind) {
				return nil, ErrDelivery
			}
			if v.State != "approved" || actor == owner {
				return nil, ErrConflict
			}
			if e = store.Release(ctx, tx, v.ID); e != nil {
				return nil, e
			}
			v.State = "cancelled"
			d.ReviewNote = c.Note
		case "execute":
			if automaticFulfillment(v.Kind) {
				return nil, ErrDelivery
			}
			if v.State != "approved" || v.Kind == "grant" {
				return nil, ErrConflict
			}
			if actor == owner {
				return nil, pgx.ErrNoRows
			}
			v.State = "executing"
			d.Executor = actor
			d.ReviewNote = c.Note
		case "complete":
			if growthFittingID(v.Kind) > 0 && validateGrowthRewards(d.Rewards) != nil {
				return nil, ErrRule
			}
			// Linked contracts must finish through verified ESI evidence, not a text receipt.
			if d.Delivery != nil || automaticFulfillment(v.Kind) {
				return nil, ErrDelivery
			}
			if v.State != "executing" || v.Kind == "grant" {
				return nil, ErrConflict
			}
			if actor == owner || !validText(c.Detail.Receipt, 1000) {
				return nil, ErrInvalid
			}
			d.Receipt = c.Detail.Receipt
			if growthFittingID(v.Kind) > 0 && d.Rewards != nil && d.Rewards.Coins > 0 {
				if s.Credit == nil {
					return nil, ErrRule
				}
				if e = s.Credit(ctx, tx, v.AccountID, v.ID, 0, d.Rewards.Coins, c.RequestKey, c.Note); e != nil {
					return nil, e
				}
			}
			v.State = "completed"
		case "link_delivery":
			if e = s.linkDelivery(ctx, tx, actor, &v, &d, c); e != nil {
				return nil, e
			}
		case "reverse":
			if v.Kind != "grant" || v.State != "completed" || s.Credit == nil {
				return nil, ErrConflict
			}
			if e = s.Credit(ctx, tx, v.AccountID, v.ID, v.Award, 0, c.RequestKey, c.Note); e != nil {
				return nil, e
			}
			v.State = "reversed"
			d.ReviewNote = c.Note
		default:
			return nil, ErrInvalid
		}
		v.Detail = raw(d)
		v, e = store.Save(ctx, tx, v)
		if e != nil {
			return nil, normalize(e)
		}
		result = v
	}
	if e = store.Audit(ctx, tx, actor, c.RequestKey, fp, c.Action, c.Note, auditID, result); e != nil {
		return nil, normalize(e)
	}
	out = raw(result)
	return out, tx.Commit(ctx)
}
func (s *Service) qualify(ctx context.Context, tx pgx.Tx, v Case, d Detail) ([]string, error) {
	keys := []string{}
	m, e := store.Profile(ctx, tx, v.AccountID)
	if e != nil {
		return nil, e
	}
	if isLoss(v.Kind) {
		keys = append(keys, "km:"+strconv.FormatInt(d.KillmailID, 10))
	}
	if isGrowth(v.Kind) {
		occupied, err := store.GrowthOccupied(ctx, tx, v.AccountID, v.Kind, d.ShipTypeID, v.ID)
		if err != nil {
			return nil, err
		}
		if occupied {
			return nil, ErrGrowthClaimed
		}
		if !growthMet(d.SkillEvidence) {
			return nil, ErrGrowthUnmet
		}
		keys = append(keys, "once:"+v.AccountID+":growth_ship_"+strconv.FormatInt(d.ShipTypeID, 10))
	}
	if v.Kind == "supercarrier" || v.Kind == "titan" {
		if m.History[v.Kind] == "used" {
			return nil, ErrRule
		}
		keys = append(keys, "once:"+v.AccountID+":"+v.Kind)
	}
	if v.Kind == "capital" {
		if !m.Verified {
			return nil, ErrRule
		}
		months := slices.Clone(m.Months)
		slices.Sort(months)
		for _, month := range months {
			k := "month:" + v.AccountID + ":" + month
			used, e := store.Claimed(ctx, tx, k)
			if e != nil {
				return nil, e
			}
			if !used {
				keys = append(keys, k)
			}
			if len(keys) == 3 {
				break
			}
		}
		if len(keys) != 3 {
			return nil, ErrRule
		}
	}
	if v.Kind == "capital" || v.Kind == "supercarrier" || v.Kind == "titan" {
		if d.ContractID <= 0 {
			return nil, ErrRule
		}
		keys = append(keys, fmt.Sprintf("purchase:%d:%d", d.ContractID, d.ShipTypeID))
	}
	for _, k := range keys {
		used, e := store.Claimed(ctx, tx, k)
		if e != nil {
			return nil, e
		}
		if used {
			return nil, ErrConflict
		}
	}
	return keys, nil
}
func normalize(e error) error {
	var p *pgconn.PgError
	if errors.As(e, &p) && (p.Code == "23505" || p.Code == "23514") {
		return ErrConflict
	}
	if e == pgx.ErrNoRows {
		return ErrConflict
	}
	return e
}

package exchange

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"glorynavy.local/seat/internal/modules/exchange/internal/store"
	"math"
	"strconv"
	"strings"
	"time"
)

var ErrInsufficientCoinsMinor = errors.New("insufficient Guoke coin balance")
var ErrRewardUnavailable = errors.New("reward unavailable")

type Reward struct {
	Pricing    store.RewardPricing `json:"pricing"`
	Content    *PhysicalReward     `json:"content,omitempty"`
	ID         int64               `json:"id,string"`
	TypeID     int64               `json:"type_id,string"`
	Name       string              `json:"name"`
	Quantity   int32               `json:"quantity"`
	Value      int64               `json:"isk_value"`
	CoinsMinor int64               `json:"coins_minor"`
	Stock      int32               `json:"stock"`
	Enabled    bool                `json:"enabled"`
	Version    int64               `json:"version,string"`
}
type Shop struct {
	Admin      bool     `json:"admin"`
	Rate       int64    `json:"isk_per_coin"`
	Version    int64    `json:"version,string"`
	Earned     int64    `json:"earned_minor"`
	Reserved   int64    `json:"reserved_minor"`
	Spent      int64    `json:"spent_minor"`
	Available  int64    `json:"available_minor"`
	Rewards    []Reward `json:"rewards"`
	NextCursor string   `json:"next_cursor"`
}

func rewardCost(value, rate int64) int64 {
	if rate <= 0 {
		return 0
	}
	v := value / rate
	if value%rate != 0 {
		v++
	}
	return v
}
func (s *Service) shopAdmin(ctx context.Context, user string) error {
	if s.Administrator == nil {
		return pgx.ErrNoRows
	}
	ok, err := s.Administrator(ctx, user)
	if err != nil {
		return err
	}
	if !ok {
		return pgx.ErrNoRows
	}
	return nil
}
func (s *Service) Shop(ctx context.Context, user string, after int64) (Shop, error) {
	return s.shopFor(ctx, user, user, after, false)
}
func (s *Service) shopFor(ctx context.Context, user, subject string, after int64, listedOnly bool) (Shop, error) {
	out := Shop{Rewards: []Reward{}}
	actor, err := uuid(subject)
	if err != nil {
		return out, err
	}
	if s.Administrator != nil {
		out.Admin, err = s.Administrator(ctx, user)
		if err != nil {
			return out, err
		}
	}
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return out, err
	}
	defer tx.Rollback(context.Background())
	q := store.New(tx)
	setting, err := q.ShopSettings(ctx)
	if err != nil {
		return out, err
	}
	out.Rate, out.Version = setting.IskPerCoin, setting.Version
	balance, err := q.ShopBalance(ctx, actor)
	if err != nil {
		return out, err
	}
	out.Earned, out.Reserved, out.Spent = balance.Earned, balance.Reserved, balance.Spent
	out.Available = balance.Earned - balance.Reserved - balance.Spent
	rows, err := q.ListRewards(ctx, store.ListRewardsParams{AfterID: after, Admin: out.Admin && !listedOnly})
	if err != nil {
		return out, err
	}
	if len(rows) > 30 {
		rows = rows[:30]
		out.NextCursor = strconv.FormatInt(rows[29].ID, 10)
	}
	// End the read transaction before resolving names through the injected service.
	if err = tx.Commit(ctx); err != nil {
		return out, err
	}
	ids := []int64{}
	for _, r := range rows {
		ids = append(ids, r.TypeID)
	}
	if s.Names == nil {
		return out, ErrUnavailable
	}
	names, err := s.Names.TypeNames(ctx, ids)
	if err != nil {
		return out, err
	}
	priceIDs := make([]int64, 0, len(rows))
	for _, r := range rows {
		priceIDs = append(priceIDs, r.ID)
	}
	prices, err := store.New(s.Pool).RewardPrices(ctx, priceIDs)
	if err != nil {
		return out, err
	}
	for _, r := range rows {
		var content PhysicalReward
		if err = json.Unmarshal(r.Content, &content); err != nil {
			return out, err
		}
		if err = s.presentPhysical(ctx, &content); err != nil {
			return out, err
		}
		name := r.Name
		if name == "" {
			name = names[r.TypeID].Name
		}
		out.Rewards = append(out.Rewards, Reward{Pricing: prices[r.ID], Content: &content, ID: r.ID, TypeID: r.TypeID, Name: name, Quantity: r.Quantity, Value: r.IskValue, CoinsMinor: rewardCost(r.IskValue*100, out.Rate), Stock: r.Stock, Enabled: r.Enabled, Version: r.Version})
	}
	return out, nil
}

type ShopEdit struct {
	AutomaticPricing bool   `json:"automatic_pricing"`
	ID               int64  `json:"id,string"`
	Version          int64  `json:"version,string"`
	RequestKey       string `json:"request_key"`
	Rate             int64  `json:"isk_per_coin"`
	TypeID           int64  `json:"type_id,string"`
	Quantity         int32  `json:"quantity"`
	Value            int64  `json:"isk_value"`
	Stock            int32  `json:"stock"`
	Enabled          bool   `json:"enabled"`
}

func shopFingerprint(kind string, v any) string {
	raw, _ := json.Marshal(struct {
		Kind  string
		Value any
	}{kind, v})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func shopReplay(ctx context.Context, q *store.Queries, actor, key pgtype.UUID, kind, fp string) (bool, error) {
	r, err := q.FindShopAudit(ctx, store.FindShopAuditParams{ActorID: actor, RequestKey: key})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if r.Kind != kind || r.Fingerprint != fp {
		return false, ErrConflict
	}
	return true, nil
}
func (s *Service) EditShop(ctx context.Context, user, kind string, c ShopEdit) error {
	if err := s.shopAdmin(ctx, user); err != nil {
		return err
	}
	actor, _ := uuid(user)
	key, err := uuid(c.RequestKey)
	if err != nil {
		return err
	}
	if kind == "rate" {
		if c.Version < 1 || c.Rate < 1 || c.Rate > 1000000000000 {
			return ErrInvalid
		}
	} else if kind == "reward" {
		if c.ID < 0 || c.ID > 0 && c.Version < 1 || c.TypeID < 0 || (c.TypeID == 0 && c.ID == 0) || c.Quantity < 1 || c.Quantity > 1000000 || c.Value < 1 || c.Value > 1000000000000 || c.Stock < 0 || c.Stock > 1000000 {
			return ErrInvalid
		}
		if s.Names == nil {
			return ErrUnavailable
		}
		names, e := s.Names.TypeNames(ctx, []int64{c.TypeID})
		if e != nil {
			return e
		}
		if c.TypeID > 0 && names[c.TypeID].Source != "sde" {
			return ErrInvalid
		}
	} else {
		return ErrInvalid
	}
	fp := shopFingerprint(kind, c)
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	q := store.New(tx)
	// Configuration changes serialize on the settings row, before reward locks.
	settings, err := q.LockShopSettings(ctx)
	if err != nil {
		return err
	}
	if done, e := shopReplay(ctx, q, actor, key, kind, fp); done || e != nil {
		return e
	}
	var previous any
	var current any
	if kind == "rate" {
		if settings.Version != c.Version {
			return ErrConflict
		}
		previous = settings
		if err = q.SetShopRate(ctx, c.Rate); err != nil {
			return err
		}
		current = c.Rate
	} else {
		if c.ID == 0 {
			r, e := q.SaveReward(ctx, store.SaveRewardParams{TypeID: c.TypeID, Quantity: c.Quantity, IskValue: c.Value, Stock: c.Stock, Enabled: c.Enabled})
			if e != nil {
				return e
			}
			current = r
			c.ID = r.ID
			if err = q.SeedRewardContent(ctx, r.ID); err != nil {
				return err
			}
		} else {
			old, e := q.LockReward(ctx, c.ID)
			if e != nil {
				return e
			}
			if old.Version != c.Version || old.TypeID != c.TypeID || old.Quantity != c.Quantity || old.Archived {
				return ErrConflict
			}
			previous = old
			r, e := q.UpdateReward(ctx, store.UpdateRewardParams{ID: c.ID, TypeID: c.TypeID, Quantity: c.Quantity, IskValue: c.Value, Stock: c.Stock, Enabled: c.Enabled})
			if e != nil {
				return e
			}
			current = r
		}
	}
	if kind == "reward" {
		if err = q.SetRewardPricing(ctx, c.ID, c.AutomaticPricing); err != nil {
			return err
		}
	}
	payload, _ := json.Marshal(map[string]any{"before": previous, "after": current})
	if err = q.ShopAudit(ctx, store.ShopAuditParams{ActorID: actor, RequestKey: key, Kind: kind, TargetID: c.ID, Fingerprint: fp, Payload: payload}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type ClaimReward struct {
	RewardID      int64  `json:"reward_id,string"`
	RewardVersion int64  `json:"reward_version,string"`
	RateVersion   int64  `json:"rate_version,string"`
	RecipientID   int64  `json:"recipient_id,string"`
	RequestKey    string `json:"request_key"`
}

func (s *Service) ClaimReward(ctx context.Context, user string, c ClaimReward) (int64, error) {
	actor, err := uuid(user)
	key, e := uuid(c.RequestKey)
	if err != nil || e != nil || c.RewardID < 1 || c.RecipientID < 1 || c.RewardVersion < 1 || c.RateVersion < 1 {
		return 0, ErrInvalid
	}
	fp := shopFingerprint("claim", c)
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(context.Background())
	q := store.New(tx)
	// Identity -> account -> settings -> reward. Replays don't require a still-bound recipient.
	if old, e := q.FindRedemption(ctx, store.FindRedemptionParams{AccountID: actor, RequestKey: key}); e == nil {
		if old.Fingerprint != fp {
			return 0, ErrConflict
		}
		return old.ID, nil
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return 0, e
	}
	if s.Bindings == nil {
		return 0, ErrUnavailable
	}
	bindings, err := s.Bindings(ctx, tx, []int64{c.RecipientID})
	if err != nil {
		return 0, err
	}
	var recipient *Binding
	for i := range bindings {
		if bindings[i].ID == c.RecipientID && bindings[i].UserID == user {
			recipient = &bindings[i]
		}
	}
	if recipient == nil {
		return 0, pgx.ErrNoRows
	}
	if err = q.LockAccount(ctx, user); err != nil {
		return 0, err
	}
	if old, e := q.FindRedemption(ctx, store.FindRedemptionParams{AccountID: actor, RequestKey: key}); e == nil {
		if old.Fingerprint != fp {
			return 0, ErrConflict
		}
		return old.ID, nil
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return 0, e
	}
	settings, err := q.ShareShopSettings(ctx)
	if err != nil {
		return 0, err
	}
	reward, err := q.LockReward(ctx, c.RewardID)
	if err != nil {
		return 0, err
	}
	if settings.Version != c.RateVersion || reward.Version != c.RewardVersion {
		return 0, ErrConflict
	}
	if settings.IskPerCoin <= 0 || !reward.Enabled || reward.Archived || reward.Stock < 1 {
		return 0, ErrRewardUnavailable
	}
	cost := rewardCost(reward.IskValue*100, settings.IskPerCoin)
	balance, err := q.ShopBalance(ctx, actor)
	if err != nil {
		return 0, err
	}
	if balance.Earned-balance.Reserved-balance.Spent < cost {
		return 0, ErrInsufficientCoinsMinor
	}
	order, err := q.CreateRedemption(ctx, store.CreateRedemptionParams{AccountID: actor, RequestKey: key, Fingerprint: fp, RewardID: reward.ID, TypeID: reward.TypeID, Quantity: reward.Quantity, RecipientID: recipient.ID, RecipientName: recipient.Name, IskPerCoin: settings.IskPerCoin, IskValue: reward.IskValue, CoinsMinor: cost})
	if err != nil {
		return 0, err
	}
	if err = q.ChangeRewardStock(ctx, store.ChangeRewardStockParams{ID: reward.ID, Delta: -1}); err != nil {
		return 0, err
	}
	if err = q.SnapshotRedemption(ctx, order.ID, store.Catalog{Name: reward.Name, Content: reward.Content, Version: reward.CatalogVersion}); err != nil {
		return 0, err
	}
	if err = q.CoinEntry(ctx, store.CoinEntryParams{AccountID: actor, Kind: "reserve", Reference: strconv.FormatInt(order.ID, 10), RequestKey: key, Delta: -cost, Reason: "奖励兑换"}); err != nil {
		return 0, err
	}
	if err = store.EnsureDelivery(ctx, tx, order.ID); err != nil {
		return 0, err
	}
	return order.ID, tx.Commit(ctx)
}

type OrderDecision struct {
	UndeliveredConfirmed bool   `json:"undelivered_confirmed,omitempty"`
	Version              int64  `json:"version,string"`
	RequestKey           string `json:"request_key"`
	State                string `json:"state"`
	Note                 string `json:"note"`
}

func (s *Service) DecideOrder(ctx context.Context, user string, id int64, c OrderDecision) error {
	if c.State == "fulfilled" {
		return ErrInvalid
	}
	c.Note = strings.TrimSpace(c.Note)
	actor, err := uuid(user)
	key, e := uuid(c.RequestKey)
	if err != nil || e != nil || c.Version < 1 || len([]rune(c.Note)) < 1 || len([]rune(c.Note)) > 200 || (c.State != "fulfilled" && c.State != "cancelled" && c.State != "cancel_requested" && c.State != "pending") {
		return ErrInvalid
	}
	row, err := store.New(s.Pool).Redemption(ctx, id)
	if err != nil {
		return err
	}
	if c.State == "cancel_requested" {
		if row.AccountID != actor {
			return pgx.ErrNoRows
		}
	} else {
		if row.AccountID == actor {
			return pgx.ErrNoRows
		}
		if err = s.shopAdmin(ctx, user); err != nil {
			return err
		}
	}
	if c.State == "cancelled" && !c.UndeliveredConfirmed {
		return ErrInvalid
	}
	originalOwner := row.AccountID
	fp := shopFingerprint("decision", struct {
		ID     int64
		Change OrderDecision
	}{id, c})
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	q := store.New(tx)
	if s.Bindings == nil || s.LockAccounts == nil {
		return ErrUnavailable
	}
	bindings, e := s.Bindings(ctx, tx, []int64{row.RecipientID})
	if e != nil {
		return e
	}
	if len(bindings) != 1 || bindings[0].UserID != row.AccountID.String() {
		return pgx.ErrNoRows
	}
	if err = s.LockAccounts(ctx, tx, []string{row.AccountID.String(), user}); err != nil {
		return err
	}
	if err = q.LockAccount(ctx, row.AccountID.String()); err != nil {
		return err
	}
	row, err = q.LockRedemption(ctx, id)
	if err != nil {
		return err
	}
	if done, e := shopReplay(ctx, q, actor, key, "decision", fp); done || e != nil {
		return e
	}
	if row.AccountID != originalOwner || row.Version != c.Version {
		return ErrConflict
	}
	switch c.State {
	case "cancel_requested":
		if row.State != "pending" {
			return ErrConflict
		}
	case "pending", "cancelled":
		if row.State != "cancel_requested" {
			return ErrConflict
		}
	default:
		if row.State != "pending" && row.State != "cancel_requested" {
			return ErrConflict
		}
	}
	if c.State != "cancel_requested" {
		if row.AccountID == actor {
			return pgx.ErrNoRows
		}
		if err := s.shopAdmin(ctx, user); err != nil {
			return err
		}
	}
	if c.State == "cancel_requested" && row.AccountID != actor {
		return ErrConflict
	}
	if c.State == "cancelled" {
		if err = s.guardCancellation(ctx, tx, row); err != nil {
			return err
		}
		current, e := q.LockReward(ctx, row.RewardID)
		if e != nil {
			return e
		}
		if current.CatalogVersion == row.CatalogVersion {
			if err = q.ChangeRewardStock(ctx, store.ChangeRewardStockParams{ID: row.RewardID, Delta: 1}); err != nil {
				return err
			}
		}
	}
	if c.State == "cancelled" {
		if err = q.CoinEntry(ctx, store.CoinEntryParams{AccountID: row.AccountID, Kind: "refund", Reference: strconv.FormatInt(id, 10), RequestKey: key, Delta: row.CoinsMinor, Reason: c.Note}); err != nil {
			return err
		}
	}
	if err = q.DecideRedemption(ctx, store.DecideRedemptionParams{ID: id, State: c.State, Note: c.Note, DecidedBy: actor}); err != nil {
		return err
	}
	payload, _ := json.Marshal(c)
	if err = q.ShopAudit(ctx, store.ShopAuditParams{ActorID: actor, RequestKey: key, Kind: "decision", TargetID: id, Fingerprint: fp, Payload: payload}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type RewardOrder struct {
	Reference        string          `json:"reference"`
	Delivery         store.Delivery  `json:"delivery"`
	CanRequestCancel bool            `json:"can_request_cancel"`
	Content          *PhysicalReward `json:"content,omitempty"`
	ID               int64           `json:"id,string"`
	Version          int64           `json:"version,string"`
	TypeID           int64           `json:"type_id,string"`
	Name             string          `json:"name"`
	Quantity         int32           `json:"quantity"`
	RecipientID      int64           `json:"recipient_id,string"`
	RecipientName    string          `json:"recipient_name"`
	CoinsMinor       int64           `json:"coins_minor"`
	Rate             int64           `json:"isk_per_coin"`
	Value            int64           `json:"isk_value"`
	State            string          `json:"state"`
	Note             string          `json:"note"`
	CreatedAt        time.Time       `json:"created_at"`
}
type RewardOrders struct {
	Items      []RewardOrder `json:"items"`
	NextCursor string        `json:"next_cursor"`
}

func (s *Service) RewardOrders(ctx context.Context, user string, all bool, before int64) (RewardOrders, error) {
	out := RewardOrders{Items: []RewardOrder{}}
	actor, err := uuid(user)
	if err != nil {
		return out, err
	}
	if all {
		if err = s.shopAdmin(ctx, user); err != nil {
			return out, err
		}
	}
	if before == 0 {
		before = math.MaxInt64
	}
	rows, err := store.New(s.Pool).ListRedemptions(ctx, store.ListRedemptionsParams{BeforeID: before, Admin: all, AccountID: actor})
	if err != nil {
		return out, err
	}
	if len(rows) > 30 {
		rows = rows[:30]
		out.NextCursor = strconv.FormatInt(rows[29].ID, 10)
	}
	ids := []int64{}
	for _, r := range rows {
		ids = append(ids, r.TypeID)
	}
	if s.Names == nil {
		return out, ErrUnavailable
	}
	names, err := s.Names.TypeNames(ctx, ids)
	if err != nil {
		return out, err
	}
	for _, r := range rows {
		var content PhysicalReward
		if err = json.Unmarshal(r.RewardContent, &content); err != nil {
			return out, err
		}
		if err = s.presentPhysical(ctx, &content); err != nil {
			return out, err
		}
		name := r.RewardName
		if name == "" {
			name = names[r.TypeID].Name
		}
		delivery, e := store.ReadDelivery(ctx, s.Pool, r.ID)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, RewardOrder{Reference: r.SettlementReference, Delivery: delivery, CanRequestCancel: r.AccountID == actor && r.State == "pending", Content: &content, ID: r.ID, Version: r.Version, TypeID: r.TypeID, Name: name, Quantity: r.Quantity, RecipientID: r.RecipientID, RecipientName: r.RecipientName, CoinsMinor: r.CoinsMinor, Rate: r.IskPerCoin, Value: r.IskValue, State: r.State, Note: r.Note, CreatedAt: r.CreatedAt.Time})
	}
	return out, nil
}

package exchange

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/exchange/internal/store"
)

type ConversionQuote struct {
	Token      string `json:"token"`
	Mode       string `json:"mode"`
	Points     int64  `json:"points"`
	Converted  int64  `json:"converted"`
	Pending    int64  `json:"pending"`
	CoinsMinor int64  `json:"coins_minor"`
	Characters int    `json:"characters"`
	UnitScale  int64  `json:"unit_scale"`
}

type coinPlan struct {
	Award                             Award
	Units, Rate, PreviousCoins, Coins int64
}

// Caller holds the source event lock. All wallet locks use the same sorted order
// as automatic issuance, corrections and redemption.
func (s *Service) prepareCoins(ctx context.Context, tx pgx.Tx, source string, awards []Award, manual bool) ([]coinPlan, ConversionQuote, error) {
	var quote ConversionQuote
	if _, ok := s.Sources[source]; !ok || len(awards) > 10000 {
		return nil, quote, ErrInvalid
	}
	scale := s.sourceScale(source)
	quote.UnitScale = scale
	q := store.New(tx)
	accounts := []string{}
	seen := map[string]bool{}
	for _, a := range awards {
		if _, err := uuid(a.AccountID); err != nil || a.Units < 0 || a.Units > 10000*scale || a.Previous < 0 || a.Reference == "" || seen[a.Reference] {
			return nil, quote, ErrInvalid
		}
		seen[a.Reference] = true
		if !slices.Contains(accounts, a.AccountID) {
			accounts = append(accounts, a.AccountID)
		}
	}
	slices.Sort(accounts)
	for _, a := range accounts {
		if err := q.LockAccount(ctx, a); err != nil {
			return nil, quote, err
		}
	}
	rate, err := q.ShareSourceRate(ctx, source)
	if err != nil {
		return nil, quote, err
	}
	quote.Mode = rate.ConversionMode
	plans := []coinPlan{}
	for _, a := range awards {
		old, err := q.SourceAward(ctx, store.SourceAwardParams{SourceID: source, Reference: a.Reference})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, quote, err
		}
		actor, _ := uuid(a.AccountID)
		if err == nil && old.AccountID != actor {
			return nil, quote, ErrConflict
		}
		converted, perUnit := old.Units, old.MinorPerUnit
		// Legacy zero-rate placeholders never represented a currency conversion.
		if perUnit == 0 {
			converted = 0
		}
		target := min(a.Units, max(int64(0), converted+min(int64(0), a.Units-a.Previous)))
		if manual {
			target = a.Units
		} else if s.AllowNew && rate.ConversionMode == "automatic" {
			// Switching to automatic does not convert the previously pending balance.
			target = min(a.Units, max(int64(0), converted+a.Units-a.Previous))
		}
		if target > converted && perUnit == 0 {
			perUnit = rate.MinorPerUnit
			if perUnit <= 0 {
				return nil, quote, ErrRateRequired
			}
		}
		if target > 0 && perUnit > 0 && target > 9223372036854775807/perUnit {
			return nil, quote, ErrInvalid
		}
		gross := target * perUnit
		coins := gross / scale
		p := coinPlan{a, target, perUnit, old.Coins, coins}
		plans = append(plans, p)
		quote.Points += a.Units
		quote.Converted += min(converted, a.Units)
		quote.Pending += a.Units - min(converted, a.Units)
		quote.CoinsMinor += p.Coins - old.Coins
		if target > converted {
			quote.Characters++
		}
	}
	if quote.CoinsMinor > 9007199254740991 {
		return nil, quote, ErrInvalid
	}
	quote.Token = shopFingerprint("conversion-preview", struct {
		Source string
		Rate   store.ExchangeSourceRate
		Plans  []coinPlan
	}{source, rate, plans})
	return plans, quote, nil
}

func saveCoins(ctx context.Context, tx pgx.Tx, source, key, reason string, plans []coinPlan) error {
	request, err := uuid(key)
	if err != nil {
		return err
	}
	q := store.New(tx)
	for _, p := range plans {
		actor, _ := uuid(p.Award.AccountID)
		if err = q.SaveSourceAward(ctx, store.SaveSourceAwardParams{SourceID: source, Reference: p.Award.Reference, AccountID: actor, Units: p.Units, MinorPerUnit: p.Rate, Coins: p.Coins}); err != nil {
			return err
		}
		if p.Coins == p.PreviousCoins {
			continue
		}
		if err = q.CoinEntry(ctx, store.CoinEntryParams{AccountID: actor, Kind: "source", Reference: source + ":" + p.Award.Reference, RequestKey: request, Delta: p.Coins - p.PreviousCoins, Reason: reason}); err != nil {
			return err
		}
	}
	return nil
}

// ConversionTx previews when key is empty. Publishing requires the exact preview
// token, an administrator, a reason and an idempotency key. No client-supplied units.
func (s *Service) ConversionTx(ctx context.Context, tx pgx.Tx, user, source, key, reason, expected string, awards []Award) (ConversionQuote, error) {
	var out ConversionQuote
	if err := s.shopAdmin(ctx, user); err != nil {
		return out, err
	}
	if !s.AllowNew {
		return out, ErrUnavailable
	}
	if key != "" && (len(strings.TrimSpace(reason)) == 0 || len([]rune(reason)) > 200 || len(expected) != 64) {
		return out, ErrInvalid
	}
	plans, out, err := s.prepareCoins(ctx, tx, source, awards, true)
	if err != nil {
		return out, err
	}
	if key == "" {
		return out, nil
	}
	actor, err := uuid(user)
	if err != nil {
		return out, err
	}
	request, err := uuid(key)
	if err != nil {
		return out, err
	}
	q := store.New(tx)
	fp := shopFingerprint("conversion", struct {
		Source, Reason, Expected string
		Awards                   []Award
	}{source, reason, expected, awards})
	if done, err := shopReplay(ctx, q, actor, request, "conversion", fp); done || err != nil {
		return out, err
	}
	if out.Token != expected || out.Pending == 0 || out.CoinsMinor <= 0 {
		return out, ErrConflict
	}
	if err = saveCoins(ctx, tx, source, key, reason, plans); err != nil {
		return out, err
	}
	payload, _ := json.Marshal(struct {
		Quote  ConversionQuote `json:"quote"`
		Reason string          `json:"reason"`
	}{out, reason})
	err = q.ShopAudit(ctx, store.ShopAuditParams{ActorID: actor, RequestKey: request, Kind: "conversion", Fingerprint: fp, Payload: payload})
	return out, err
}

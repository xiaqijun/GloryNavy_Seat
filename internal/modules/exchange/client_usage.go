package exchange

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/exchange/internal/store"
)

// cumulativeAlertCost carries the fraction prepaid by the previous ceiling.
// Credit can exceed one unit after merging accounts; no historical refund or
// new charge is needed until future consumption exhausts that existing credit.
func cumulativeAlertCost(seconds, price, unit, credit int64) (cost, nextCredit int64, err error) {
	if seconds <= 0 || price <= 0 || unit <= 0 || credit < 0 || seconds > math.MaxInt64/price {
		return 0, 0, ErrInvalid
	}
	numerator := seconds * price
	if numerator <= credit {
		return 0, credit - numerator, nil
	}
	numerator -= credit
	cost = numerator / unit
	if fraction := numerator % unit; fraction > 0 {
		cost++
		nextCredit = unit - fraction
	}
	return cost, nextCredit, nil
}

// SettleClientUsage commits one immutable remote usage ID and the cumulative
// rounding credit together. The legacy prepaid allowance methods remain intact.
func (s *Service) SettleClientUsage(ctx context.Context, c AlertTimeGrantRequest, interval AlertTimeIntervalRequest, settleKey string) error {
	if err := s.alertEnabled(); err != nil {
		return err
	}
	c.SystemID = strings.TrimSpace(c.SystemID)
	interval.IntervalID = strings.TrimSpace(interval.IntervalID)
	delta := interval.EndedAt.Sub(interval.StartedAt)
	if c.UnitSeconds <= 0 || c.UnitPriceMinor <= 0 || c.ReservedSeconds <= 0 || len(c.SystemID) > 120 || strings.TrimSpace(c.PriceVersion) == "" || interval.IntervalID == "" || len(interval.IntervalID) > 200 || delta <= 0 || delta%time.Second != 0 || int64(delta/time.Second) != c.ReservedSeconds || interval.GrantID != c.ID || c.ExpiresAt.IsZero() {
		return ErrInvalid
	}
	grantID, err := uuid(c.ID)
	if err != nil {
		return err
	}
	account, err := uuid(c.AccountID)
	if err != nil {
		return err
	}
	reserveKey, err := uuid(c.RequestKey)
	if err != nil {
		return err
	}
	intervalKey, err := uuid(interval.RequestKey)
	if err != nil {
		return err
	}
	settlementKey, err := uuid(settleKey)
	if err != nil {
		return err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if s.LockAccounts == nil {
		return ErrUnavailable
	}
	if err = s.LockAccounts(ctx, tx, []string{c.AccountID}); err != nil {
		return err
	}
	q := store.New(tx)
	if err = q.LockAccount(ctx, account.String()); err != nil {
		return err
	}
	if old, e := q.LockAlertGrant(ctx, grantID); e == nil {
		// A replay uses its original frozen grant, even after policy changes or
		// expiry. Resume old in-flight reservations without touching new credit.
		if old.AccountID != account || old.RequestKey != reserveKey || old.SystemID != c.SystemID || old.ReservedSeconds != c.ReservedSeconds {
			return ErrConflict
		}
		if charge, e := q.LockAlertCharge(ctx, grantID, interval.IntervalID); e == nil {
			if !charge.StartedAt.Equal(interval.StartedAt) || !charge.EndedAt.Equal(interval.EndedAt) || charge.DurationSeconds != c.ReservedSeconds {
				return ErrConflict
			}
			if charge.State == "settled" {
				return tx.Commit(ctx)
			}
		} else if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if err = s.ReserveAlertIntervalTx(ctx, tx, interval); err != nil {
			return err
		}
		if err = s.SettleAlertTimeTx(ctx, tx, c.ID, interval.IntervalID, settleKey); err != nil {
			return err
		}
		return tx.Commit(ctx)
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return e
	}
	covered, overlaps, err := q.LegacyAlertUsageCoverage(ctx, account, c.SystemID, interval.StartedAt, interval.EndedAt)
	if err != nil {
		return err
	}
	if covered {
		return tx.Commit(ctx)
	}
	if overlaps {
		return ErrConflict
	}
	credit, err := q.AlertRoundingCredit(ctx, account, c.SystemID, c.PriceVersion, c.UnitSeconds, c.UnitPriceMinor)
	if err != nil {
		return err
	}
	cost, nextCredit, err := cumulativeAlertCost(c.ReservedSeconds, c.UnitPriceMinor, c.UnitSeconds, credit)
	if err != nil {
		return err
	}
	if cost > 0 {
		balance, err := q.ShopBalance(ctx, account)
		if err != nil {
			return err
		}
		if balance.Earned-balance.Reserved-balance.Spent < cost {
			return ErrInsufficientCoinsMinor
		}
	}
	if err = q.CreateAlertGrant(ctx, grantID, account, reserveKey, c.PriceVersion, c.UnitSeconds, c.UnitPriceMinor, c.ReservedSeconds, cost, c.ExpiresAt.UTC(), c.SystemID); err != nil {
		return err
	}
	if err = q.MarkCumulativeAlertGrant(ctx, grantID); err != nil {
		return err
	}
	if err = q.CreateAlertCharge(ctx, grantID, account, interval.IntervalID, interval.StartedAt, interval.EndedAt, c.ReservedSeconds, cost, intervalKey, c.SystemID); err != nil {
		return err
	}
	charge, err := q.LockAlertCharge(ctx, grantID, interval.IntervalID)
	if err != nil {
		return err
	}
	if err = q.SettleAlertCharge(ctx, charge.ID, settlementKey); err != nil {
		return err
	}
	if err = q.AddAlertSettled(ctx, grantID, c.ReservedSeconds, cost); err != nil {
		return err
	}
	if err = q.CloseAlertGrant(ctx, grantID); err != nil {
		return err
	}
	if cost > 0 {
		if err = q.CoinEntry(ctx, store.CoinEntryParams{AccountID: account, Kind: "alert_reserve", Reference: c.ID, RequestKey: reserveKey, Delta: -cost, Reason: "预警时长额度预留"}); err != nil {
			return err
		}
	}
	if err = q.SetAlertRoundingCredit(ctx, account, c.SystemID, c.PriceVersion, c.UnitSeconds, c.UnitPriceMinor, nextCredit); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

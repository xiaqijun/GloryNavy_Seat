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

var (
	ErrAlertConsumptionDisabled = errors.New("alert consumption is disabled")
	ErrAlertExpired             = errors.New("alert time grant expired")
	ErrAlertQuotaExhausted      = errors.New("alert time grant exhausted")
	ErrAlertState               = errors.New("invalid alert consumption state")
)

// AlertTimeGrantRequest freezes one pricing policy for a prepaid allowance.
// UnitSeconds permits per-minute, per-hour, or per-second policies without
// putting a production default into the exchange module.
type AlertTimeGrantRequest struct {
	ID              string
	AccountID       string
	SystemID        string
	RequestKey      string
	PriceVersion    string
	UnitSeconds     int64
	UnitPriceMinor  int64
	ReservedSeconds int64
	ExpiresAt       time.Time
}

type AlertTimeIntervalRequest struct {
	GrantID    string
	IntervalID string
	RequestKey string
	StartedAt  time.Time
	EndedAt    time.Time
}

func ceilMulDiv(a, b, d int64) (int64, error) {
	if a <= 0 || b <= 0 || d <= 0 || a > math.MaxInt64/b {
		return 0, ErrInvalid
	}
	n := a * b
	if n > math.MaxInt64-(d-1) {
		return 0, ErrInvalid
	}
	return (n + d - 1) / d, nil
}

func (s *Service) alertEnabled() error {
	if !s.AllowAlertConsumption {
		return ErrAlertConsumptionDisabled
	}
	return nil
}

func (s *Service) ReserveAlertTime(ctx context.Context, c AlertTimeGrantRequest) error {
	if err := s.alertEnabled(); err != nil {
		return err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if err = s.ReserveAlertTimeTx(ctx, tx, c); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) ReserveAlertTimeTx(ctx context.Context, tx pgx.Tx, c AlertTimeGrantRequest) error {
	if err := s.alertEnabled(); err != nil {
		return err
	}
	expiresAt := c.ExpiresAt.UTC().Truncate(time.Microsecond)
	if c.UnitSeconds <= 0 || c.UnitPriceMinor <= 0 || c.ReservedSeconds <= 0 || expiresAt.IsZero() || !expiresAt.After(time.Now().UTC()) {
		return ErrInvalid
	}
	c.SystemID = strings.TrimSpace(c.SystemID)
	if len(c.SystemID) > 120 {
		return ErrInvalid
	}
	id, err := uuid(c.ID)
	if err != nil {
		return err
	}
	account, err := uuid(c.AccountID)
	if err != nil {
		return err
	}
	key, err := uuid(c.RequestKey)
	if err != nil {
		return err
	}
	reservedMinor, err := ceilMulDiv(c.ReservedSeconds, c.UnitPriceMinor, c.UnitSeconds)
	if err != nil {
		return err
	}
	q := store.New(tx)
	if err = q.LockAccount(ctx, c.AccountID); err != nil {
		return err
	}
	if old, e := q.FindAlertGrantByRequest(ctx, account, key); e == nil {
		if old.ID != id || old.SystemID != c.SystemID || old.PriceVersion != c.PriceVersion || old.UnitSeconds != c.UnitSeconds || old.UnitPriceMinor != c.UnitPriceMinor || old.ReservedSeconds != c.ReservedSeconds || old.ReservedMinor != reservedMinor || !old.ExpiresAt.Equal(expiresAt) {
			return ErrConflict
		}
		return nil
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return e
	}
	if _, e := q.AlertGrant(ctx, id); e == nil {
		return ErrConflict
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return e
	}
	balance, err := q.ShopBalance(ctx, account)
	if err != nil {
		return err
	}
	if balance.Earned-balance.Reserved-balance.Spent < reservedMinor {
		return ErrInsufficientCoinsMinor
	}
	if err = q.CreateAlertGrant(ctx, id, account, key, c.PriceVersion, c.UnitSeconds, c.UnitPriceMinor, c.ReservedSeconds, reservedMinor, expiresAt, c.SystemID); err != nil {
		return err
	}
	return q.CoinEntry(ctx, store.CoinEntryParams{AccountID: account, Kind: "alert_reserve", Reference: c.ID, RequestKey: key, Delta: -reservedMinor, Reason: "预警时长额度预留"})
}

func (s *Service) ReserveAlertInterval(ctx context.Context, c AlertTimeIntervalRequest) error {
	if err := s.alertEnabled(); err != nil {
		return err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if err = s.ReserveAlertIntervalTx(ctx, tx, c); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) ReserveAlertIntervalTx(ctx context.Context, tx pgx.Tx, c AlertTimeIntervalRequest) error {
	if err := s.alertEnabled(); err != nil {
		return err
	}
	intervalID := strings.TrimSpace(c.IntervalID)
	if intervalID == "" || len(intervalID) > 200 || c.EndedAt.Before(c.StartedAt) || c.EndedAt.Equal(c.StartedAt) {
		return ErrInvalid
	}
	grantID, err := uuid(c.GrantID)
	if err != nil {
		return err
	}
	key, err := uuid(c.RequestKey)
	if err != nil {
		return err
	}
	q := store.New(tx)
	preview, err := q.AlertGrant(ctx, grantID)
	if err != nil {
		return err
	}
	if err = q.LockAccount(ctx, preview.AccountID.String()); err != nil {
		return err
	}
	grant, err := q.LockAlertGrant(ctx, grantID)
	if err != nil {
		return err
	}
	if old, e := q.LockAlertCharge(ctx, grantID, intervalID); e == nil {
		if old.State == "reserved" || old.State == "settled" {
			return nil
		}
		return ErrAlertState
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return e
	}
	if grant.State != "active" || !grant.ExpiresAt.After(time.Now()) {
		return ErrAlertExpired
	}
	delta := c.EndedAt.Sub(c.StartedAt)
	if delta%time.Second != 0 {
		return ErrInvalid
	}
	seconds := int64(delta / time.Second)
	if seconds <= 0 {
		return ErrInvalid
	}
	overlaps, err := q.AlertIntervalOverlap(ctx, grantID, c.StartedAt, c.EndedAt)
	if err != nil {
		return err
	}
	if overlaps {
		return ErrConflict
	}
	reserved, err := q.CountAlertReserved(ctx, grantID)
	if err != nil {
		return err
	}
	if grant.SettledSeconds+grant.ReleasedSeconds+reserved+seconds > grant.ReservedSeconds {
		return ErrAlertQuotaExhausted
	}
	cost, err := ceilMulDiv(seconds, grant.UnitPriceMinor, grant.UnitSeconds)
	if err != nil {
		return err
	}
	if cost <= 0 || grant.SettledMinor+grant.ReleasedMinor+cost > grant.ReservedMinor {
		return ErrAlertQuotaExhausted
	}
	return q.CreateAlertCharge(ctx, grantID, grant.AccountID, intervalID, c.StartedAt, c.EndedAt, seconds, cost, key, grant.SystemID)
}

func (s *Service) SettleAlertTime(ctx context.Context, grantID, intervalID, requestKey string) error {
	if err := s.alertEnabled(); err != nil {
		return err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if err = s.SettleAlertTimeTx(ctx, tx, grantID, intervalID, requestKey); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) SettleAlertTimeTx(ctx context.Context, tx pgx.Tx, grantID, intervalID, requestKey string) error {
	if err := s.alertEnabled(); err != nil {
		return err
	}
	intervalID = strings.TrimSpace(intervalID)
	if intervalID == "" || len(intervalID) > 200 {
		return ErrInvalid
	}
	gid, err := uuid(grantID)
	if err != nil {
		return err
	}
	key, err := uuid(requestKey)
	if err != nil {
		return err
	}
	q := store.New(tx)
	preview, err := q.AlertGrant(ctx, gid)
	if err != nil {
		return err
	}
	if err = q.LockAccount(ctx, preview.AccountID.String()); err != nil {
		return err
	}
	grant, err := q.LockAlertGrant(ctx, gid)
	if err != nil {
		return err
	}
	charge, err := q.LockAlertCharge(ctx, gid, intervalID)
	if err != nil {
		return err
	}
	if charge.State == "settled" {
		return nil
	}
	if charge.State != "reserved" {
		return ErrAlertState
	}
	if grant.SettledMinor+grant.ReleasedMinor+charge.CoinsMinor > grant.ReservedMinor {
		return ErrAlertQuotaExhausted
	}
	if err = q.SettleAlertCharge(ctx, charge.ID, key); err != nil {
		return err
	}
	if err = q.AddAlertSettled(ctx, gid, charge.DurationSeconds, charge.CoinsMinor); err != nil {
		return err
	}
	// The grant reservation already removed this amount from available funds.
	// Settlement only moves it from reserved to spent in ShopBalance; writing a
	// second negative ledger entry would double-charge the account journal.
	return nil
}

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

// AlertGrantAccount returns the owning Seat account for an alert grant. The
// host uses this narrow read to authorize grant cancellation without exposing
// exchange's private store to the sentry module.
func (s *Service) AlertGrantAccount(ctx context.Context, grantID string) (string, error) {
	gid, err := uuid(grantID)
	if err != nil {
		return "", err
	}
	grant, err := store.New(s.Pool).AlertGrant(ctx, gid)
	if err != nil {
		return "", err
	}
	return grant.AccountID.String(), nil
}

func (s *Service) AlertGrantExpiresAt(ctx context.Context, grantID string) (time.Time, error) {
	gid, err := uuid(grantID)
	if err != nil {
		return time.Time{}, err
	}
	grant, err := store.New(s.Pool).AlertGrant(ctx, gid)
	if err != nil {
		return time.Time{}, err
	}
	return grant.ExpiresAt, nil
}

func (s *Service) ReserveAlertTimeTx(ctx context.Context, tx pgx.Tx, c AlertTimeGrantRequest) error {
	if err := s.alertEnabled(); err != nil {
		return err
	}
	expiresAt := c.ExpiresAt.UTC().Truncate(time.Microsecond)
	if c.UnitSeconds <= 0 || c.UnitPriceMinor <= 0 || c.ReservedSeconds <= 0 || expiresAt.IsZero() || !expiresAt.After(time.Now().UTC()) {
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
		if old.ID != id || old.PriceVersion != c.PriceVersion || old.UnitSeconds != c.UnitSeconds || old.UnitPriceMinor != c.UnitPriceMinor || old.ReservedSeconds != c.ReservedSeconds || old.ReservedMinor != reservedMinor || !old.ExpiresAt.Equal(expiresAt) {
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
	if err = q.CreateAlertGrant(ctx, id, account, key, c.PriceVersion, c.UnitSeconds, c.UnitPriceMinor, c.ReservedSeconds, reservedMinor, expiresAt); err != nil {
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
	return q.CreateAlertCharge(ctx, grantID, grant.AccountID, intervalID, c.StartedAt, c.EndedAt, seconds, cost, key)
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

func (s *Service) ReleaseAlertInterval(ctx context.Context, grantID, intervalID, requestKey string) error {
	if err := s.alertEnabled(); err != nil {
		return err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if err = s.ReleaseAlertIntervalTx(ctx, tx, grantID, intervalID, requestKey); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) ReleaseAlertIntervalTx(ctx context.Context, tx pgx.Tx, grantID, intervalID, requestKey string) error {
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
	if charge.State == "released" {
		return nil
	}
	if charge.State != "reserved" {
		return ErrAlertState
	}
	if err = q.ReleaseAlertCharge(ctx, charge.ID, key); err != nil {
		return err
	}
	if err = q.AddAlertReleased(ctx, gid, charge.DurationSeconds, charge.CoinsMinor); err != nil {
		return err
	}
	return q.CoinEntry(ctx, store.CoinEntryParams{AccountID: grant.AccountID, Kind: "alert_release", Reference: grantID + ":" + intervalID, RequestKey: key, Delta: charge.CoinsMinor, Reason: "预警未确认时长释放"})
}

func (s *Service) RefundAlertTime(ctx context.Context, grantID, intervalID, requestKey string) error {
	if err := s.alertEnabled(); err != nil {
		return err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if err = s.RefundAlertTimeTx(ctx, tx, grantID, intervalID, requestKey); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) RefundAlertTimeTx(ctx context.Context, tx pgx.Tx, grantID, intervalID, requestKey string) error {
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
	if charge.State == "refunded" {
		return nil
	}
	if charge.State != "settled" {
		return ErrAlertState
	}
	if err = q.RefundAlertCharge(ctx, charge.ID, key); err != nil {
		return err
	}
	if err = q.ReverseAlertSettled(ctx, gid, charge.DurationSeconds, charge.CoinsMinor); err != nil {
		return err
	}
	// A refund restores the prepaid grant's usable capacity. If the grant was
	// closed after releasing its unused remainder, reopen it while it is still
	// valid so the corrected interval can be reserved again.
	if grant.State == "closed" && grant.ExpiresAt.After(time.Now().UTC()) {
		if err = q.ReopenAlertGrant(ctx, gid); err != nil {
			return err
		}
	}
	return q.CoinEntry(ctx, store.CoinEntryParams{AccountID: grant.AccountID, Kind: "alert_refund", Reference: grantID + ":" + intervalID, RequestKey: key, Delta: charge.CoinsMinor, Reason: "预警无效时长退款"})
}

func (s *Service) ReleaseAlertGrant(ctx context.Context, grantID, requestKey string) error {
	if err := s.alertEnabled(); err != nil {
		return err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if err = s.ReleaseAlertGrantTx(ctx, tx, grantID, requestKey); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) ReleaseAlertGrantTx(ctx context.Context, tx pgx.Tx, grantID, requestKey string) error {
	if err := s.alertEnabled(); err != nil {
		return err
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
	if grant.State == "closed" {
		return nil
	}
	reserved, err := q.CountAlertReserved(ctx, gid)
	if err != nil {
		return err
	}
	if reserved != 0 {
		return ErrConflict
	}
	unusedSeconds := grant.ReservedSeconds - grant.SettledSeconds - grant.ReleasedSeconds
	unusedMinor := grant.ReservedMinor - grant.SettledMinor - grant.ReleasedMinor
	if unusedSeconds < 0 || unusedMinor < 0 {
		return ErrConflict
	}
	if unusedSeconds > 0 || unusedMinor > 0 {
		if err = q.AddAlertReleased(ctx, gid, unusedSeconds, unusedMinor); err != nil {
			return err
		}
		if err = q.CoinEntry(ctx, store.CoinEntryParams{AccountID: grant.AccountID, Kind: "alert_release", Reference: grantID, RequestKey: key, Delta: unusedMinor, Reason: "预警未使用时长释放"}); err != nil {
			return err
		}
	}
	return q.CloseAlertGrant(ctx, gid)
}

// Package exchange owns the site-wide Guoke coin wallet and rewards.
package exchange

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/exchange/internal/store"
	"glorynavy.local/seat/internal/modules/market"
	"time"
)

var ErrInvalid = errors.New("invalid exchange request")
var ErrConflict = errors.New("exchange version conflict")
var ErrUnavailable = errors.New("exchange unavailable")
var ErrRateRequired = errors.New("source conversion rate required")

type Binding struct {
	ID     int64  `json:"id,string"`
	Name   string `json:"name"`
	UserID string `json:"-"`
}
type Service struct {
	Contracts      func(context.Context, pgx.Tx, string, int64, string, time.Time) ([]eve.DeliveryContract, error)
	ClaimDelivery  func(context.Context, pgx.Tx, int64, string, int64) error
	LockAccounts   func(context.Context, pgx.Tx, []string) error
	EstimateReward func(context.Context, []market.Item) (market.Appraisal, int64, error)
	RewardFitting  func(context.Context, string, int64) (PhysicalFitting, error)
	Pool           *pgxpool.Pool
	Administrator  func(context.Context, string) (bool, error)
	MemberExists   func(context.Context, string) (bool, error)
	Bindings       func(context.Context, pgx.Tx, []int64) ([]Binding, error)
	Own            func(context.Context, string) ([]Binding, error)
	Names          interface {
		TypeNames(context.Context, []int64) (map[int64]eve.StaticTypeName, error)
	}
	SearchTypes func(context.Context, string) ([]eve.StaticTypeName, error)
	// Only the host registers delivered source modules; clients cannot invent sources.
	Sources map[string]string
	// SourceScales stores the number of persisted source units per displayed
	// unit. PAP uses one; fractional sources may use a finer scale.
	SourceScales map[string]int64
	AllowNew     bool
	// AllowAlertConsumption is deliberately disabled by the host until the
	// Sentry time-usage contract and production pricing have been accepted.
	AllowAlertConsumption bool
}

// SettlementEligible is the narrow host-facing check used by welfare batch
// orchestration. It accepts only orders still waiting for delivery checking.
func (s *Service) SettlementEligible(ctx context.Context, id int64) error {
	row, err := store.New(s.Pool).Redemption(ctx, id)
	if err != nil {
		return err
	}
	if row.State != "pending" && row.State != "cancel_requested" {
		return ErrConflict
	}
	return nil
}

// SettlementAccount returns the owning Seat account used to group batch
// settlement selections. Characters under the same account may be included
// together; records from different accounts must use separate batches.
func (s *Service) SettlementAccount(ctx context.Context, id int64) (string, error) {
	row, err := store.New(s.Pool).Redemption(ctx, id)
	if err != nil {
		return "", err
	}
	return row.AccountID.String(), nil
}

// SettlementComplete reports whether the order has actually been fulfilled.
// A successful contract scan can still leave an order pending while the
// recipient has not accepted the contract, so callers must not treat a nil
// CheckDelivery error as completion.
func (s *Service) SettlementComplete(ctx context.Context, id int64) (bool, error) {
	row, err := store.New(s.Pool).Redemption(ctx, id)
	if err != nil {
		return false, err
	}
	return row.State == "fulfilled", nil
}

// SettlementReward exposes only the frozen reward projection needed by the
// welfare host to build a single aggregate contract. It never reads the live
// catalog, so later reward edits cannot alter an existing settlement batch.
func (s *Service) SettlementReward(ctx context.Context, id int64) (PhysicalReward, int64, string, string, error) {
	row, err := store.New(s.Pool).Redemption(ctx, id)
	if err != nil {
		return PhysicalReward{}, 0, "", "", err
	}
	var reward PhysicalReward
	if err := json.Unmarshal(row.RewardContent, &reward); err != nil || !physicalValid(reward) {
		return PhysicalReward{}, 0, "", "", ErrConflict
	}
	return reward, row.RecipientID, row.AccountID.String(), row.RecipientName, nil
}

func (s *Service) sourceScale(source string) int64 {
	if scale := s.SourceScales[source]; scale > 0 {
		return scale
	}
	return 1
}

func uuid(raw string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if id.Scan(raw) != nil || !id.Valid {
		return id, ErrInvalid
	}
	return id, nil
}

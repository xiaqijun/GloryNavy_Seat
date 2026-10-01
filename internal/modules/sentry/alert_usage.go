package sentry

import (
	"context"
	"errors"
	"time"
)

var ErrAlertUsageUnavailable = errors.New("alert usage is unavailable")
var ErrAlertUsageInvalid = errors.New("invalid alert usage query")

// AlertUsageSummary is a read-only projection of exchange's member-facing
// financial facts. Coins are the accounting unit; seconds explain pricing.
type AlertUsageSummary struct {
	AvailableMinor     int64     `json:"available_minor"`
	AlertReservedMinor int64     `json:"alert_reserved_minor"`
	AlertSettledMinor  int64     `json:"alert_settled_minor"`
	AlertReleasedMinor int64     `json:"alert_released_minor"`
	AlertRefundedMinor int64     `json:"alert_refunded_minor"`
	AsOf               time.Time `json:"as_of"`
}

type AlertConsumption struct {
	ID              int64     `json:"id,string"`
	GrantID         string    `json:"grant_id"`
	IntervalID      string    `json:"interval_id"`
	StartedAt       time.Time `json:"started_at"`
	EndedAt         time.Time `json:"ended_at"`
	DurationSeconds int64     `json:"duration_seconds"`
	CoinsMinor      int64     `json:"coins_minor"`
	State           string    `json:"state"`
	UnitSeconds     int64     `json:"unit_seconds"`
	UnitPriceMinor  int64     `json:"unit_price_minor"`
	PriceVersion    string    `json:"price_version,omitempty"`
	ExpiresAt       time.Time `json:"expires_at"`
}

type AlertConsumptionPage struct {
	Items      []AlertConsumption `json:"items"`
	NextCursor string             `json:"next_cursor"`
	AsOf       time.Time          `json:"as_of"`
}

type AlertUsageReader interface {
	AlertUsage(context.Context, string) (AlertUsageSummary, error)
	AlertConsumptions(context.Context, string, int64, string, *time.Time, *time.Time, int) (AlertConsumptionPage, error)
}

func (s *Service) ReadAlertUsage(ctx context.Context, account string) (AlertUsageSummary, error) {
	if s.AlertUsageReader == nil {
		return AlertUsageSummary{}, ErrAlertUsageUnavailable
	}
	return s.AlertUsageReader.AlertUsage(ctx, account)
}

func (s *Service) ReadAlertConsumptions(ctx context.Context, account string, before int64, state string, from, to *time.Time, limit int) (AlertConsumptionPage, error) {
	if s.AlertUsageReader == nil {
		return AlertConsumptionPage{}, ErrAlertUsageUnavailable
	}
	return s.AlertUsageReader.AlertConsumptions(ctx, account, before, state, from, to, limit)
}

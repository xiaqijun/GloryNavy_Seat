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
	MonitorRewardMinor int64     `json:"monitor_reward_minor"`
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

// MonitorRewardRecord is a member-facing record for a monitoring interval
// that actually credited coins. Zero-priced remainder-only evidence stays in
// the audit table and is intentionally omitted from this payout list.
type MonitorRewardRecord struct {
	ContributionID  string    `json:"contribution_id"`
	StartedAt       time.Time `json:"started_at"`
	EndedAt         time.Time `json:"ended_at"`
	DurationSeconds int64     `json:"duration_seconds"`
	CoinsMinor      int64     `json:"coins_minor"`
	SystemName      string    `json:"system_name"`
}

type MonitorRewardPage struct {
	Items []MonitorRewardRecord `json:"items"`
	AsOf  time.Time             `json:"as_of"`
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

// ReadMonitorRewards returns the most recent credited monitoring intervals.
// The underlying evidence remains in sentry_monitor_rewards; this endpoint is
// only a concise member-facing payout projection.
func (s *Service) ReadMonitorRewards(ctx context.Context, account string, limit int) (MonitorRewardPage, error) {
	out := MonitorRewardPage{Items: make([]MonitorRewardRecord, 0)}
	id, err := uuidText(account)
	if err != nil {
		return out, err
	}
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	rows, err := s.Pool.Query(ctx, `
SELECT contribution_id,started_at,ended_at,duration_seconds,coins_minor,system_name
FROM sentry_monitor_rewards
WHERE account_id=$1 AND state='rewarded' AND coins_minor>0
ORDER BY ended_at DESC, contribution_id DESC
LIMIT $2`, id, limit)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var item MonitorRewardRecord
		if err := rows.Scan(&item.ContributionID, &item.StartedAt, &item.EndedAt, &item.DurationSeconds, &item.CoinsMinor, &item.SystemName); err != nil {
			return out, err
		}
		out.Items = append(out.Items, item)
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	out.AsOf = time.Now().UTC()
	return out, nil
}

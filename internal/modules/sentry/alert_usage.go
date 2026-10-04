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
	SystemID        string    `json:"system_id,omitempty"`
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
	ClientID        string    `json:"client_id,omitempty"`
	StartedAt       time.Time `json:"started_at"`
	EndedAt         time.Time `json:"ended_at"`
	DurationSeconds int64     `json:"duration_seconds"`
	CoinsMinor      int64     `json:"coins_minor"`
	SystemID        string    `json:"system_id,omitempty"`
	SystemName      string    `json:"system_name"`
}

func mergeMonitorRewards(items []MonitorRewardRecord) []MonitorRewardRecord {
	out := make([]MonitorRewardRecord, 0, len(items))
	for _, item := range items {
		if len(out) > 0 {
			newer := &out[len(out)-1]
			if newer.StartedAt.Equal(item.EndedAt) && newer.ClientID == item.ClientID && newer.SystemID == item.SystemID && newer.SystemName == item.SystemName {
				item.EndedAt = newer.EndedAt
				item.DurationSeconds += newer.DurationSeconds
				item.CoinsMinor += newer.CoinsMinor
				item.ContributionID = "merged:" + item.ContributionID
				out[len(out)-1] = item
				continue
			}
		}
		out = append(out, item)
	}
	positive := make([]MonitorRewardRecord, 0, len(out))
	for _, item := range out {
		if item.CoinsMinor > 0 {
			positive = append(positive, item)
		}
	}
	return positive
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
SELECT contribution_id,client_id,started_at,ended_at,duration_seconds,coins_minor,system_id,system_name
FROM sentry_monitor_rewards
WHERE account_id=$1 AND state='rewarded'
ORDER BY ended_at DESC, contribution_id DESC
LIMIT 10000`, id)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var item MonitorRewardRecord
		if err := rows.Scan(&item.ContributionID, &item.ClientID, &item.StartedAt, &item.EndedAt, &item.DurationSeconds, &item.CoinsMinor, &item.SystemID, &item.SystemName); err != nil {
			return out, err
		}
		out.Items = append(out.Items, item)
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	out.Items = mergeMonitorRewards(out.Items)
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
	}
	out.AsOf = time.Now().UTC()
	return out, nil
}

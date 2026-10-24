package sentry

import (
	"context"
	"time"
)

// ClientUsageSettlement is the host boundary for one raw usage ID. It keeps
// exchange's private schema and transaction details out of the Sentry worker.
type ClientUsageSettlement struct {
	UsageID, GrantID, AccountID, SystemID string
	ReserveKey, IntervalKey, SettleKey    string
	PriceVersion                          string
	UnitSeconds, UnitPriceMinor           int64
	DurationSeconds                       int64
	StartedAt, EndedAt, ExpiresAt         time.Time
}

type CumulativeAlertSettlement interface {
	SettleClientUsage(context.Context, ClientUsageSettlement) error
}

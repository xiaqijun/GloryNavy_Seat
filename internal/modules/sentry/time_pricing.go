package sentry

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// TimePricing exposes the two business prices and the administrator switch. Allowance bounds remain
// internal delivery controls, and each new alert grant freezes its own price.
type TimePricing struct {
	AlertHourlyPriceMinor    int64      `json:"alert_hourly_price_minor"`
	MonitorHourlyRewardMinor int64      `json:"monitor_hourly_reward_minor"`
	Version                  int64      `json:"version"`
	CanEdit                  bool       `json:"can_edit"`
	ChargingEnabled          bool       `json:"charging_enabled"`
	UpdatedAt                *time.Time `json:"updated_at,omitempty"`
}

type TimePricingEdit struct {
	AlertHourlyPriceMinor    int64 `json:"alert_hourly_price_minor"`
	MonitorHourlyRewardMinor int64 `json:"monitor_hourly_reward_minor"`
	Version                  int64 `json:"version"`
	ChargingEnabled          *bool `json:"charging_enabled,omitempty"`
}

func (s *Service) ReadTimePricing(ctx context.Context, user string) (TimePricing, error) {
	enabled, err := s.alertChargingEnabled(ctx)
	if err != nil {
		return TimePricing{}, err
	}
	out := TimePricing{ChargingEnabled: enabled}
	if s.Administrator != nil {
		out.CanEdit, err = s.Administrator(ctx, user)
		if err != nil {
			return out, err
		}
	}
	if s.Pool != nil {
		var stamp time.Time
		err = s.Pool.QueryRow(ctx, `SELECT alert_hourly_price_minor,monitor_hourly_reward_minor,version,effective_at FROM sentry_time_pricing ORDER BY version DESC LIMIT 1`).Scan(&out.AlertHourlyPriceMinor, &out.MonitorHourlyRewardMinor, &out.Version, &stamp)
		if err == nil {
			out.UpdatedAt = &stamp
			return out, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return out, err
		}
	}
	p, err := s.currentAlertPolicy(ctx)
	if err != nil {
		return out, err
	}
	if p.UnitSeconds > 0 && p.UnitPriceMinor > 0 && p.UnitPriceMinor <= 1000000000000 {
		out.AlertHourlyPriceMinor = (p.UnitPriceMinor*3600 + p.UnitSeconds - 1) / p.UnitSeconds
	}
	return out, nil
}

func (s *Service) EditTimePricing(ctx context.Context, user string, e TimePricingEdit) (TimePricing, error) {
	if s.Pool == nil || s.Administrator == nil {
		return TimePricing{}, pgx.ErrNoRows
	}
	ok, err := s.Administrator(ctx, user)
	if err != nil {
		return TimePricing{}, err
	}
	if !ok {
		return TimePricing{}, pgx.ErrNoRows
	}
	if e.Version < 0 || e.AlertHourlyPriceMinor < 1 || e.AlertHourlyPriceMinor > 1000000000000 || e.MonitorHourlyRewardMinor < 0 || e.MonitorHourlyRewardMinor > 1000000000000 {
		return TimePricing{}, ErrAlertPricingInvalid
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return TimePricing{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(62062)`); err != nil {
		return TimePricing{}, err
	}
	var version int64
	if err = tx.QueryRow(ctx, `SELECT coalesce(max(version),0) FROM sentry_time_pricing`).Scan(&version); err != nil {
		return TimePricing{}, err
	}
	if version != e.Version {
		return TimePricing{}, ErrAlertPricingConflict
	}
	p := s.alertPolicy()
	var old AlertGrantPolicy
	var ttl, oldVersion int64
	var currentEnabled bool
	err = tx.QueryRow(ctx, `SELECT price_version,unit_seconds,unit_price_minor,max_grant_seconds,grant_ttl_seconds,version,charging_enabled FROM sentry_alert_pricing WHERE id=1 FOR UPDATE`).Scan(&old.PriceVersion, &old.UnitSeconds, &old.UnitPriceMinor, &old.MaxGrantSeconds, &ttl, &oldVersion, &currentEnabled)
	if err == nil {
		old.GrantTTL = time.Duration(ttl) * time.Second
		p = old
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return TimePricing{}, err
	}
	enabled := currentEnabled
	if e.ChargingEnabled != nil {
		enabled = *e.ChargingEnabled
	}
	if p.MaxGrantSeconds <= 0 {
		p.MaxGrantSeconds = 3600
	}
	if p.GrantTTL < time.Minute {
		p.GrantTTL = time.Hour
	}
	p.PriceVersion = fmt.Sprintf("hourly-v%d", version+1)
	p.UnitSeconds = 3600
	p.UnitPriceMinor = e.AlertHourlyPriceMinor
	if !policyValid(p) {
		return TimePricing{}, ErrAlertPricingInvalid
	}
	var stamp time.Time
	if err = tx.QueryRow(ctx, `INSERT INTO sentry_time_pricing(version,alert_hourly_price_minor,monitor_hourly_reward_minor,updated_by) VALUES($1,$2,$3,$4) RETURNING effective_at`, version+1, e.AlertHourlyPriceMinor, e.MonitorHourlyRewardMinor, user).Scan(&stamp); err != nil {
		return TimePricing{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO sentry_alert_pricing(id,price_version,unit_seconds,unit_price_minor,max_grant_seconds,grant_ttl_seconds,version,charging_enabled,updated_by) VALUES(1,$1,3600,$2,$3,$4,$5,$6,$7) ON CONFLICT(id) DO UPDATE SET price_version=EXCLUDED.price_version,unit_seconds=3600,unit_price_minor=EXCLUDED.unit_price_minor,version=EXCLUDED.version,charging_enabled=EXCLUDED.charging_enabled,updated_by=EXCLUDED.updated_by,updated_at=now()`, p.PriceVersion, p.UnitPriceMinor, p.MaxGrantSeconds, int64(p.GrantTTL/time.Second), oldVersion+1, enabled, user)
	if err != nil {
		return TimePricing{}, err
	}
	before := pricingFromPolicy(old, oldVersion > 0, currentEnabled, true, oldVersion, time.Time{})
	after := pricingFromPolicy(p, true, enabled, true, oldVersion+1, stamp)
	if _, err = tx.Exec(ctx, `INSERT INTO sentry_alert_pricing_audit(actor_id,previous_version,version,before_snapshot,after_snapshot) VALUES($1,$2,$3,$4,$5)`, user, oldVersion, oldVersion+1, before, after); err != nil {
		return TimePricing{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return TimePricing{}, err
	}
	s.setAlertPolicy(p)
	return TimePricing{AlertHourlyPriceMinor: e.AlertHourlyPriceMinor, MonitorHourlyRewardMinor: e.MonitorHourlyRewardMinor, Version: version + 1, CanEdit: true, ChargingEnabled: enabled, UpdatedAt: &stamp}, nil
}

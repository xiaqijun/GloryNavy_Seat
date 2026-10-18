package app

import (
	"context"
	"fmt"
	"glorynavy.local/seat/internal/modules/access"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/identity"
	"glorynavy.local/seat/internal/modules/reminder"
	"glorynavy.local/seat/internal/modules/structures"
	"glorynavy.local/seat/internal/platform/remindermessage"
	"slices"
	"time"
)

type reminderSourceService interface {
	ReminderKeys(context.Context, string, string, time.Time) ([]string, error)
	ReminderDetails(context.Context, string, string, time.Time) ([]remindermessage.Item, error)
}

func registerReminderSources(s *reminder.Service, enabled []string, module string, source reminderSourceService, kinds ...string) {
	if !slices.Contains(enabled, module) {
		return
	}
	if s.Sources == nil {
		s.Sources = map[string]reminder.Source{}
	}
	if s.Details == nil {
		s.Details = map[string]reminder.DetailSource{}
	}
	for _, kind := range kinds {
		s.Sources[kind] = func(ctx context.Context, u string, since time.Time) ([]string, error) {
			return source.ReminderKeys(ctx, u, kind, since)
		}
		s.Details[kind] = func(ctx context.Context, u string, since time.Time) ([]remindermessage.Item, error) {
			return source.ReminderDetails(ctx, u, kind, since)
		}
	}
}
func wireEveReminders(s *reminder.Service, accounts *identity.Service, policy *access.Service, auth *eve.AuthorizationService, contracts *eve.ContractHTTP, buildings structures.Handler, enabled []string) {
	wireEveReminderDetails(s, accounts, policy, auth, contracts, buildings, enabled)
	if s.Sources == nil {
		s.Sources = map[string]reminder.Source{}
	}
	for _, kind := range []string{"authorization_expired", "contract_pending"} {
		s.Sources[kind] = func(ctx context.Context, u string, _ time.Time) ([]string, error) {
			chars, err := accounts.ActiveCharacters(ctx, u)
			if err != nil {
				return nil, err
			}
			out := []string{}
			for _, ch := range chars {
				var keys []string
				if kind == "authorization_expired" {
					keys, err = auth.AuthorizationReminderKeys(ctx, ch.ID, ch.OwnerHash)
				} else {
					keys, err = contracts.ContractReminderKeys(ctx, u, ch.ID, ch.OwnerHash)
				}
				if err != nil {
					return nil, err
				}
				out = append(out, keys...)
			}
			return out, nil
		}
	}
	s.Sources["sync_anomaly"] = func(ctx context.Context, u string, _ time.Time) ([]string, error) {
		ok, err := policy.IsAdministrator(ctx, u)
		if err != nil {
			return nil, err
		}
		if !ok {
			return []string{}, nil
		}
		return auth.SyncReminderKeys(ctx)
	}
	if slices.Contains(enabled, "structures") {
		s.Sources["structure_alert"] = func(ctx context.Context, u string, _ time.Time) ([]string, error) {
			rows, err := buildings.Read(ctx, u, 0)
			if err != nil {
				return nil, err
			}
			out := []string{}
			now := time.Now().UTC()
			for _, row := range rows {
				if row.FuelExpires != nil {
					left := row.FuelExpires.Sub(now)
					stage := ""
					if left <= 0 {
						stage = "empty"
					} else if left <= 24*time.Hour {
						stage = "24h"
					} else if left <= 72*time.Hour {
						stage = "72h"
					}
					if stage != "" {
						out = append(out, fmt.Sprintf("%d:fuel:%s:%s", row.ID, row.FuelExpires.UTC().Format("2006-01-02"), stage))
					}
				}
				if row.UnanchorsAt != nil && row.UnanchorsAt.After(now) {
					out = append(out, fmt.Sprintf("%d:unanchor:%s", row.ID, row.UnanchorsAt.UTC().Format(time.RFC3339)))
				}
			}
			return out, nil
		}
	}
}

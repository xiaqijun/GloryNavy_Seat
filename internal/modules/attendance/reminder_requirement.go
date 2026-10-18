package attendance

import (
	"context"
	"errors"
	"time"
)

// Require a complete fresh current-month snapshot; unknown is not zero PAP.
func (s *Service) RequirementReminderKeys(ctx context.Context, account string, _ time.Time) ([]string, error) {
	now := time.Now().UTC()
	last := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	if last.Sub(now) > 7*24*time.Hour {
		return []string{}, nil
	}
	rule, err := s.PAPRequirement(ctx, account)
	if err != nil {
		return nil, err
	}
	if rule.MonthlyPoints <= 0 {
		return []string{}, nil
	}
	report, err := s.AlliancePAPReport(ctx, account)
	if err != nil {
		return nil, err
	}
	return requirementReminderKeys(report, rule.MonthlyPoints, now)
}
func requirementReminderKeys(report AlliancePAPReport, points int32, now time.Time) ([]string, error) {
	last := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	if last.Sub(now) > 7*24*time.Hour || points <= 0 {
		return []string{}, nil
	}
	if !report.Available || !report.Complete || report.Month != now.UTC().Format("2006-01") || report.LastSyncedAt == nil || now.Sub(*report.LastSyncedAt) > 24*time.Hour {
		return nil, errors.New("PAP observation unavailable")
	}
	if len(report.Characters) > 0 && report.Points < float64(points) {
		return []string{report.Month}, nil
	}
	return []string{}, nil
}

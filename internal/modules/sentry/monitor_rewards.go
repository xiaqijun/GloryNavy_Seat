package sentry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	platformjobs "glorynavy.local/seat/internal/platform/jobs"
)

type monitorRewardArgs struct{}

func (monitorRewardArgs) Kind() string { return "sentry.monitor-reconcile.v1" }

type monitorRewardWorker struct {
	river.WorkerDefaults[monitorRewardArgs]
	runtime *Service
}

var errMonitorRewardUnpriced = errors.New("monitor reward price is not configured")

func monitorRewardExtension(s *Service, enabled bool) platformjobs.Extension {
	return platformjobs.Extension{
		Register: func(workers *river.Workers) []*river.PeriodicJob {
			river.AddWorker(workers, &monitorRewardWorker{runtime: s})
			if !enabled {
				return nil
			}
			return []*river.PeriodicJob{river.NewPeriodicJob(river.PeriodicInterval(time.Minute), func() (river.JobArgs, *river.InsertOpts) {
				return monitorRewardArgs{}, &river.InsertOpts{Queue: "exchange", UniqueOpts: platformjobs.ActiveUnique(), MaxAttempts: 5}
			}, &river.PeriodicJobOpts{RunOnStart: true})}
		},
		Bind: func(_ *river.Client[pgx.Tx]) {},
	}
}

func (w *monitorRewardWorker) Work(ctx context.Context, _ *river.Job[monitorRewardArgs]) error {
	s := w.runtime
	if s == nil || s.MonitorRemote == nil || s.MonitorRewardFunding == nil || s.Pool == nil {
		return river.JobSnooze(time.Hour)
	}
	cursor, err := s.monitorCursor(ctx)
	if err != nil {
		return err
	}
	page, err := s.MonitorRemote.ListMonitorContributions(ctx, cursor, 100)
	if err != nil {
		return err
	}
	next := monitorContributionPageCursor(page)
	if err := s.settleMonitorContributions(ctx, page.Contributions, cursor, next); errors.Is(err, errMonitorRewardUnpriced) {
		return river.JobSnooze(time.Hour)
	} else if err != nil {
		return err
	}
	if len(page.Contributions) > 0 {
		if err := s.saveMonitorCursor(ctx, next); err != nil {
			return err
		}
	}
	if page.HasMore {
		return river.JobSnooze(time.Second)
	}
	return river.JobSnooze(time.Minute)
}

func (s *Service) monitorCursor(ctx context.Context) (string, error) {
	var c string
	err := s.Pool.QueryRow(ctx, `SELECT cursor FROM sentry_monitor_reconcile_state WHERE id=1`).Scan(&c)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return c, err
}
func (s *Service) saveMonitorCursor(ctx context.Context, c string) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO sentry_monitor_reconcile_state(id,cursor,updated_at) VALUES(1,$1,now()) ON CONFLICT(id) DO UPDATE SET cursor=EXCLUDED.cursor,updated_at=EXCLUDED.updated_at`, c)
	return err
}

type monitorPolicyRow struct {
	Reward    int64
	Effective time.Time
	Version   int64
}

func monitorContributionPageCursor(page MonitorContributionPage) string {
	next := strings.TrimSpace(page.NextCursor)
	if next == "" && len(page.Contributions) > 0 {
		last := page.Contributions[len(page.Contributions)-1]
		next = last.CreatedAt + "|" + last.ContributionID
	}
	return next
}

func (s *Service) settleMonitorContribution(ctx context.Context, c MonitorContribution) error {
	return s.settleMonitorContributions(ctx, []MonitorContribution{c}, "", c.ContributionID)
}

type monitorPayout struct {
	accountID string
	amount    int64
}

// monitorSystemID normalizes the remote system identifier into the stable
// string used by the Seat ledger. Older Sentry responses only carried a
// display name; those records remain attributable under a legacy namespace
// until the remote side begins sending system_id.
func systemIDValue(value any) string {
	switch v := value.(type) {
	case string:
		if id := strings.TrimSpace(v); id != "" {
			return id
		}
	case float64:
		if v > 0 && v == math.Trunc(v) {
			return strconv.FormatInt(int64(v), 10)
		}
	case json.Number:
		if id := strings.TrimSpace(v.String()); id != "" {
			return id
		}
	case int64:
		if v > 0 {
			return strconv.FormatInt(v, 10)
		}
	case int:
		if v > 0 {
			return strconv.Itoa(v)
		}
	}
	return ""
}

func monitorSystemID(c MonitorContribution) string {
	if id := systemIDValue(c.SystemID); id != "" {
		return id
	}
	name := strings.ToLower(strings.TrimSpace(c.SystemName))
	if name == "" {
		return "legacy:unknown"
	}
	return "legacy:" + name
}

// settleMonitorContributions records every interval but combines all rewards
// from one reconciliation page into one wallet credit per account. This keeps
// evidence granular while avoiding one coin-ledger row for every heartbeat.
func (s *Service) settleMonitorContributions(ctx context.Context, contributions []MonitorContribution, cursor, next string) error {
	if len(contributions) == 0 {
		return nil
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	remainders := make(map[string]int64)
	payouts := make(map[string]int64)
	for _, c := range contributions {
		payout, err := s.settleMonitorContributionTx(ctx, tx, c, remainders)
		if err != nil {
			return err
		}
		if payout.accountID != "" && payout.amount > 0 {
			payouts[payout.accountID] += payout.amount
		}
	}
	for key, numerator := range remainders {
		parts := strings.SplitN(key, "\x00", 2)
		if len(parts) != 2 {
			return ErrInvalid
		}
		if _, err = tx.Exec(ctx, `UPDATE sentry_monitor_remainders SET numerator=$3 WHERE account_id=$1 AND system_id=$2`, parts[0], parts[1], numerator); err != nil {
			return err
		}
	}
	for accountID, amount := range payouts {
		batchID := stableUUID("monitor-batch|" + cursor + "|" + next + "|" + accountID)
		reference := "sentry-monitor-batch:" + batchID
		if err = s.MonitorRewardFunding.CreditMonitorRewardTx(ctx, tx, accountID, reference, batchID, amount); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Service) settleMonitorContributionTx(ctx context.Context, tx pgx.Tx, c MonitorContribution, remainders map[string]int64) (monitorPayout, error) {
	if strings.TrimSpace(c.ContributionID) == "" || c.DurationSeconds <= 0 || c.DurationSeconds > 86400 {
		return monitorPayout{}, ErrInvalid
	}
	start, err := time.Parse(time.RFC3339Nano, c.StartedAt)
	if err != nil {
		return monitorPayout{}, ErrInvalid
	}
	end, err := time.Parse(time.RFC3339Nano, c.EndedAt)
	if err != nil || !end.After(start) || end.Sub(start)%time.Second != 0 || int64(end.Sub(start)/time.Second) != c.DurationSeconds {
		return monitorPayout{}, ErrInvalid
	}
	evidence, _ := json.Marshal(c.Evidence)
	if len(evidence) == 0 {
		evidence = []byte(`{}`)
	}
	var existingState string
	err = tx.QueryRow(ctx, `SELECT state FROM sentry_monitor_rewards WHERE contribution_id=$1`, c.ContributionID).Scan(&existingState)
	exists := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return monitorPayout{}, err
	}
	if exists && existingState != "excluded" {
		return monitorPayout{}, nil
	}
	account := strings.TrimSpace(c.AccountID)
	keyID := strings.TrimSpace(c.KeyID)
	systemID := monitorSystemID(c)
	var accountID string
	if _, err := uuidText(account); err == nil && keyID != "" {
		// The Sentry client authenticates with the remote key ID. Seat stores
		// that value in sentry_keys.remote_key_id while its local row ID is a
		// different UUID; resolve the row by the remote ID before crediting.
		err = tx.QueryRow(ctx, `SELECT account_id::text FROM sentry_keys WHERE remote_key_id=$1 AND account_id=$2 AND 'monitor'=ANY(permissions) AND status='active'`, keyID, account).Scan(&accountID)
		if errors.Is(err, pgx.ErrNoRows) {
			// Keep compatibility with locally generated contribution fixtures
			// that may carry the Seat row ID instead of the remote key ID.
			err = tx.QueryRow(ctx, `SELECT account_id::text FROM sentry_keys WHERE id=$1 AND account_id=$2 AND 'monitor'=ANY(permissions) AND status='active'`, keyID, account).Scan(&accountID)
		}
	}
	if accountID == "" || strings.TrimSpace(c.Eligibility) != "eligible" || c.PrimaryGeneration <= 0 {
		if !exists {
			_, err = tx.Exec(ctx, `INSERT INTO sentry_monitor_rewards(contribution_id,account_id,remote_key_id,client_id,system_id,system_name,primary_generation,started_at,ended_at,duration_seconds,state,numerator,coins_minor,fingerprint,evidence,price_snapshot) VALUES($1,NULL,$2,$3,$4,$5,$6,$7,$8,$9,'excluded',0,0,$10,$11,'{}')`, c.ContributionID, keyID, c.ClientID, systemID, c.SystemName, c.PrimaryGeneration, start, end, c.DurationSeconds, monitorFingerprint(c), evidence)
		}
		return monitorPayout{}, err
	}
	var overlap bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sentry_monitor_rewards WHERE system_id=$1 AND state='rewarded' AND contribution_id<>$2 AND started_at < $4 AND ended_at > $3)`, systemID, c.ContributionID, start, end).Scan(&overlap); err != nil {
		return monitorPayout{}, err
	}
	if overlap {
		if !exists {
			_, err = tx.Exec(ctx, `INSERT INTO sentry_monitor_rewards(contribution_id,account_id,remote_key_id,client_id,system_id,system_name,primary_generation,started_at,ended_at,duration_seconds,state,numerator,coins_minor,fingerprint,evidence,price_snapshot) VALUES($1,NULL,$2,$3,$4,$5,$6,$7,$8,$9,'excluded',0,0,$10,$11,'{}')`, c.ContributionID, keyID, c.ClientID, systemID, c.SystemName, c.PrimaryGeneration, start, end, c.DurationSeconds, monitorFingerprint(c), evidence)
		}
		return monitorPayout{}, err
	}
	policies, err := s.monitorPolicies(ctx, tx, end)
	if err != nil {
		return monitorPayout{}, err
	}
	if len(policies) == 0 { // No configured reward price: retain evidence but do not invent a zero-priced payout.
		return monitorPayout{}, errMonitorRewardUnpriced
	}
	remainderKey := accountID + "\x00" + systemID
	remainder, loaded := remainders[remainderKey]
	if !loaded {
		err = tx.QueryRow(ctx, `SELECT numerator FROM sentry_monitor_remainders WHERE account_id=$1 AND system_id=$2 FOR UPDATE`, accountID, systemID).Scan(&remainder)
		if errors.Is(err, pgx.ErrNoRows) {
			if _, err = tx.Exec(ctx, `INSERT INTO sentry_monitor_remainders(account_id,system_id,numerator) VALUES($1,$2,0) ON CONFLICT DO NOTHING`, accountID, systemID); err != nil {
				return monitorPayout{}, err
			}
			remainder = 0
		} else if err != nil {
			return monitorPayout{}, err
		}
		remainders[remainderKey] = remainder
	}
	total := remainder
	priceSnapshot := make([]map[string]any, 0, len(policies))
	credited := int64(0)
	for i, p := range policies {
		segStart := start
		if p.Effective.After(segStart) {
			segStart = p.Effective
		}
		segEnd := end
		if i+1 < len(policies) && policies[i+1].Effective.Before(segEnd) {
			segEnd = policies[i+1].Effective
		}
		seconds := int64(segEnd.Sub(segStart) / time.Second)
		if seconds <= 0 || p.Reward == 0 {
			continue
		}
		if seconds > math.MaxInt64/p.Reward {
			return monitorPayout{}, ErrInvalid
		}
		total += seconds * p.Reward
		if total < 0 {
			return monitorPayout{}, ErrInvalid
		}
		credited += total / 3600
		total %= 3600
		priceSnapshot = append(priceSnapshot, map[string]any{"version": p.Version, "effective_at": p.Effective.UTC().Format(time.RFC3339Nano), "hourly_reward_minor": p.Reward, "seconds": seconds})
	}
	remainders[remainderKey] = total
	snapshot, _ := json.Marshal(priceSnapshot)
	if exists {
		_, err = tx.Exec(ctx, `UPDATE sentry_monitor_rewards SET account_id=$2,remote_key_id=$3,client_id=$4,system_id=$5,system_name=$6,primary_generation=$7,started_at=$8,ended_at=$9,duration_seconds=$10,state='rewarded',numerator=$11,coins_minor=$12,fingerprint=$13,evidence=$14,price_snapshot=$15 WHERE contribution_id=$1`, c.ContributionID, accountID, keyID, c.ClientID, systemID, c.SystemName, c.PrimaryGeneration, start, end, c.DurationSeconds, total, credited, monitorFingerprint(c), evidence, snapshot)
	} else {
		_, err = tx.Exec(ctx, `INSERT INTO sentry_monitor_rewards(contribution_id,account_id,remote_key_id,client_id,system_id,system_name,primary_generation,started_at,ended_at,duration_seconds,state,numerator,coins_minor,fingerprint,evidence,price_snapshot) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'rewarded',$11,$12,$13,$14,$15)`, c.ContributionID, accountID, keyID, c.ClientID, systemID, c.SystemName, c.PrimaryGeneration, start, end, c.DurationSeconds, total, credited, monitorFingerprint(c), evidence, snapshot)
	}
	if err != nil {
		return monitorPayout{}, err
	}
	return monitorPayout{accountID: accountID, amount: credited}, nil
}

func (s *Service) monitorPolicies(ctx context.Context, tx pgx.Tx, end time.Time) ([]monitorPolicyRow, error) {
	rows, err := tx.Query(ctx, `SELECT monitor_hourly_reward_minor,effective_at,version FROM sentry_time_pricing WHERE effective_at < $1 ORDER BY effective_at,version`, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []monitorPolicyRow{}
	for rows.Next() {
		var p monitorPolicyRow
		if err := rows.Scan(&p.Reward, &p.Effective, &p.Version); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
func monitorFingerprint(c MonitorContribution) string {
	return fmt.Sprintf("%s|%s|%s|%s|%d|%s|%s", c.ContributionID, c.ClientID, monitorSystemID(c), c.SystemName, c.PrimaryGeneration, c.StartedAt, c.EndedAt)
}
func stableUUID(raw string) string {
	h := sha256.Sum256([]byte(raw))
	h[6] = (h[6] & 0x0f) | 0x40
	h[8] = (h[8] & 0x3f) | 0x80
	return fmt.Sprintf("%s-%s-%s-%s-%s", hex.EncodeToString(h[0:4]), hex.EncodeToString(h[4:6]), hex.EncodeToString(h[6:8]), hex.EncodeToString(h[8:10]), hex.EncodeToString(h[10:16]))
}
func uuidText(raw string) (string, error) {
	if len(raw) != 36 || raw[8] != '-' || raw[13] != '-' || raw[18] != '-' || raw[23] != '-' {
		return "", ErrInvalid
	}
	return raw, nil
}

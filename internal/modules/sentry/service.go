package sentry

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"glorynavy.local/seat/internal/platform/jobs"
)

var (
	ErrInvalid              = errors.New("invalid sentry key request")
	ErrConflict             = errors.New("sentry key state conflict")
	ErrAlertDisabled        = errors.New("sentry alert consumption is disabled")
	ErrAlertPricingConflict = errors.New("sentry alert pricing version conflict")
	ErrAlertPricingInvalid  = errors.New("invalid sentry alert pricing")
)

type AlertGrantPolicy struct {
	PriceVersion    string
	UnitSeconds     int64
	UnitPriceMinor  int64
	MaxGrantSeconds int64
	GrantTTL        time.Duration
}

// AlertPricing is the administrator-visible policy. Coins are returned in
// exchange minor units so the API never rounds a billing value in the browser.
type AlertPricing struct {
	PriceVersion    string     `json:"price_version"`
	UnitSeconds     int64      `json:"unit_seconds"`
	UnitPriceMinor  int64      `json:"unit_price_minor"`
	MaxGrantSeconds int64      `json:"max_grant_seconds"`
	GrantTTLSeconds int64      `json:"grant_ttl_seconds"`
	Version         int64      `json:"version"`
	Configured      bool       `json:"configured"`
	ChargingEnabled bool       `json:"charging_enabled"`
	CanEdit         bool       `json:"can_edit"`
	UpdatedAt       *time.Time `json:"updated_at,omitempty"`
}

type AlertPricingEdit struct {
	PriceVersion    string `json:"price_version"`
	UnitSeconds     int64  `json:"unit_seconds"`
	UnitPriceMinor  int64  `json:"unit_price_minor"`
	MaxGrantSeconds int64  `json:"max_grant_seconds"`
	GrantTTLSeconds int64  `json:"grant_ttl_seconds"`
	Version         int64  `json:"version"`
}

// AlertGrantFunding is the host-owned exchange boundary for online time
// charges. The sentry module never imports exchange's private store.
type AlertGrantFunding interface {
	ReserveAlertTime(context.Context, string, string, string, string, int64, int64, int64, time.Time) error
}

// AlertSystemGrantFunding is the versioned extension used when a warning
// allowance is tied to one selected solar system. Legacy integrations may
// continue using AlertGrantFunding with an empty system attribution.
type AlertSystemGrantFunding interface {
	ReserveAlertTimeForSystem(context.Context, string, string, string, string, string, int64, int64, int64, time.Time) error
}

type Key struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Prefix      string   `json:"prefix"`
	Permissions []string `json:"permissions"`
	Status      string   `json:"status"`
	RemoteID    string   `json:"remote_key_id,omitempty"`
	Version     int64    `json:"version"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
	RevokedAt   string   `json:"revoked_at,omitempty"`
	LastError   string   `json:"last_error,omitempty"`
	Secret      string   `json:"secret,omitempty"`
}

type Service struct {
	Pool                 *pgxpool.Pool
	Remote               Remote
	ClientUsageRemote    ClientUsageRemote
	MonitorRemote        MonitorRemote
	AlertSettlement      AlertIntervalSettlement
	AlertEnabled         bool
	AlertPolicy          AlertGrantPolicy
	AlertFunding         AlertGrantFunding
	AlertUsageReader     AlertUsageReader
	MonitorRewardFunding MonitorRewardFunding
	Administrator        func(context.Context, string) (bool, error)
	policyMu             sync.RWMutex
}

func New(pool *pgxpool.Pool, remote Remote) *Service {
	monitor, _ := remote.(MonitorRemote)
	usage, _ := remote.(ClientUsageRemote)
	return &Service{Pool: pool, Remote: remote, MonitorRemote: monitor, ClientUsageRemote: usage}
}

// MonitorRewardFunding is the host-owned coin ledger boundary. The sentry
// module supplies a stable evidence reference and amount; exchange owns the
// actual wallet row and idempotency constraint.
type MonitorRewardFunding interface {
	CreditMonitorRewardTx(context.Context, pgx.Tx, string, string, string, int64) error
}

// SetAlertSettlement installs the host's exchange boundary without importing
// exchange's private store into the sentry module.
func (s *Service) SetAlertSettlement(settlement AlertIntervalSettlement) {
	s.AlertSettlement = settlement
}

func (s *Service) SetAlertFunding(funding AlertGrantFunding) {
	s.AlertFunding = funding
}

func (s *Service) SetAlertUsageReader(reader AlertUsageReader) {
	s.AlertUsageReader = reader
}

func (s *Service) alertPolicy() AlertGrantPolicy {
	s.policyMu.RLock()
	defer s.policyMu.RUnlock()
	return s.AlertPolicy
}

func (s *Service) setAlertPolicy(policy AlertGrantPolicy) {
	s.policyMu.Lock()
	s.AlertPolicy = policy
	s.policyMu.Unlock()
}

// alertChargingEnabled combines the deployment capability with the persisted
// administrator switch. A disabled switch stops new online-time charges.
func (s *Service) alertChargingEnabled(ctx context.Context) (bool, error) {
	if !s.AlertEnabled {
		return false, nil
	}
	if s.Pool == nil {
		// Keep in-memory service fixtures useful while the production service
		// always reads the durable switch below.
		return true, nil
	}
	var enabled bool
	err := s.Pool.QueryRow(ctx, `SELECT charging_enabled FROM sentry_alert_pricing WHERE id=1`).Scan(&enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return enabled, nil
}

// currentAlertPolicy refreshes the process-local bootstrap policy from the
// singleton row. This keeps a multi-instance deployment consistent when an
// administrator saves a rule through another API process.
func (s *Service) currentAlertPolicy(ctx context.Context) (AlertGrantPolicy, error) {
	policy := s.alertPolicy()
	if s.Pool == nil {
		return policy, nil
	}
	var ttl int64
	err := s.Pool.QueryRow(ctx, `SELECT price_version,unit_seconds,unit_price_minor,max_grant_seconds,grant_ttl_seconds FROM sentry_alert_pricing WHERE id=1`).Scan(&policy.PriceVersion, &policy.UnitSeconds, &policy.UnitPriceMinor, &policy.MaxGrantSeconds, &ttl)
	if errors.Is(err, pgx.ErrNoRows) {
		return policy, nil
	}
	if err != nil {
		return AlertGrantPolicy{}, err
	}
	policy.GrantTTL = time.Duration(ttl) * time.Second
	if !policyValid(policy) {
		return AlertGrantPolicy{}, ErrAlertPricingInvalid
	}
	s.setAlertPolicy(policy)
	return policy, nil
}

func policyValid(policy AlertGrantPolicy) bool {
	return strings.TrimSpace(policy.PriceVersion) != "" && len(policy.PriceVersion) <= 80 &&
		!strings.ContainsAny(policy.PriceVersion, "\r\n") && policy.UnitSeconds > 0 && policy.UnitSeconds <= 86400 &&
		policy.UnitPriceMinor > 0 && policy.UnitPriceMinor <= 1000000000000 &&
		policy.MaxGrantSeconds > 0 && policy.MaxGrantSeconds <= 31*24*60*60 &&
		policy.GrantTTL >= time.Minute && policy.GrantTTL <= 31*24*time.Hour
}

func pricingFromPolicy(policy AlertGrantPolicy, configured, enabled, canEdit bool, version int64, updatedAt time.Time) AlertPricing {
	var stamp *time.Time
	if !updatedAt.IsZero() {
		value := updatedAt.UTC()
		stamp = &value
	}
	return AlertPricing{PriceVersion: policy.PriceVersion, UnitSeconds: policy.UnitSeconds, UnitPriceMinor: policy.UnitPriceMinor, MaxGrantSeconds: policy.MaxGrantSeconds, GrantTTLSeconds: int64(policy.GrantTTL / time.Second), Version: version, Configured: configured, ChargingEnabled: enabled, CanEdit: canEdit, UpdatedAt: stamp}
}

func policyFromEdit(edit AlertPricingEdit) AlertGrantPolicy {
	return AlertGrantPolicy{PriceVersion: strings.TrimSpace(edit.PriceVersion), UnitSeconds: edit.UnitSeconds, UnitPriceMinor: edit.UnitPriceMinor, MaxGrantSeconds: edit.MaxGrantSeconds, GrantTTL: time.Duration(edit.GrantTTLSeconds) * time.Second}
}

// LoadAlertPricing applies the persisted policy when one exists. The
// environment policy remains the bootstrap fallback for old databases; the
// durable charging switch is read separately from sentry_alert_pricing.
func (s *Service) LoadAlertPricing(ctx context.Context, fallback AlertGrantPolicy) error {
	if s.Pool == nil {
		s.setAlertPolicy(fallback)
		return nil
	}
	var p AlertGrantPolicy
	var ttl int64
	if err := s.Pool.QueryRow(ctx, `SELECT price_version,unit_seconds,unit_price_minor,max_grant_seconds,grant_ttl_seconds FROM sentry_alert_pricing WHERE id=1`).Scan(&p.PriceVersion, &p.UnitSeconds, &p.UnitPriceMinor, &p.MaxGrantSeconds, &ttl); errors.Is(err, pgx.ErrNoRows) {
		s.setAlertPolicy(fallback)
		return nil
	} else if err != nil {
		return err
	} else {
		p.GrantTTL = time.Duration(ttl) * time.Second
		if !policyValid(p) {
			return ErrAlertPricingInvalid
		}
		s.setAlertPolicy(p)
		return nil
	}
}

func (s *Service) ReadAlertPricing(ctx context.Context, user string) (AlertPricing, error) {
	canEdit := false
	if s.Administrator != nil {
		var err error
		canEdit, err = s.Administrator(ctx, user)
		if err != nil {
			return AlertPricing{}, err
		}
	}
	enabled, err := s.alertChargingEnabled(ctx)
	if err != nil {
		return AlertPricing{}, err
	}
	if s.Pool == nil {
		return pricingFromPolicy(s.alertPolicy(), false, enabled, canEdit, 0, time.Time{}), nil
	}
	if _, err := s.currentAlertPolicy(ctx); err != nil {
		return AlertPricing{}, err
	}
	var p AlertGrantPolicy
	var ttl, version int64
	var updated time.Time
	err = s.Pool.QueryRow(ctx, `SELECT price_version,unit_seconds,unit_price_minor,max_grant_seconds,grant_ttl_seconds,version,updated_at FROM sentry_alert_pricing WHERE id=1`).Scan(&p.PriceVersion, &p.UnitSeconds, &p.UnitPriceMinor, &p.MaxGrantSeconds, &ttl, &version, &updated)
	if errors.Is(err, pgx.ErrNoRows) {
		return pricingFromPolicy(s.alertPolicy(), false, enabled, canEdit, 0, time.Time{}), nil
	}
	if err != nil {
		return AlertPricing{}, err
	}
	p.GrantTTL = time.Duration(ttl) * time.Second
	if !policyValid(p) {
		return AlertPricing{}, ErrAlertPricingInvalid
	}
	return pricingFromPolicy(p, true, enabled, canEdit, version, updated), nil
}

func (s *Service) EditAlertPricing(ctx context.Context, user string, edit AlertPricingEdit) (AlertPricing, error) {
	if s.Administrator == nil {
		return AlertPricing{}, pgx.ErrNoRows
	}
	if s.Pool == nil {
		return AlertPricing{}, ErrRemoteUnavailable
	}
	ok, err := s.Administrator(ctx, user)
	if err != nil {
		return AlertPricing{}, err
	}
	if !ok {
		return AlertPricing{}, pgx.ErrNoRows
	}
	policy := policyFromEdit(edit)
	if edit.Version < 0 || !policyValid(policy) {
		return AlertPricing{}, ErrAlertPricingInvalid
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return AlertPricing{}, err
	}
	defer tx.Rollback(ctx)
	// The singleton row does not exist on the first save, so FOR UPDATE cannot
	// serialize two concurrent inserts. A transaction advisory lock gives the
	// absent-row case the same optimistic-version semantics as later updates.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(62062)`); err != nil {
		return AlertPricing{}, err
	}
	var current AlertGrantPolicy
	var ttl, currentVersion int64
	var updated time.Time
	var chargingEnabled bool
	rowErr := tx.QueryRow(ctx, `SELECT price_version,unit_seconds,unit_price_minor,max_grant_seconds,grant_ttl_seconds,version,updated_at,charging_enabled FROM sentry_alert_pricing WHERE id=1 FOR UPDATE`).Scan(&current.PriceVersion, &current.UnitSeconds, &current.UnitPriceMinor, &current.MaxGrantSeconds, &ttl, &currentVersion, &updated, &chargingEnabled)
	if errors.Is(rowErr, pgx.ErrNoRows) {
		if edit.Version != 0 {
			return AlertPricing{}, ErrAlertPricingConflict
		}
		currentVersion = 0
	} else if rowErr != nil {
		return AlertPricing{}, rowErr
	} else {
		current.GrantTTL = time.Duration(ttl) * time.Second
		if edit.Version != currentVersion {
			return AlertPricing{}, ErrAlertPricingConflict
		}
	}
	nextVersion := currentVersion + 1
	if currentVersion == 0 {
		_, err = tx.Exec(ctx, `INSERT INTO sentry_alert_pricing(id,price_version,unit_seconds,unit_price_minor,max_grant_seconds,grant_ttl_seconds,version,updated_by) VALUES(1,$1,$2,$3,$4,$5,$6,$7)`, policy.PriceVersion, policy.UnitSeconds, policy.UnitPriceMinor, policy.MaxGrantSeconds, edit.GrantTTLSeconds, nextVersion, user)
	} else {
		_, err = tx.Exec(ctx, `UPDATE sentry_alert_pricing SET price_version=$1,unit_seconds=$2,unit_price_minor=$3,max_grant_seconds=$4,grant_ttl_seconds=$5,version=$6,updated_by=$7,updated_at=now() WHERE id=1`, policy.PriceVersion, policy.UnitSeconds, policy.UnitPriceMinor, policy.MaxGrantSeconds, edit.GrantTTLSeconds, nextVersion, user)
	}
	if err != nil {
		return AlertPricing{}, err
	}
	before := map[string]any{"price_version": current.PriceVersion, "unit_seconds": current.UnitSeconds, "unit_price_minor": current.UnitPriceMinor, "max_grant_seconds": current.MaxGrantSeconds, "grant_ttl_seconds": int64(current.GrantTTL / time.Second), "version": currentVersion, "charging_enabled": chargingEnabled}
	after := map[string]any{"price_version": policy.PriceVersion, "unit_seconds": policy.UnitSeconds, "unit_price_minor": policy.UnitPriceMinor, "max_grant_seconds": policy.MaxGrantSeconds, "grant_ttl_seconds": edit.GrantTTLSeconds, "version": nextVersion, "charging_enabled": chargingEnabled}
	if _, err = tx.Exec(ctx, `INSERT INTO sentry_alert_pricing_audit(actor_id,previous_version,version,before_snapshot,after_snapshot) VALUES($1,$2,$3,$4,$5)`, user, currentVersion, nextVersion, before, after); err != nil {
		return AlertPricing{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return AlertPricing{}, err
	}
	s.setAlertPolicy(policy)
	enabled, err := s.alertChargingEnabled(ctx)
	if err != nil {
		return AlertPricing{}, err
	}
	return pricingFromPolicy(policy, true, enabled, true, nextVersion, time.Now().UTC()), nil
}

// Extension registers monitor rewards and online-time charging workers.
func (s *Service) Extension(enabled bool) jobs.Extension {
	m := monitorRewardExtension(s, enabled)
	u := clientUsageExtension(s, enabled)
	return jobs.Extension{
		Register: func(workers *river.Workers) []*river.PeriodicJob {
			jobs := m.Register(workers)
			return append(jobs, u.Register(workers)...)
		},
		Bind: func(queue *river.Client[pgx.Tx]) { m.Bind(queue); u.Bind(queue) },
	}
}

func normalizePermissions(values []string) ([]string, error) {
	seen := map[string]bool{}
	for _, raw := range values {
		p := strings.TrimSpace(raw)
		if p != "monitor" && p != "alert" {
			return nil, ErrInvalid
		}
		seen[p] = true
	}
	if len(seen) == 0 {
		return nil, ErrInvalid
	}
	out := make([]string, 0, len(seen))
	for _, p := range []string{"monitor", "alert"} {
		if seen[p] {
			out = append(out, p)
		}
	}
	return out, nil
}

func validateName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 80 || strings.ContainsAny(name, "\r\n") {
		return ""
	}
	return name
}

func newSecret() (string, []byte, string, error) {
	raw := make([]byte, 36)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, "", err
	}
	secret := "eve_" + base64.RawURLEncoding.EncodeToString(raw)
	digest := sha256.Sum256([]byte(secret))
	return secret, digest[:], secret[:12], nil
}

func newID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:16]), nil
}

func operationID(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return newID()
	}
	var id pgtype.UUID
	if err := id.Scan(strings.TrimSpace(raw)); err != nil || !id.Valid {
		return "", ErrInvalid
	}
	return id.String(), nil
}

func (s *Service) List(ctx context.Context, account string) ([]Key, error) {
	// A Seat account owns one current warning key. Historical revoked rows stay
	// in the database for audit, but the member-facing view only returns the
	// current card.
	rows, err := s.Pool.Query(ctx, `SELECT id::text,name,key_prefix,permissions,status,remote_key_id,remote_version,created_at::text,updated_at::text,COALESCE(revoked_at::text,''),last_error FROM sentry_keys WHERE account_id=$1 AND status <> 'revoked' ORDER BY created_at DESC LIMIT 1`, account)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Key{}
	for rows.Next() {
		var k Key
		if err := rows.Scan(&k.ID, &k.Name, &k.Prefix, &k.Permissions, &k.Status, &k.RemoteID, &k.Version, &k.CreatedAt, &k.UpdatedAt, &k.RevokedAt, &k.LastError); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (s *Service) Create(ctx context.Context, account, name string, rawPermissions []string, requestKey string) (Key, error) {
	name = validateName(name)
	permissions, err := normalizePermissions(rawPermissions)
	if name == "" || err != nil {
		return Key{}, ErrInvalid
	}
	if s.Remote == nil {
		return Key{}, ErrRemoteUnavailable
	}
	opID, err := operationID(requestKey)
	if err != nil {
		return Key{}, err
	}
	// The request key is the durable operation id. A retried browser request
	// must not create a second remote key after the first response was lost.
	var existing Key
	var existingAccount string
	var existingHash []byte
	if err = s.Pool.QueryRow(ctx, `SELECT account_id::text,id::text,name,key_prefix,key_hash,permissions,status,COALESCE(remote_key_id,''),remote_version,created_at::text,updated_at::text,COALESCE(revoked_at::text,''),last_error FROM sentry_keys WHERE operation_id=$1`, opID).Scan(&existingAccount, &existing.ID, &existing.Name, &existing.Prefix, &existingHash, &existing.Permissions, &existing.Status, &existing.RemoteID, &existing.Version, &existing.CreatedAt, &existing.UpdatedAt, &existing.RevokedAt, &existing.LastError); err == nil {
		if existingAccount != account {
			return Key{}, ErrConflict
		}
		if existing.Name != name || !slices.Equal(existing.Permissions, permissions) {
			return Key{}, ErrConflict
		}
		if (existing.Status == "creating" || existing.Status == "sync_error") && existing.RemoteID == "" {
			if len(existingHash) != sha256.Size {
				return Key{}, ErrConflict
			}
			remote, remoteErr := s.Remote.Create(ctx, ProvisionRequest{OperationID: opID, KeyID: existing.ID, AccountID: account, Name: name, Prefix: existing.Prefix, Hash: hex.EncodeToString(existingHash), Permissions: permissions, Version: 1})
			if remoteErr != nil || remote.ID == "" || remote.ID != existing.ID {
				_, _ = s.Pool.Exec(ctx, `UPDATE sentry_keys SET status='sync_error',last_error=$2,updated_at=now() WHERE id=$1 AND account_id=$3`, existing.ID, "预警平台未确认创建结果", account)
				_, _ = s.Pool.Exec(ctx, `INSERT INTO sentry_key_events(key_id,account_id,action,detail) VALUES($1,$2,'sync_error',jsonb_build_object('operation_id',$3::text))`, existing.ID, account, opID)
				return Key{}, ErrRemoteUnavailable
			}
			if _, err = s.Pool.Exec(ctx, `UPDATE sentry_keys SET remote_key_id=$2,remote_version=$3,status='active',last_error='',updated_at=now() WHERE id=$1 AND account_id=$4`, existing.ID, remote.ID, remote.Version, account); err != nil {
				return Key{}, err
			}
			_, err = s.Pool.Exec(ctx, `INSERT INTO sentry_key_events(key_id,account_id,action,detail) VALUES($1,$2,'active',jsonb_build_object('remote_key_id',$3::text,'remote_version',$4::bigint))`, existing.ID, account, remote.ID, remote.Version)
			if err != nil {
				return Key{}, err
			}
			existing.RemoteID = remote.ID
			existing.Version = remote.Version
			existing.Status = "active"
			existing.LastError = ""
		}
		return existing, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Key{}, err
	}
	// One account has one current card. This check also makes older databases
	// with duplicate active rows safe to use while they are being reconciled.
	var currentID, currentStatus string
	if err = s.Pool.QueryRow(ctx, `SELECT id::text,status FROM sentry_keys WHERE account_id=$1 AND status <> 'revoked' ORDER BY created_at DESC LIMIT 1`, account).Scan(&currentID, &currentStatus); err == nil {
		return Key{}, ErrConflict
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Key{}, err
	}
	keyID, err := newID()
	if err != nil {
		return Key{}, err
	}
	secret, digest, prefix, err := newSecret()
	if err != nil {
		return Key{}, err
	}
	// Record the local intent before crossing the service boundary. A lost
	// response remains visible as creating/sync_error and can be reconciled.
	if _, err = s.Pool.Exec(ctx, `INSERT INTO sentry_keys(id,account_id,operation_id,name,key_prefix,key_hash,permissions,status) VALUES($1,$2,$3,$4,$5,$6,$7,'creating')`, keyID, account, opID, name, prefix, digest, permissions); err != nil {
		return Key{}, err
	}
	if _, err = s.Pool.Exec(ctx, `INSERT INTO sentry_key_events(key_id,account_id,action) VALUES($1,$2,'creating')`, keyID, account); err != nil {
		return Key{}, err
	}
	remote, err := s.Remote.Create(ctx, ProvisionRequest{OperationID: opID, KeyID: keyID, AccountID: account, Name: name, Prefix: prefix, Hash: hex.EncodeToString(digest), Permissions: permissions, Version: 1})
	if err != nil {
		_, _ = s.Pool.Exec(ctx, `UPDATE sentry_keys SET status='sync_error',last_error=$2,updated_at=now() WHERE id=$1`, keyID, "预警平台未确认创建结果")
		_, _ = s.Pool.Exec(ctx, `INSERT INTO sentry_key_events(key_id,account_id,action,detail) VALUES($1,$2,'sync_error',jsonb_build_object('operation_id',$3::text))`, keyID, account, opID)
		return Key{}, ErrRemoteUnavailable
	}
	if remote.ID == "" || remote.ID != keyID {
		_, _ = s.Pool.Exec(ctx, `UPDATE sentry_keys SET status='sync_error',last_error=$2,updated_at=now() WHERE id=$1`, keyID, "预警平台未确认创建结果")
		_, _ = s.Pool.Exec(ctx, `INSERT INTO sentry_key_events(key_id,account_id,action,detail) VALUES($1,$2,'sync_error',jsonb_build_object('operation_id',$3::text))`, keyID, account, opID)
		return Key{}, ErrRemoteUnavailable
	}
	_, err = s.Pool.Exec(ctx, `UPDATE sentry_keys SET remote_key_id=$2,remote_version=$3,status='active',last_error='',updated_at=now() WHERE id=$1`, keyID, remote.ID, remote.Version)
	if err != nil {
		return Key{}, err
	}
	_, err = s.Pool.Exec(ctx, `INSERT INTO sentry_key_events(key_id,account_id,action,detail) VALUES($1,$2,'active',jsonb_build_object('remote_key_id',$3::text,'remote_version',$4::bigint))`, keyID, account, remote.ID, remote.Version)
	if err != nil {
		return Key{}, err
	}
	return Key{ID: keyID, Name: name, Prefix: prefix, Permissions: permissions, Status: "active", RemoteID: remote.ID, Version: remote.Version, Secret: secret}, nil
}

// Rotate provisions a new remote key before revoking the old one. The local
// row keeps its identity and audit history, so the account still has one key
// card while the one-time secret is replaced safely.
func (s *Service) Rotate(ctx context.Context, account, id string) (Key, error) {
	if len(id) != 36 {
		return Key{}, ErrInvalid
	}
	if s.Remote == nil {
		return Key{}, ErrRemoteUnavailable
	}
	var old Key
	if err := s.Pool.QueryRow(ctx, `SELECT id::text,name,key_prefix,permissions,status,COALESCE(remote_key_id,''),remote_version,created_at::text,updated_at::text,COALESCE(revoked_at::text,''),last_error FROM sentry_keys WHERE id=$1 AND account_id=$2`, id, account).Scan(&old.ID, &old.Name, &old.Prefix, &old.Permissions, &old.Status, &old.RemoteID, &old.Version, &old.CreatedAt, &old.UpdatedAt, &old.RevokedAt, &old.LastError); err != nil {
		return Key{}, err
	}
	if old.Status != "active" || strings.TrimSpace(old.RemoteID) == "" {
		return Key{}, ErrConflict
	}
	newKeyID, err := newID()
	if err != nil {
		return Key{}, err
	}
	newOp, err := newID()
	if err != nil {
		return Key{}, err
	}
	revokeOp, err := newID()
	if err != nil {
		return Key{}, err
	}
	secret, digest, prefix, err := newSecret()
	if err != nil {
		return Key{}, err
	}
	if _, err = s.Pool.Exec(ctx, `UPDATE sentry_keys SET status='revoking',last_error='',updated_at=now() WHERE id=$1 AND account_id=$2 AND status='active'`, id, account); err != nil {
		return Key{}, err
	}
	if _, err = s.Pool.Exec(ctx, `INSERT INTO sentry_key_events(key_id,account_id,action,detail) VALUES($1,$2,'revoke_requested',jsonb_build_object('rotation',true,'remote_key_id',$3::text))`, id, account, old.RemoteID); err != nil {
		return Key{}, err
	}
	remote, err := s.Remote.Create(ctx, ProvisionRequest{OperationID: newOp, KeyID: newKeyID, AccountID: account, Name: old.Name, Prefix: prefix, Hash: hex.EncodeToString(digest), Permissions: old.Permissions, Version: 1})
	if err != nil || remote.ID == "" || remote.ID != newKeyID {
		_, _ = s.Pool.Exec(ctx, `UPDATE sentry_keys SET status='active',last_error='',updated_at=now() WHERE id=$1 AND account_id=$2`, id, account)
		return Key{}, ErrRemoteUnavailable
	}
	if err = s.Remote.Revoke(ctx, old.RemoteID, revokeOp); err != nil {
		// The old key remains the usable one. Remove the new remote key so a
		// failed rotation cannot leave two active credentials behind.
		rollbackOp, rollbackIDErr := newID()
		var rollbackErr error
		if rollbackIDErr != nil {
			rollbackErr = rollbackIDErr
		} else {
			rollbackErr = s.Remote.Revoke(ctx, newKeyID, rollbackOp)
		}
		if rollbackErr == nil {
			_, _ = s.Pool.Exec(ctx, `UPDATE sentry_keys SET status='active',last_error='',updated_at=now() WHERE id=$1 AND account_id=$2`, id, account)
		} else {
			_, _ = s.Pool.Exec(ctx, `UPDATE sentry_keys SET status='sync_error',last_error='预警平台未确认密钥更新结果',updated_at=now() WHERE id=$1 AND account_id=$2`, id, account)
			_, _ = s.Pool.Exec(ctx, `INSERT INTO sentry_key_events(key_id,account_id,action,detail) VALUES($1,$2,'sync_error',jsonb_build_object('rotation',true,'remote_key_id',$3::text))`, id, account, newKeyID)
		}
		return Key{}, ErrRemoteUnavailable
	}
	if _, err = s.Pool.Exec(ctx, `UPDATE sentry_keys SET operation_id=$2,remote_key_id=$3,key_prefix=$4,key_hash=$5,permissions=$6,remote_version=$7,status='active',last_error='',revoked_at=NULL,updated_at=now() WHERE id=$1 AND account_id=$8`, id, newOp, remote.ID, prefix, digest, old.Permissions, remote.Version, account); err != nil {
		_, _ = s.Pool.Exec(ctx, `UPDATE sentry_keys SET status='sync_error',last_error='预警平台已更新，本地记录待核对',updated_at=now() WHERE id=$1 AND account_id=$2`, id, account)
		return Key{}, err
	}
	_, err = s.Pool.Exec(ctx, `INSERT INTO sentry_key_events(key_id,account_id,action,detail) VALUES($1,$2,'active',jsonb_build_object('rotation',true,'previous_remote_key_id',$3::text,'remote_key_id',$4::text,'remote_version',$5::bigint))`, id, account, old.RemoteID, remote.ID, remote.Version)
	if err != nil {
		return Key{}, err
	}
	return Key{ID: id, Name: old.Name, Prefix: prefix, Permissions: old.Permissions, Status: "active", RemoteID: remote.ID, Version: remote.Version, Secret: secret}, nil
}

func (s *Service) Revoke(ctx context.Context, account, id string) error {
	if len(id) != 36 {
		return ErrInvalid
	}
	var remoteID, status string
	if err := s.Pool.QueryRow(ctx, `SELECT remote_key_id,status FROM sentry_keys WHERE id=$1 AND account_id=$2`, id, account).Scan(&remoteID, &status); err != nil {
		return err
	}
	if status == "revoked" {
		// Revoke is intentionally idempotent. A browser retry after the
		// first response was lost must not turn an already revoked key into
		// a user-visible conflict.
		return nil
	}
	if status != "active" && status != "sync_error" {
		return ErrConflict
	}
	if strings.TrimSpace(remoteID) == "" {
		return ErrRemoteUnavailable
	}
	if s.Remote == nil {
		return ErrRemoteUnavailable
	}
	op, err := newID()
	if err != nil {
		return err
	}
	if _, err = s.Pool.Exec(ctx, `UPDATE sentry_keys SET status='revoking',updated_at=now(),last_error='' WHERE id=$1 AND account_id=$2 AND status IN ('active','sync_error')`, id, account); err != nil {
		return err
	}
	if err = s.Remote.Revoke(ctx, remoteID, op); err != nil {
		_, _ = s.Pool.Exec(ctx, `UPDATE sentry_keys SET status='sync_error',last_error='预警平台未确认吊销结果',updated_at=now() WHERE id=$1`, id)
		return ErrRemoteUnavailable
	}
	if _, err = s.Pool.Exec(ctx, `UPDATE sentry_keys SET status='revoked',revoked_at=now(),updated_at=now(),last_error='' WHERE id=$1 AND account_id=$2`, id, account); err != nil {
		return err
	}
	_, err = s.Pool.Exec(ctx, `INSERT INTO sentry_key_events(key_id,account_id,action) VALUES($1,$2,'revoked')`, id, account)
	return err
}

func (s *Service) MergeAccountTx(ctx context.Context, tx pgx.Tx, source, target string, apply bool) (json.RawMessage, error) {
	var keys int64
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM sentry_keys WHERE account_id=$1`, source).Scan(&keys); err != nil {
		return nil, err
	}
	var monitorRewards int64
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM sentry_monitor_rewards WHERE account_id=$1`, source).Scan(&monitorRewards); err != nil {
		return nil, err
	}
	if !apply {
		return json.Marshal(map[string]any{"keys": keys, "monitor_rewards": monitorRewards})
	}
	if _, err := tx.Exec(ctx, `UPDATE sentry_keys SET account_id=$2,updated_at=now() WHERE account_id=$1`, source, target); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE sentry_key_events SET account_id=$2 WHERE account_id=$1`, source, target); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE sentry_monitor_rewards SET original_account_id=coalesce(original_account_id,account_id),account_id=$2 WHERE account_id=$1`, source, target); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO sentry_monitor_remainders(account_id,system_id,numerator) SELECT $2,system_id,numerator FROM sentry_monitor_remainders WHERE account_id=$1 ON CONFLICT(account_id,system_id) DO UPDATE SET numerator=sentry_monitor_remainders.numerator+EXCLUDED.numerator`, source, target); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM sentry_monitor_remainders WHERE account_id=$1`, source); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"keys": keys, "monitor_rewards": monitorRewards})
}

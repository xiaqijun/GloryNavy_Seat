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
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"glorynavy.local/seat/internal/platform/jobs"
)

var (
	ErrInvalid       = errors.New("invalid sentry key request")
	ErrConflict      = errors.New("sentry key state conflict")
	ErrAlertDisabled = errors.New("sentry alert consumption is disabled")
)

type AlertGrantPolicy struct {
	PriceVersion    string
	UnitSeconds     int64
	UnitPriceMinor  int64
	MaxGrantSeconds int64
	GrantTTL        time.Duration
}

// AlertGrantFunding is the host-owned exchange boundary for prepaid alert
// grants. The sentry module never imports exchange's private store.
type AlertGrantFunding interface {
	ReserveAlertTime(context.Context, string, string, string, string, int64, int64, int64, time.Time) error
	ReleaseAlertGrant(context.Context, string, string) error
	AlertGrantAccount(context.Context, string) (string, error)
	AlertGrantExpiresAt(context.Context, string) (time.Time, error)
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
	Pool             *pgxpool.Pool
	Remote           Remote
	AlertRemote      AlertRemote
	AlertSettlement  AlertIntervalSettlement
	AlertEnabled     bool
	AlertPolicy      AlertGrantPolicy
	AlertFunding     AlertGrantFunding
	AlertUsageReader AlertUsageReader
	alertQueue       *river.Client[pgx.Tx]
}

func New(pool *pgxpool.Pool, remote Remote) *Service {
	alerts, _ := remote.(AlertRemote)
	return &Service{Pool: pool, Remote: remote, AlertRemote: alerts}
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

// CreateAlertGrant reserves coins locally before projecting the same frozen
// time/price snapshot to Sentry. The request UUID is also the stable grant ID,
// so a lost response can be retried without creating a second allowance.
func (s *Service) CreateAlertGrant(ctx context.Context, account, keyID, requestKey string, reservedSeconds int64) (AlertGrant, error) {
	var out AlertGrant
	if !s.AlertEnabled || s.AlertRemote == nil || s.AlertFunding == nil {
		return out, ErrAlertDisabled
	}
	if strings.TrimSpace(account) == "" || strings.TrimSpace(keyID) == "" || strings.TrimSpace(requestKey) == "" || reservedSeconds <= 0 || reservedSeconds > s.AlertPolicy.MaxGrantSeconds || s.AlertPolicy.PriceVersion == "" || s.AlertPolicy.UnitSeconds <= 0 || s.AlertPolicy.UnitPriceMinor <= 0 || s.AlertPolicy.GrantTTL <= 0 {
		return out, ErrInvalid
	}
	opID, err := operationID(requestKey)
	if err != nil {
		return out, err
	}
	var remoteKeyID, status string
	var permissions []string
	if err = s.Pool.QueryRow(ctx, `SELECT COALESCE(remote_key_id,''),status,permissions FROM sentry_keys WHERE id=$1 AND account_id=$2`, keyID, account).Scan(&remoteKeyID, &status, &permissions); err != nil {
		return out, err
	}
	if status != "active" || remoteKeyID == "" || !slices.Contains(permissions, "alert") {
		return out, ErrConflict
	}
	expiresAt, err := s.AlertFunding.AlertGrantExpiresAt(ctx, opID)
	if errors.Is(err, pgx.ErrNoRows) {
		expiresAt = time.Now().UTC().Add(s.AlertPolicy.GrantTTL)
	} else if err != nil {
		return out, err
	}
	if err = s.AlertFunding.ReserveAlertTime(ctx, opID, account, opID, s.AlertPolicy.PriceVersion, s.AlertPolicy.UnitSeconds, s.AlertPolicy.UnitPriceMinor, reservedSeconds, expiresAt); err != nil {
		return out, err
	}
	return s.AlertRemote.CreateAlertGrant(ctx, AlertGrantRequest{OperationID: opID, GrantID: opID, AccountID: account, KeyID: remoteKeyID, PriceVersion: s.AlertPolicy.PriceVersion, UnitSeconds: s.AlertPolicy.UnitSeconds, UnitPriceMinor: s.AlertPolicy.UnitPriceMinor, ReservedSeconds: reservedSeconds, ExpiresAt: expiresAt})
}

// RevokeAlertGrant first revokes the remote allowance, then releases any
// unused local reservation. Both calls are idempotent under the request UUID.
func (s *Service) RevokeAlertGrant(ctx context.Context, account, grantID, requestKey string) error {
	if !s.AlertEnabled || s.AlertRemote == nil || s.AlertFunding == nil {
		return ErrAlertDisabled
	}
	if strings.TrimSpace(requestKey) == "" {
		return ErrInvalid
	}
	opID, err := operationID(requestKey)
	if err != nil {
		return err
	}
	owner, err := s.AlertFunding.AlertGrantAccount(ctx, grantID)
	if err != nil {
		return err
	}
	if owner != account {
		return pgx.ErrNoRows
	}
	if err = s.AlertRemote.RevokeAlertGrant(ctx, grantID, opID); err != nil {
		return err
	}
	return s.AlertFunding.ReleaseAlertGrant(ctx, grantID, opID)
}

// Extension registers the durable Sentry delivery reconciliation worker with
// the shared River runtime. It remains opt-in until pricing and production
// evidence have been verified by the host.
func (s *Service) Extension(enabled bool) jobs.Extension {
	return alertReconcileExtension(s, enabled)
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
	if !apply {
		return json.Marshal(map[string]any{"keys": keys})
	}
	if _, err := tx.Exec(ctx, `UPDATE sentry_keys SET account_id=$2,updated_at=now() WHERE account_id=$1`, source, target); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE sentry_key_events SET account_id=$2 WHERE account_id=$1`, source, target); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"keys": keys})
}

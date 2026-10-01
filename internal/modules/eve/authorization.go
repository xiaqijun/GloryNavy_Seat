package eve

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
)

type AuthorizationService struct {
	pool   *pgxpool.Pool
	client *Client
	esi    *ESIService
	box    cipher.AEAD
	logger *slog.Logger
	sync   *SyncService
}

func NewAuthorization(pool *pgxpool.Pool, client *Client, key string, logger *slog.Logger) (*AuthorizationService, error) {
	raw, err := base64.StdEncoding.DecodeString(key)
	if err != nil || len(raw) != 32 {
		return nil, errors.New("EVE_TOKEN_KEY must be a base64-encoded 32-byte key")
	}
	block, err := aes.NewCipher(raw)
	if err != nil {
		return nil, err
	}
	box, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	s := &AuthorizationService{pool: pool, client: client, esi: newESI(pool, box), box: box, logger: logger}
	s.esi.credentials = s
	s.esi.shared.Observer = s.observeESI
	return s, nil
}
func (s *AuthorizationService) seal(id int64, owner []byte, a *authorization) ([]byte, error) {
	raw, err := json.Marshal(a)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, s.box.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, err
	}
	return s.box.Seal(nonce, nonce, raw, append([]byte(strconv.FormatInt(id, 10)+":"), owner...)), nil
}
func (s *AuthorizationService) open(id int64, owner, raw []byte) (*authorization, error) {
	n := s.box.NonceSize()
	if len(raw) < n {
		return nil, ErrReauthorize
	}
	plain, err := s.box.Open(nil, raw[:n], raw[n:], append([]byte(strconv.FormatInt(id, 10)+":"), owner...))
	if err != nil {
		return nil, ErrReauthorize
	}
	var a authorization
	if json.Unmarshal(plain, &a) != nil {
		return nil, ErrReauthorize
	}
	return &a, nil
}
func timestamp(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }
func (s *AuthorizationService) Save(ctx context.Context, ch Character) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if err = s.SaveTx(ctx, tx, ch); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SaveTx lets the host commit identity, credentials and session changes together.
func (s *AuthorizationService) SaveTx(ctx context.Context, tx pgx.Tx, ch Character) error {
	if ch.authorization == nil || ch.authorization.RefreshToken == "" || !slices.Contains(ch.authorization.Scopes, CorporationRolesScope) {
		return ErrReauthorize
	}
	owner := httpapi.Hash(ch.Owner)
	sealed, err := s.seal(ch.ID, owner, ch.authorization)
	if err != nil {
		return err
	}
	q := store.New(tx)
	previous, readErr := q.GetCredentialForUpdate(ctx, ch.ID)
	if readErr != nil && !errors.Is(readErr, pgx.ErrNoRows) {
		return readErr
	}
	if readErr == nil && !bytes.Equal(previous.OwnerHash, owner) {
		if err = q.DeleteOnlineHistory(ctx, ch.ID); err != nil {
			return err
		}
		if err = q.DeletePersonalContracts(ctx, ch.ID); err != nil {
			return err
		}
	}
	if err = q.SaveCredential(ctx, store.SaveCredentialParams{CharacterID: ch.ID, OwnerHash: owner, Sealed: sealed, Scopes: ch.authorization.Scopes}); err != nil {
		return err
	}
	generation, err := q.AdvanceGeneration(ctx, ch.ID)
	if err != nil {
		return err
	}
	// Retain display observations only across uninterrupted grants for the same
	// owner. Observed times stay unchanged and freshness is reset; old jobs still
	// cannot publish with their previous generation/fence.
	if readErr == nil && previous.State != "reauthorize" && bytes.Equal(previous.OwnerHash, owner) {
		if err = q.RetainSkillSnapshots(ctx, store.RetainSkillSnapshotsParams{
			CharacterID: ch.ID, PreviousGeneration: previous.GrantGeneration, PreviousScopes: previous.Scopes,
		}); err != nil {
			return err
		}
	}
	if err = q.ResetTokenObservation(ctx, store.ResetTokenObservationParams{CharacterID: ch.ID, ExpiresAt: timestamp(ch.authorization.ExpiresAt)}); err != nil {
		return err
	}
	if err = q.InsertTokenEvent(ctx, store.InsertTokenEventParams{CharacterID: ch.ID, Generation: generation, Outcome: "authorized"}); err != nil {
		return err
	}
	if err = q.DeletePrivateCache(ctx, ch.ID); err != nil {
		return err
	}
	if err = q.SeedSyncTargets(ctx, store.SeedSyncTargetsParams{CharacterID: ch.ID, DisplayName: ch.Name}); err != nil {
		return err
	}
	if s.sync != nil {
		return s.sync.scheduleCharacter(ctx, tx, ch.ID)
	}
	return nil
}

// RemoveCharacterTx deletes usable ESI state and pending member flows on unlink.
func RemoveCharacterTx(ctx context.Context, tx pgx.Tx, id int64, userID string) error {
	q := store.New(tx)
	if err := q.DeletePersonalContracts(ctx, id); err != nil {
		return err
	}
	if err := q.DeleteCredential(ctx, id); err != nil {
		return err
	}
	var user pgtype.UUID
	if err := user.Scan(userID); err != nil {
		return err
	}
	return q.RemoveUserFlows(ctx, user)
}

func (s *AuthorizationService) Revoke(ctx context.Context, id int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	q := store.New(tx)
	if err = q.RevokeCredential(ctx, id); err != nil {
		return err
	}
	if err = q.InvalidateSnapshot(ctx, id); err != nil {
		return err
	}
	if _, err = q.AdvanceGeneration(ctx, id); err != nil {
		return err
	}
	if err = q.BlockCharacterSync(ctx, store.BlockCharacterSyncParams{CharacterID: id, Reason: "reauthorize"}); err != nil {
		return err
	}
	if err = q.DeletePrivateCache(ctx, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Authorization is a token-free DTO; identity ownership must be checked by the host.
type Authorization struct {
	Scopes                           []string
	CharacterID                      int64
	State                            string
	OwnerHash                        []byte
	CorporationID, AllianceID, CEOID int64
	CorporationName                  string
	Roles, HQ, Base, Other           []string
	SyncedAt, ValidUntil             time.Time
}

func (s *AuthorizationService) Get(ctx context.Context, id int64) (Authorization, error) {
	a := Authorization{CharacterID: id, State: "reauthorize", Roles: []string{}, HQ: []string{}, Base: []string{}, Other: []string{}}
	row, err := store.New(s.pool).GetAuthorization(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, nil
	}
	if err != nil {
		return a, err
	}
	a.State = row.State
	a.OwnerHash = row.OwnerHash
	a.Scopes = row.Scopes
	a.CorporationID = row.CorporationID.Int64
	a.CorporationName = row.CorporationName.String
	a.AllianceID = row.AllianceID.Int64
	a.CEOID = row.CeoID.Int64
	if row.CorporationID.Valid {
		a.Roles = row.Roles
		a.HQ = row.RolesAtHq
		a.Base = row.RolesAtBase
		a.Other = row.RolesAtOther
		a.SyncedAt = row.SyncedAt.Time
		a.ValidUntil = row.ValidUntil.Time
	}
	if a.State == "ready" && !a.ValidUntil.After(time.Now()) {
		a.State = "stale"
	}
	return a, nil
}

// MemberAuthorizations returns only current ownership and corporation facts.
// Missing characters are omitted, matching Get's reauthorization fallback.
func (s *AuthorizationService) MemberAuthorizations(ctx context.Context, ids []int64) (map[int64]Authorization, error) {
	rows, err := store.New(s.pool).MemberAuthorizations(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]Authorization, len(rows))
	for _, row := range rows {
		out[row.CharacterID] = Authorization{
			CharacterID:   row.CharacterID,
			State:         row.State,
			OwnerHash:     row.OwnerHash,
			CorporationID: row.CorporationID,
			ValidUntil:    row.ValidUntil,
		}
	}
	return out, nil
}
func (s *AuthorizationService) Corporation(ctx context.Context, id int64) (Authorization, error) {
	row, err := store.New(s.pool).CorporationSnapshot(ctx, id)
	return Authorization{CorporationID: row.CorporationID, CorporationName: row.CorporationName, AllianceID: row.AllianceID, CEOID: row.CeoID}, err
}

// ReadAuthorization does not need token decryption, including when SSO is disabled.
func ReadAuthorization(pool *pgxpool.Pool) *AuthorizationService {
	return &AuthorizationService{pool: pool}
}

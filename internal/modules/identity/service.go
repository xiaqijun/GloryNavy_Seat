package identity

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/httpapi"
	"glorynavy.local/seat/internal/modules/identity/internal/store"
)

var ErrOwnership = errors.New("character ownership needs review")

const SessionLifetime = 7 * 24 * time.Hour
const SessionRenewInterval = 5 * time.Minute

type Service struct {
	pool              *pgxpool.Pool
	MergeParticipants map[string]MergeParticipant
}

func New(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

type Character struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type Session struct {
	UserID        string    `json:"user_id"`
	Character     Character `json:"character"`
	MainCharacter Character `json:"main_character"`
	CSRFToken     string    `json:"csrf_token"`
	ExpiresAt     time.Time `json:"expires_at"`
}

// SignIn accepts an identity already verified by the trusted EVE adapter.
// It does not link different characters or grant corporation permissions.
func (s *Service) SignIn(ctx context.Context, id int64, name, owner, previousToken string) (string, error) {
	return s.Complete(ctx, id, name, owner, previousToken, LoginIntent{Kind: "login"}, nil)
}

func (s *Service) Session(ctx context.Context, token string) (*Session, error) {
	if token == "" {
		return nil, nil
	}
	row, err := store.New(s.pool).GetSession(ctx, httpapi.Hash(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &Session{UserID: row.UserID.String(), Character: Character{ID: strconv.FormatInt(row.CharacterID, 10), Name: row.Name}, MainCharacter: Character{ID: strconv.FormatInt(row.MainID, 10), Name: row.MainName}, CSRFToken: row.CsrfToken, ExpiresAt: row.ExpiresAt.Time}, nil
}
func (s *Service) Logout(ctx context.Context, token string) error {
	return store.New(s.pool).DeleteSession(ctx, httpapi.Hash(token))
}

// Called after browser authentication and request-level checks, not by read-only
// session inspection or an SSO callback. Coalesce frequent polling writes.
func (s *Service) RenewSession(ctx context.Context, token string, session *Session) (bool, error) {
	if session == nil || token == "" {
		return false, pgx.ErrNoRows
	}
	if time.Until(session.ExpiresAt) > SessionLifetime-SessionRenewInterval {
		return false, nil
	}
	expires, err := store.RenewSession(ctx, s.pool, httpapi.Hash(token), int64(SessionLifetime/time.Second))
	if err != nil {
		return false, err
	}
	session.ExpiresAt = expires
	return true, nil
}

type OwnedCharacter struct {
	Name      string
	ID        int64
	OwnerHash []byte
}

func (s *Service) ActiveCharacters(ctx context.Context, user string) ([]OwnedCharacter, error) {
	var id pgtype.UUID
	if err := id.Scan(user); err != nil {
		return nil, err
	}
	rows, err := store.New(s.pool).ActiveCharacters(ctx, id)
	if err != nil {
		return nil, err
	}
	result := make([]OwnedCharacter, 0, len(rows))
	for _, row := range rows {
		result = append(result, OwnedCharacter{Name: row.Name, ID: row.CharacterID, OwnerHash: row.OwnerHash})
	}
	return result, nil
}
func (s *Service) UserForCharacter(ctx context.Context, id int64) (string, error) {
	user, err := store.New(s.pool).UserForCharacter(ctx, id)
	return user.String(), err
}

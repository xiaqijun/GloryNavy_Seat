package reminder

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/reminder/internal/store"
)

var ErrEmailCode = errors.New("email code invalid or expired")
var ErrEmailRate = errors.New("email code rate limited")

func (s *Service) EmailAccount(ctx context.Context, user string) (store.EmailAccount, error) {
	a, err := store.ReadEmailAccount(ctx, s.Pool, user, false)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.EmailAccount{Revision: 1}, nil
	}
	return a, err
}

func emailCodeHash(user, code string, salt []byte) []byte {
	h := sha256.New()
	_, _ = h.Write([]byte("reminder-email-code-v1:"))
	_, _ = h.Write(salt)
	_, _ = h.Write([]byte(user))
	_, _ = h.Write([]byte(code))
	return h.Sum(nil)
}

func (s *Service) RequestEmailCode(ctx context.Context, user, address string) error {
	address = strings.TrimSpace(address)
	if _, err := smtpMailbox(address); err != nil {
		return ErrEmailConfig
	}
	c, err := store.ReadEmailSettings(ctx, s.Pool, false)
	if err != nil {
		return err
	}
	if !c.Enabled {
		return ErrEmailConfig
	}
	secret, err := s.openEmailSecret(c.Secret)
	if err != nil {
		return err
	}
	codeNumber, err := rand.Int(rand.Reader, big.NewInt(100000000))
	if err != nil {
		return err
	}
	code := fmt.Sprintf("%08d", codeNumber.Int64())
	salt := make([]byte, 32)
	if _, err = rand.Read(salt); err != nil {
		return err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if s.LockAccounts == nil {
		return ErrForbidden
	}
	if err = s.LockAccounts(ctx, tx, []string{user}); err != nil {
		return err
	}
	if err = store.EnsureEmailAccount(ctx, tx, user); err != nil {
		return err
	}
	a, err := store.ReadEmailAccount(ctx, tx, user, true)
	if err != nil {
		return err
	}
	if a.LastCode != nil && time.Since(*a.LastCode) < time.Minute {
		return ErrEmailRate
	}
	current, err := store.ReadEmailSettings(ctx, tx, false)
	if err != nil {
		return err
	}
	if !current.Enabled || current.Revision != c.Revision {
		return ErrVersion
	}
	if err = store.UpdateEmailChallenge(ctx, tx, user, address, emailCodeHash(user, code, salt), salt, time.Now().UTC().Add(10*time.Minute)); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	current, err = store.ReadEmailSettings(ctx, s.Pool, false)
	if err != nil {
		return err
	}
	if !current.Enabled || current.Revision != c.Revision {
		return ErrVersion
	}
	result := s.sendEmail(ctx, smtpConfig{Host: c.Host, Port: c.Port, ImplicitTLS: c.ImplicitTLS, Username: c.Username, Password: secret, From: c.Sender}, address, "荣耀海军邮箱验证码", "验证码："+code+"<br>验证码十分钟内有效。")
	if result.State != "sent" {
		return ErrEmailConfig
	}
	return nil
}

func (s *Service) ConfirmEmailCode(ctx context.Context, user, code string) (store.EmailAccount, error) {
	if len(code) != 8 {
		return store.EmailAccount{}, ErrEmailCode
	}
	for _, c := range code {
		if c < '0' || c > '9' {
			return store.EmailAccount{}, ErrEmailCode
		}
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return store.EmailAccount{}, err
	}
	defer tx.Rollback(context.Background())
	if s.LockAccounts == nil {
		return store.EmailAccount{}, ErrForbidden
	}
	if err = s.LockAccounts(ctx, tx, []string{user}); err != nil {
		return store.EmailAccount{}, err
	}
	a, err := store.ReadEmailAccount(ctx, tx, user, true)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.EmailAccount{}, ErrEmailCode
	}
	if err != nil {
		return store.EmailAccount{}, err
	}
	if a.Expires == nil || time.Now().After(*a.Expires) || a.Attempts >= 5 || a.Pending == "" || subtle.ConstantTimeCompare(emailCodeHash(user, code, a.CodeSalt), a.CodeHash) != 1 {
		if err = store.FailEmailChallenge(ctx, tx, user); err != nil {
			return store.EmailAccount{}, err
		}
		if err = tx.Commit(ctx); err != nil {
			return store.EmailAccount{}, err
		}
		return store.EmailAccount{}, ErrEmailCode
	}
	if err = store.VerifyEmailAccount(ctx, tx, user); err != nil {
		return store.EmailAccount{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return store.EmailAccount{}, err
	}
	return s.EmailAccount(ctx, user)
}

func (s *Service) SaveEmailAccount(ctx context.Context, user string, revision int64, enabled bool) (store.EmailAccount, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return store.EmailAccount{}, err
	}
	defer tx.Rollback(context.Background())
	if s.LockAccounts == nil {
		return store.EmailAccount{}, ErrForbidden
	}
	if err = s.LockAccounts(ctx, tx, []string{user}); err != nil {
		return store.EmailAccount{}, err
	}
	a, err := store.ReadEmailAccount(ctx, tx, user, true)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.EmailAccount{}, ErrEmailCode
	}
	if err != nil {
		return store.EmailAccount{}, err
	}
	if a.Revision != revision {
		return store.EmailAccount{}, ErrVersion
	}
	if enabled && !a.Verified {
		return store.EmailAccount{}, ErrEmailCode
	}
	if err = store.SetEmailEnabled(ctx, tx, user, enabled); err != nil {
		return store.EmailAccount{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return store.EmailAccount{}, err
	}
	return s.EmailAccount(ctx, user)
}

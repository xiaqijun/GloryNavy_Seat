package reminder

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"slices"
	"strings"
	"time"

	"glorynavy.local/seat/internal/modules/reminder/internal/store"
)

var ErrEmailConfig = errors.New("email configuration unavailable")

type EmailSettings struct {
	Enabled          bool     `json:"enabled"`
	Host             string   `json:"host"`
	Port             int      `json:"port"`
	ImplicitTLS      bool     `json:"implicit_tls"`
	Username         string   `json:"username"`
	Sender           string   `json:"sender"`
	SecretConfigured bool     `json:"secret_configured"`
	Revision         int64    `json:"revision,string"`
	Events           []string `json:"events"`
	AvailableEvents  []Event  `json:"available_events"`
}
type EmailSettingsInput struct {
	Enabled     bool     `json:"enabled"`
	Host        string   `json:"host"`
	Port        int      `json:"port"`
	ImplicitTLS bool     `json:"implicit_tls"`
	Username    string   `json:"username"`
	Sender      string   `json:"sender"`
	Secret      string   `json:"secret"`
	Revision    int64    `json:"revision,string"`
	Events      []string `json:"events"`
}

func (s *Service) SetEmailKey(encoded string) error {
	if encoded == "" {
		return nil
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(raw) != 32 {
		return ErrEmailConfig
	}
	key := sha256.Sum256(append([]byte("glorynavy:reminder-email:v1:"), raw...))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return err
	}
	s.emailBox, err = cipher.NewGCM(block)
	return err
}
func (s *Service) sealEmailSecret(secret string) ([]byte, error) {
	if s.emailBox == nil {
		return nil, ErrEmailConfig
	}
	nonce := make([]byte, s.emailBox.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return s.emailBox.Seal(nonce, nonce, []byte(secret), []byte("reminder.email.secret.v1")), nil
}
func (s *Service) openEmailSecret(raw []byte) (string, error) {
	if s.emailBox == nil || len(raw) < s.emailBox.NonceSize() {
		return "", ErrEmailConfig
	}
	plain, err := s.emailBox.Open(nil, raw[:s.emailBox.NonceSize()], raw[s.emailBox.NonceSize():], []byte("reminder.email.secret.v1"))
	if err != nil {
		return "", ErrEmailConfig
	}
	return string(plain), nil
}
func (s *Service) EmailConfiguration(ctx context.Context, user string) (EmailSettings, error) {
	if err := s.admin(ctx, user); err != nil {
		return EmailSettings{}, err
	}
	c, err := store.ReadEmailSettings(ctx, s.Pool, false)
	return EmailSettings{Enabled: c.Enabled, Host: c.Host, Port: c.Port, ImplicitTLS: c.ImplicitTLS, Username: c.Username, Sender: c.Sender, SecretConfigured: len(c.Secret) > 0, Revision: c.Revision, Events: c.Events, AvailableEvents: s.catalog()}, err
}
func (s *Service) SaveEmailConfiguration(ctx context.Context, user string, in EmailSettingsInput) (EmailSettings, error) {
	if err := s.admin(ctx, user); err != nil {
		return EmailSettings{}, err
	}
	in.Host = strings.TrimSpace(in.Host)
	in.Username = strings.TrimSpace(in.Username)
	in.Sender = strings.TrimSpace(in.Sender)
	if in.Port < 1 || in.Port > 65535 || len(in.Host) > 253 || strings.ContainsAny(in.Host, "/\\: \t\r\n") || len(in.Username) > 254 || strings.ContainsAny(in.Username, "\r\n\x00") || len(in.Secret) > 512 || strings.ContainsAny(in.Secret, "\r\n\x00") {
		return EmailSettings{}, ErrEmailConfig
	}
	if in.Host != "" && net.ParseIP(in.Host) == nil {
		for _, part := range strings.Split(in.Host, ".") {
			if part == "" {
				return EmailSettings{}, ErrEmailConfig
			}
			for _, c := range part {
				if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
					return EmailSettings{}, ErrEmailConfig
				}
			}
		}
	}
	if in.Sender != "" {
		if _, err := smtpMailbox(in.Sender); err != nil {
			return EmailSettings{}, ErrEmailConfig
		}
	}
	selected := slices.Clone(in.Events)
	slices.Sort(selected)
	if len(slices.Compact(slices.Clone(selected))) != len(selected) || len(selected) > len(events) {
		return EmailSettings{}, ErrEvents
	}
	for _, kind := range selected {
		if _, ok := eventByID(kind); !ok {
			return EmailSettings{}, ErrEvents
		}
	}
	if in.Enabled && (in.Host == "" || in.Username == "" || in.Sender == "" || len(selected) == 0) {
		return EmailSettings{}, ErrEmailConfig
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return EmailSettings{}, err
	}
	defer tx.Rollback(context.Background())
	if s.LockAccounts == nil {
		return EmailSettings{}, ErrForbidden
	}
	if err = s.LockAccounts(ctx, tx, []string{user}); err != nil {
		return EmailSettings{}, err
	}
	if err = s.admin(ctx, user); err != nil {
		return EmailSettings{}, err
	}
	c, err := store.ReadEmailSettings(ctx, tx, true)
	if err != nil {
		return EmailSettings{}, err
	}
	if c.Revision != in.Revision {
		return EmailSettings{}, ErrVersion
	}
	for _, kind := range selected {
		if s.source(kind) == nil && !slices.Contains(c.Events, kind) {
			return EmailSettings{}, ErrEvents
		}
	}
	if in.Secret != "" {
		c.Secret, err = s.sealEmailSecret(in.Secret)
		if err != nil {
			return EmailSettings{}, err
		}
	} else if c.Host != in.Host || c.Username != in.Username {
		c.Secret = []byte{}
	}
	if in.Enabled && len(c.Secret) == 0 {
		return EmailSettings{}, ErrEmailConfig
	}
	if c.Started == nil {
		c.Started = map[string]time.Time{}
	}
	for _, kind := range selected {
		if !slices.Contains(c.Events, kind) || in.Enabled && (!c.Enabled || c.Started[kind].IsZero()) {
			c.Started[kind] = time.Now().UTC()
		}
	}
	c.Enabled, c.Host, c.Port, c.ImplicitTLS, c.Username, c.Sender, c.Events = in.Enabled, in.Host, in.Port, in.ImplicitTLS, in.Username, in.Sender, selected
	if err = store.SaveEmailSettings(ctx, tx, user, c); err != nil {
		return EmailSettings{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return EmailSettings{}, err
	}
	return s.EmailConfiguration(ctx, user)
}

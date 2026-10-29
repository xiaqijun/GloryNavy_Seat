package community

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/modules/community/internal/store"
)

// This probe reads configuration and connects only when explicitly selected.
// It never dispatches messages or changes the database.
func TestQQGatewayRealProbe(t *testing.T) {
	if os.Getenv("QQ_GATEWAY_REAL_PROBE") != "1" {
		t.Skip("explicit live probe not selected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatal("database connection failed")
	}
	defer pool.Close()
	s := New(pool)
	if err = s.SetQQBotConfigKey(os.Getenv("EVE_TOKEN_KEY")); err != nil {
		t.Fatal("configuration key unavailable")
	}
	row, err := store.New(pool).GetQQBotSettings(ctx)
	if err != nil {
		t.Fatal("settings unavailable")
	}
	secret, err := s.openQQBotSecret(row.SecretCiphertext)
	if err != nil {
		t.Fatal("secret unavailable")
	}
	bot := newOfficialQQBot(row.AppID, secret, row.ApiBase)
	token, err := bot.accessToken(ctx)
	if err != nil {
		t.Fatalf("token request: %v", err)
	}
	endpoint, err := bot.gatewayURL(ctx, token)
	if err != nil {
		t.Fatalf("gateway lookup: %v", err)
	}
	ready := false
	err = qqGatewaySession(ctx, endpoint, token, 1<<25, &qqGatewayResume{}, func(frame qqGatewayFrame, _ []byte) error {
		if frame.Type == "READY" {
			ready = true
			cancel()
		}
		return nil
	})
	if !ready {
		t.Fatalf("gateway did not become ready: %v", err)
	}
}

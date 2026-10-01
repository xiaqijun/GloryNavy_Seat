package community

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestQQBotSecretIsEncryptedAndRoundTrips(t *testing.T) {
	s := New(nil)
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))
	if err := s.SetQQBotConfigKey(key); err != nil {
		t.Fatal(err)
	}
	secret := "secret-value-123"
	ciphertext, err := s.sealQQBotSecret(secret)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ciphertext, []byte(secret)) {
		t.Fatal("ciphertext contains plaintext secret")
	}
	plain, err := s.openQQBotSecret(ciphertext)
	if err != nil || plain != secret {
		t.Fatalf("round trip plain=%q err=%v", plain, err)
	}
}

func TestQQBotAPIBaseValidation(t *testing.T) {
	if got, err := validateQQBotAPIBase("https://api.bot.qq.com/"); err != nil || got != "https://api.bot.qq.com" {
		t.Fatalf("valid API base got=%q err=%v", got, err)
	}
	if _, err := validateQQBotAPIBase("http://example.com"); err == nil {
		t.Fatal("public HTTP API base accepted")
	}
	if got, err := validateQQBotAPIBase("http://127.0.0.1:18080/"); err != nil || got != "http://127.0.0.1:18080" {
		t.Fatalf("loopback API base got=%q err=%v", got, err)
	}
}

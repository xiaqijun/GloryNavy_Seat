-- Module: sentry. Keep the current client key copyable without rotation.
-- The value is authenticated ciphertext encrypted with the server key;
-- plaintext is only returned to the owning account over the protected API.
-- +goose Up
ALTER TABLE sentry_keys
    ADD COLUMN secret_ciphertext bytea NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE sentry_keys DROP COLUMN secret_ciphertext;

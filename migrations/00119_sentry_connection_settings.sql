-- +goose Up
CREATE TABLE sentry_connection_settings (
    singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
    origin text NOT NULL CHECK(origin <> ''),
    token_ciphertext bytea NOT NULL CHECK(octet_length(token_ciphertext) > 0),
    revision bigint NOT NULL CHECK(revision > 0),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM sentry_connection_settings) THEN
        RAISE EXCEPTION 'Sentry connection settings are in use';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE sentry_connection_settings;

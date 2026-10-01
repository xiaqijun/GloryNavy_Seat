-- Official QQ Bot API v2 callback events are authenticated at the HTTP
-- boundary and kept separately from member confirmation records for
-- idempotency and replay audit.
-- +goose Up
CREATE TABLE community_bot_events (
 source text NOT NULL CHECK(length(source) > 0 AND length(source) <= 32),
 event_id text NOT NULL CHECK(length(event_id) > 0 AND length(event_id) <= 160),
 payload_hash bytea NOT NULL CHECK(octet_length(payload_hash)=32),
 received_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(source, event_id)
);
CREATE INDEX community_bot_events_received_at ON community_bot_events(received_at);
-- +goose Down
DROP TABLE community_bot_events;

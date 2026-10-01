-- EVE owns synchronized character killmail evidence; welfare stores only submitted snapshots.
-- +goose Up
CREATE TABLE eve_loss_cursors (
 target_id bigint PRIMARY KEY REFERENCES eve_sync_targets(id) ON DELETE CASCADE,
 generation bigint NOT NULL,
 payload jsonb NOT NULL
);
CREATE TABLE eve_character_killmails (
 character_id bigint NOT NULL REFERENCES eve_credentials(character_id) ON DELETE CASCADE,
 killmail_id bigint NOT NULL CHECK(killmail_id>0),
 owner_hash bytea NOT NULL,
 killmail_hash text NOT NULL,
 victim_id bigint NOT NULL,
 occurred_at timestamptz NOT NULL,
 observed_at timestamptz NOT NULL,
 payload jsonb,
 PRIMARY KEY(character_id,killmail_id)
);
CREATE INDEX eve_character_losses_recent ON eve_character_killmails(character_id,killmail_id DESC) WHERE payload IS NOT NULL;
-- +goose Down
DELETE FROM eve_sync_targets WHERE resource='killmails';
DROP TABLE eve_loss_cursors;
DROP TABLE eve_character_killmails;

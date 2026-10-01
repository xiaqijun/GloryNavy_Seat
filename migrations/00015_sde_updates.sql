-- Module: eve. Durable SDE update clock, ownership fence and operator pin.
-- +goose Up
CREATE TABLE eve_sde_update_state (
 singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
 pinned boolean NOT NULL DEFAULT false,
 fence bigint NOT NULL DEFAULT 0,
 lease_until timestamptz NOT NULL DEFAULT 'epoch',
 next_check_at timestamptz NOT NULL DEFAULT now(),
 last_checked_at timestamptz,
 last_success_at timestamptz,
 last_status text NOT NULL DEFAULT 'never',
 last_error text NOT NULL DEFAULT '',
 failure_count integer NOT NULL DEFAULT 0,
 observed_build bigint NOT NULL DEFAULT 0
);
INSERT INTO eve_sde_update_state(singleton) VALUES(true);
-- +goose Down
DROP TABLE eve_sde_update_state;

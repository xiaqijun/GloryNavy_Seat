-- +goose Up
CREATE TABLE platform_metadata (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    schema_version integer NOT NULL CHECK (schema_version > 0),
    environment text NOT NULL CHECK (environment = 'tranquility'),
    created_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO platform_metadata (schema_version, environment)
VALUES (1, 'tranquility');

-- +goose Down
DROP TABLE platform_metadata;

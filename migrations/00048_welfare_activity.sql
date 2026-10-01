-- +goose Up
CREATE SEQUENCE welfare_activity_project_ids;
CREATE TABLE welfare_activity_images (
 case_id bigint NOT NULL REFERENCES welfare_cases(id) ON DELETE RESTRICT,
 ordinal smallint NOT NULL CHECK (ordinal BETWEEN 1 AND 3),
 mime text NOT NULL CHECK (mime IN ('image/jpeg','image/png','image/webp')),
 content bytea NOT NULL CHECK (octet_length(content) BETWEEN 1 AND 2097152),
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(case_id, ordinal)
);
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM welfare_cases WHERE kind LIKE 'activity_%')
 OR EXISTS(SELECT 1 FROM welfare_policies WHERE kind LIKE 'activity_%')
 THEN RAISE EXCEPTION 'activity welfare projects or cases require schema 48'; END IF;
END $$;
-- +goose StatementEnd
DROP TABLE welfare_activity_images;
DROP SEQUENCE welfare_activity_project_ids;

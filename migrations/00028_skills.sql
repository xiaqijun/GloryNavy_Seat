-- +goose Up
ALTER TABLE eve_fitting_snapshots DROP CONSTRAINT eve_fitting_snapshots_resource_check;
ALTER TABLE eve_fitting_snapshots ADD CONSTRAINT eve_fitting_snapshots_resource_check CHECK(resource IN ('fittings','skills','skillqueue'));
CREATE TABLE skills_plans (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 corporation_id bigint NOT NULL CHECK(corporation_id>0),
 name text NOT NULL CHECK(char_length(name) BETWEEN 1 AND 80),
 requirements jsonb NOT NULL CHECK(jsonb_typeof(requirements)='array'),
 version bigint NOT NULL DEFAULT 1,
 created_by uuid NOT NULL,
 request_key uuid NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(created_by,request_key)
);
CREATE INDEX skills_plans_corporation ON skills_plans(corporation_id,id DESC);
CREATE TABLE skills_plan_audit (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 plan_id bigint NOT NULL,
 corporation_id bigint NOT NULL,
 actor_id uuid NOT NULL,
 action text NOT NULL CHECK(action IN ('create','update','delete')),
 before_value jsonb,
 after_value jsonb,
 created_at timestamptz NOT NULL DEFAULT now()
);
-- +goose Down
DROP TABLE skills_plan_audit;
DROP TABLE skills_plans;
DELETE FROM eve_sync_targets WHERE resource='skillqueue';
DELETE FROM eve_fitting_snapshots WHERE resource='skillqueue';
ALTER TABLE eve_fitting_snapshots DROP CONSTRAINT eve_fitting_snapshots_resource_check;
ALTER TABLE eve_fitting_snapshots ADD CONSTRAINT eve_fitting_snapshots_resource_check CHECK(resource IN ('fittings','skills'));

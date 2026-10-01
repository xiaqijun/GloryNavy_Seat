-- Module: eve. Immutable SDE name releases and an atomic active pointer.
-- +goose Up
CREATE TABLE eve_sde_name_releases (
  id bigserial PRIMARY KEY,
  build_number bigint NOT NULL CHECK(build_number>0),
  sha256 text NOT NULL CHECK(length(sha256)=64),
  mapper_version integer NOT NULL DEFAULT 1,
  type_count bigint NOT NULL DEFAULT 0,
  imported_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(build_number,sha256,mapper_version)
);
CREATE TABLE eve_sde_type_names (
  release_id bigint NOT NULL REFERENCES eve_sde_name_releases(id) ON DELETE CASCADE,
  type_id bigint NOT NULL CHECK(type_id>=0),
  name_zh text NOT NULL,
  name_en text NOT NULL CHECK(name_en<>''),
  PRIMARY KEY(release_id,type_id)
);
CREATE TABLE eve_sde_active_names (
  singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
  release_id bigint NOT NULL REFERENCES eve_sde_name_releases(id)
);
-- +goose Down
DROP TABLE eve_sde_active_names;
DROP TABLE eve_sde_type_names;
DROP TABLE eve_sde_name_releases;

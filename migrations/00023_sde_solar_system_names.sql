-- Module: eve. Solar-system names share the immutable SDE release pointer.
-- +goose Up
ALTER TABLE eve_sde_name_releases ADD COLUMN system_count bigint NOT NULL DEFAULT 0;
CREATE TABLE eve_sde_solar_system_names (
 release_id bigint NOT NULL REFERENCES eve_sde_name_releases(id) ON DELETE CASCADE,
 solar_system_id bigint NOT NULL CHECK(solar_system_id>0),
 name_zh text NOT NULL,
 name_en text NOT NULL CHECK(name_en<>''),
 PRIMARY KEY(release_id,solar_system_id)
);
-- +goose Down
DROP TABLE eve_sde_solar_system_names;
ALTER TABLE eve_sde_name_releases DROP COLUMN system_count;

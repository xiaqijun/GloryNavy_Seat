-- name: GetPlatformMetadata :one
SELECT schema_version, environment FROM platform_metadata WHERE singleton = true;

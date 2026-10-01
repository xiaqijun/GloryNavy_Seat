-- Module: eve. Keep large history index builds outside a write-blocking transaction.
-- +goose NO TRANSACTION
-- +goose Up
-- A failed concurrent build can leave an invalid index. Rebuild these migration-owned
-- names on retry rather than skipping an invalid index with IF NOT EXISTS.
DROP INDEX CONCURRENTLY IF EXISTS eve_sync_runs_success_expiry;
CREATE INDEX CONCURRENTLY eve_sync_runs_success_expiry
 ON eve_sync_runs(finished_at,id)
 WHERE outcome='success' AND finished_at IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS eve_sync_runs_other_expiry;
CREATE INDEX CONCURRENTLY eve_sync_runs_other_expiry
 ON eve_sync_runs(finished_at,id)
 WHERE outcome<>'success' AND finished_at IS NOT NULL;

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS eve_sync_runs_other_expiry;
DROP INDEX CONCURRENTLY IF EXISTS eve_sync_runs_success_expiry;

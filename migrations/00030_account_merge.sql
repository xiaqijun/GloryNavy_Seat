-- Modules: identity and participating account-owned data modules. No historical auto-merge.
-- +goose Up
CREATE TABLE identity_account_merges(source_id uuid PRIMARY KEY REFERENCES identity_users(id),target_id uuid NOT NULL REFERENCES identity_users(id),request_id uuid NOT NULL UNIQUE,created_at timestamptz NOT NULL DEFAULT now(),CHECK(source_id<>target_id));
CREATE TABLE identity_merge_requests(id uuid PRIMARY KEY DEFAULT gen_random_uuid(),target_id uuid NOT NULL REFERENCES identity_users(id),source_id uuid NOT NULL REFERENCES identity_users(id),session_hash bytea NOT NULL,proof_character_id bigint NOT NULL,identity_fingerprint text NOT NULL,preview_fingerprint text NOT NULL DEFAULT '',preview jsonb,expires_at timestamptz NOT NULL DEFAULT now()+interval '10 minutes',completed_at timestamptz,CHECK(source_id<>target_id));
CREATE INDEX identity_merge_expiry ON identity_merge_requests(expires_at);
ALTER TABLE eve_login_flows DROP CONSTRAINT eve_login_flows_intent_check, DROP CONSTRAINT eve_flow_intent;
ALTER TABLE eve_login_flows ADD CONSTRAINT eve_login_flows_intent_check CHECK(intent IN ('login','link','reauthorize','merge'));
ALTER TABLE eve_login_flows ADD CONSTRAINT eve_flow_intent CHECK((intent='login' AND target_user_id IS NULL AND expected_character_id IS NULL) OR (intent IN ('link','merge') AND target_user_id IS NOT NULL AND expected_character_id IS NULL) OR (intent='reauthorize' AND target_user_id IS NOT NULL AND expected_character_id>0));
-- Shared identity contract used by module-owned triggers. Lock before checking retirement:
-- an old in-flight request may not write new account-owned records after a merge commits.
-- +goose StatementBegin
CREATE FUNCTION identity_guard_active_account() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE account uuid;
BEGIN
 IF (to_jsonb(NEW)->>TG_ARGV[0]) !~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$' THEN RETURN NEW; END IF;
 account := (to_jsonb(NEW)->>TG_ARGV[0])::uuid;
 IF account IS NULL THEN RETURN NEW; END IF;
 PERFORM 1 FROM identity_users WHERE id=account FOR KEY SHARE;
 IF EXISTS(SELECT 1 FROM identity_account_merges WHERE source_id=account) THEN RAISE EXCEPTION 'account merged; retry with current session' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
ALTER TABLE attendance_entries ADD COLUMN original_account_id uuid;
CREATE TRIGGER attendance_entries_active_account BEFORE INSERT OR UPDATE ON attendance_entries FOR EACH ROW EXECUTE FUNCTION identity_guard_active_account('account_id');
ALTER TABLE attendance_pap_awards ADD COLUMN original_account_id uuid;
CREATE TRIGGER attendance_pap_awards_active_account BEFORE INSERT OR UPDATE ON attendance_pap_awards FOR EACH ROW EXECUTE FUNCTION identity_guard_active_account('account_id');
ALTER TABLE attendance_pap_ledger ADD COLUMN original_account_id uuid;
CREATE TRIGGER attendance_pap_ledger_active_account BEFORE INSERT OR UPDATE ON attendance_pap_ledger FOR EACH ROW EXECUTE FUNCTION identity_guard_active_account('account_id');
ALTER TABLE attendance_battle_tasks ADD COLUMN original_account_id uuid;
CREATE TRIGGER attendance_battle_tasks_active_account BEFORE INSERT OR UPDATE ON attendance_battle_tasks FOR EACH ROW EXECUTE FUNCTION identity_guard_active_account('account_id');
ALTER TABLE exchange_source_awards ADD COLUMN original_account_id uuid;
CREATE TRIGGER exchange_source_awards_active_account BEFORE INSERT OR UPDATE ON exchange_source_awards FOR EACH ROW EXECUTE FUNCTION identity_guard_active_account('account_id');
ALTER TABLE exchange_coin_ledger ADD COLUMN original_account_id uuid;
CREATE TRIGGER exchange_coin_ledger_active_account BEFORE INSERT OR UPDATE ON exchange_coin_ledger FOR EACH ROW EXECUTE FUNCTION identity_guard_active_account('account_id');
ALTER TABLE exchange_redemptions ADD COLUMN original_account_id uuid;
CREATE TRIGGER exchange_redemptions_active_account BEFORE INSERT OR UPDATE ON exchange_redemptions FOR EACH ROW EXECUTE FUNCTION identity_guard_active_account('account_id');
ALTER TABLE fittings_drafts ADD COLUMN original_account_id uuid;
CREATE TRIGGER fittings_drafts_active_account BEFORE INSERT OR UPDATE ON fittings_drafts FOR EACH ROW EXECUTE FUNCTION identity_guard_active_account('account_id');
ALTER TABLE fittings_game_saves ADD COLUMN original_account_id uuid;
CREATE TRIGGER fittings_game_saves_active_account BEFORE INSERT OR UPDATE ON fittings_game_saves FOR EACH ROW EXECUTE FUNCTION identity_guard_active_account('account_id');
CREATE TRIGGER identity_characters_active_account BEFORE INSERT OR UPDATE ON identity_characters FOR EACH ROW EXECUTE FUNCTION identity_guard_active_account('user_id');
CREATE TRIGGER identity_sessions_active_account BEFORE INSERT OR UPDATE ON identity_sessions FOR EACH ROW EXECUTE FUNCTION identity_guard_active_account('user_id');
CREATE TRIGGER community_profiles_active_account BEFORE INSERT OR UPDATE ON community_profiles FOR EACH ROW EXECUTE FUNCTION identity_guard_active_account('user_id');
CREATE TRIGGER access_assignments_active_account BEFORE INSERT OR UPDATE ON access_assignments FOR EACH ROW EXECUTE FUNCTION identity_guard_active_account('user_id');
CREATE TRIGGER access_administrators_active_account BEFORE INSERT OR UPDATE ON access_administrators FOR EACH ROW EXECUTE FUNCTION identity_guard_active_account('user_id');
CREATE TRIGGER attendance_audit_active_actor BEFORE INSERT ON attendance_audit FOR EACH ROW EXECUTE FUNCTION identity_guard_active_account('actor_id');
CREATE TRIGGER exchange_shop_audit_active_actor BEFORE INSERT ON exchange_shop_audit FOR EACH ROW EXECUTE FUNCTION identity_guard_active_account('actor_id');
CREATE TRIGGER fittings_library_audit_active_actor BEFORE INSERT ON fittings_library_audit FOR EACH ROW EXECUTE FUNCTION identity_guard_active_account('actor_id');
CREATE TRIGGER skills_plan_audit_active_actor BEFORE INSERT ON skills_plan_audit FOR EACH ROW EXECUTE FUNCTION identity_guard_active_account('actor_id');
CREATE TRIGGER community_profile_events_active_actor BEFORE INSERT ON community_profile_events FOR EACH ROW EXECUTE FUNCTION identity_guard_active_account('user_id');
CREATE TRIGGER community_confirmations_active_actor BEFORE INSERT ON community_confirmations FOR EACH ROW EXECUTE FUNCTION identity_guard_active_account('user_id');
CREATE TRIGGER access_audit_active_actor BEFORE INSERT ON access_audit FOR EACH ROW EXECUTE FUNCTION identity_guard_active_account('actor');
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM identity_account_merges) THEN RAISE EXCEPTION 'Cannot remove account merge provenance after completed merges'; END IF;
END $$;
-- +goose StatementEnd
DROP TRIGGER access_audit_active_actor ON access_audit;
DROP TRIGGER attendance_audit_active_actor ON attendance_audit;
DROP TRIGGER exchange_shop_audit_active_actor ON exchange_shop_audit;
DROP TRIGGER fittings_library_audit_active_actor ON fittings_library_audit;
DROP TRIGGER skills_plan_audit_active_actor ON skills_plan_audit;
DROP TRIGGER community_profile_events_active_actor ON community_profile_events;
DROP TRIGGER community_confirmations_active_actor ON community_confirmations;
DROP TRIGGER identity_characters_active_account ON identity_characters;
DROP TRIGGER identity_sessions_active_account ON identity_sessions;
DROP TRIGGER community_profiles_active_account ON community_profiles;
DROP TRIGGER access_assignments_active_account ON access_assignments;
DROP TRIGGER access_administrators_active_account ON access_administrators;
DROP TRIGGER attendance_entries_active_account ON attendance_entries;
ALTER TABLE attendance_entries DROP COLUMN original_account_id;
DROP TRIGGER attendance_pap_awards_active_account ON attendance_pap_awards;
ALTER TABLE attendance_pap_awards DROP COLUMN original_account_id;
DROP TRIGGER attendance_pap_ledger_active_account ON attendance_pap_ledger;
ALTER TABLE attendance_pap_ledger DROP COLUMN original_account_id;
DROP TRIGGER attendance_battle_tasks_active_account ON attendance_battle_tasks;
ALTER TABLE attendance_battle_tasks DROP COLUMN original_account_id;
DROP TRIGGER exchange_source_awards_active_account ON exchange_source_awards;
ALTER TABLE exchange_source_awards DROP COLUMN original_account_id;
DROP TRIGGER exchange_coin_ledger_active_account ON exchange_coin_ledger;
ALTER TABLE exchange_coin_ledger DROP COLUMN original_account_id;
DROP TRIGGER exchange_redemptions_active_account ON exchange_redemptions;
ALTER TABLE exchange_redemptions DROP COLUMN original_account_id;
DROP TRIGGER fittings_drafts_active_account ON fittings_drafts;
ALTER TABLE fittings_drafts DROP COLUMN original_account_id;
DROP TRIGGER fittings_game_saves_active_account ON fittings_game_saves;
ALTER TABLE fittings_game_saves DROP COLUMN original_account_id;
DROP FUNCTION identity_guard_active_account();
DELETE FROM eve_login_flows WHERE intent='merge';
ALTER TABLE eve_login_flows DROP CONSTRAINT eve_login_flows_intent_check, DROP CONSTRAINT eve_flow_intent;
ALTER TABLE eve_login_flows ADD CONSTRAINT eve_login_flows_intent_check CHECK(intent IN ('login','link','reauthorize'));
ALTER TABLE eve_login_flows ADD CONSTRAINT eve_flow_intent CHECK((intent='login' AND target_user_id IS NULL AND expected_character_id IS NULL) OR (intent='link' AND target_user_id IS NOT NULL AND expected_character_id IS NULL) OR (intent='reauthorize' AND target_user_id IS NOT NULL AND expected_character_id>0));
DROP TABLE identity_merge_requests;
DROP TABLE identity_account_merges;

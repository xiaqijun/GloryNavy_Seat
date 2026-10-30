-- Module: eve. Preserve the purpose of pending SSO flows and guard full-scope grants.
-- +goose Up
ALTER TABLE eve_login_flows ADD COLUMN purpose text NOT NULL DEFAULT ''
 CHECK (purpose IN ('', 'reminder-mail', 'fleet-write'));
UPDATE eve_login_flows SET purpose = CASE
 WHEN intent = 'reauthorize' AND 'esi-mail.send_mail.v1' = ANY(requested_scopes) THEN 'reminder-mail'
 WHEN intent = 'reauthorize' AND 'esi-fleets.write_fleet.v1' = ANY(requested_scopes) THEN 'fleet-write'
 ELSE '' END;
CREATE TABLE eve_full_scope_grant_state (
 singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
 full_grants_seen boolean NOT NULL DEFAULT false
);
INSERT INTO eve_full_scope_grant_state(singleton, full_grants_seen)
SELECT true, EXISTS (SELECT 1 FROM eve_credentials WHERE scopes && ARRAY[
 'esi-access.read_lists.v1','esi-activities.read_character.v1',
 'esi-calendar.respond_calendar_events.v1','esi-characters.read_freelance_jobs.v1',
 'esi-characters.write_contacts.v1','esi-corporations.read_freelance_jobs.v1',
 'esi-mail.organize_mail.v1','esi-structures.read_character.v1',
 'esi-structures.read_corporation.v1','esi-ui.write_waypoint.v1',
 'esi.activity.char:read','esi.cosmetic.char:read']);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF (SELECT full_grants_seen FROM eve_full_scope_grant_state WHERE singleton)
    OR EXISTS (SELECT 1 FROM eve_login_flows WHERE expires_at > now() AND
      (purpose <> '' OR requested_scopes && ARRAY[
       'esi-access.read_lists.v1','esi-activities.read_character.v1',
       'esi-calendar.respond_calendar_events.v1','esi-characters.read_freelance_jobs.v1',
       'esi-characters.write_contacts.v1','esi-corporations.read_freelance_jobs.v1',
       'esi-mail.organize_mail.v1','esi-structures.read_character.v1',
       'esi-structures.read_corporation.v1','esi-ui.write_waypoint.v1',
       'esi.activity.char:read','esi.cosmetic.char:read'])) THEN
  RAISE EXCEPTION 'Full ESI grants or pending authorization flows exist; rollback would lose scope protection';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE eve_full_scope_grant_state;
ALTER TABLE eve_login_flows DROP COLUMN purpose;

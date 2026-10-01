-- attendance-owned global monthly requirement; no PAP or coin balances are changed.
-- +goose Up
CREATE TABLE attendance_pap_requirement (
  singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
  monthly_points integer NOT NULL CHECK (monthly_points BETWEEN 1 AND 100000),
  version bigint NOT NULL CHECK (version > 0)
);
INSERT INTO attendance_pap_requirement VALUES (true, 3, 1);
CREATE TABLE attendance_pap_requirement_audit (
  version bigint PRIMARY KEY,
  actor_id uuid NOT NULL,
  previous_points integer NOT NULL,
  monthly_points integer NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
-- +goose Down
DROP TABLE attendance_pap_requirement_audit;
DROP TABLE attendance_pap_requirement;

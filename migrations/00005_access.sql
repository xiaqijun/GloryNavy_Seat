-- Module: access. Site administration is independent from EVE corporation roles.
-- +goose Up
CREATE TABLE access_administrators (
    user_id uuid PRIMARY KEY REFERENCES identity_users(id),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE access_roles (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL UNIQUE CHECK (length(name) BETWEEN 1 AND 80),
    grants jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(grants) = 'array'),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE access_assignments (
    user_id uuid NOT NULL REFERENCES identity_users(id),
    role_id uuid NOT NULL REFERENCES access_roles(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, role_id)
);
CREATE TABLE access_audit (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor text NOT NULL,
    action text NOT NULL,
    subject text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
-- +goose Down
DROP TABLE access_audit;
DROP TABLE access_assignments;
DROP TABLE access_roles;
DROP TABLE access_administrators;

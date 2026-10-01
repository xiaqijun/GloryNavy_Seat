-- name: IsAdministrator :one
SELECT EXISTS(SELECT 1 FROM access_administrators WHERE user_id=$1);
-- name: SetAdministrator :exec
INSERT INTO access_administrators(user_id) VALUES($1) ON CONFLICT DO NOTHING;
-- name: RemoveAdministrator :exec
DELETE FROM access_administrators WHERE user_id=$1;
-- name: ListRoles :many
SELECT * FROM access_roles ORDER BY name;
-- name: UserRoles :many
SELECT r.* FROM access_roles r JOIN access_assignments a ON a.role_id=r.id WHERE a.user_id=$1 ORDER BY r.name;
-- name: CreateRole :one
INSERT INTO access_roles(id,name,grants) VALUES($1,$2,$3) ON CONFLICT(id) DO NOTHING RETURNING *;
-- name: UpdateRole :one
UPDATE access_roles SET name=$2,grants=$3,version=version+1,updated_at=now()
WHERE id=$1 AND version=$4 RETURNING *;
-- name: DeleteRole :execrows
DELETE FROM access_roles WHERE id=$1 AND version=$2;
-- name: AssignRole :exec
INSERT INTO access_assignments(user_id,role_id) VALUES($1,$2) ON CONFLICT DO NOTHING;
-- name: UnassignRole :exec
DELETE FROM access_assignments WHERE user_id=$1 AND role_id=$2;
-- name: Audit :exec
INSERT INTO access_audit(actor,action,subject) VALUES($1,$2,$3);
-- name: ListAudit :many
SELECT * FROM access_audit WHERE id < $1 ORDER BY id DESC LIMIT 51;

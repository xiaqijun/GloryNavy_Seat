import { apiFetch } from "@/lib/http";
import { msg } from "@/lib/i18n";
import { APIError, getData } from "@/lib/http";

export interface Grant {
  permission: string;
  corporations: string[];
  alliances: string[];
}
export interface Role {
  id: string;
  name: string;
  version: string;
  grants: Grant[];
}
export interface Permission {
  id: string;
  label: string;
  scope: "self" | "global" | "corporation";
}
export interface Member {
  user_id: string;
  character_id: string;
  name: string;
  character_count: number;
  administrator: boolean;
  roles: Role[];
}
export interface AuditEntry {
  id: string;
  actor: string;
  action: string;
  subject: string;
  created_at: string;
}
export interface ListPage<T> {
  items: T[];
  next: string;
}
const record = (v: unknown): v is Record<string, unknown> =>
  !!v && typeof v === "object";
const strings = (v: unknown): v is string[] =>
  Array.isArray(v) && v.every((x) => typeof x === "string");
const isGrant = (v: unknown): v is Grant =>
  record(v) &&
  typeof v.permission === "string" &&
  strings(v.corporations) &&
  strings(v.alliances);
export const isRole = (v: unknown): v is Role =>
  record(v) &&
  typeof v.id === "string" &&
  typeof v.name === "string" &&
  typeof v.version === "string" &&
  /^[1-9]\d*$/.test(v.version) &&
  Array.isArray(v.grants) &&
  v.grants.every(isGrant);
const isMember = (v: unknown): v is Member =>
  record(v) &&
  typeof v.user_id === "string" &&
  typeof v.character_id === "string" &&
  typeof v.name === "string" &&
  Number.isInteger(v.character_count) &&
  typeof v.administrator === "boolean" &&
  Array.isArray(v.roles) &&
  v.roles.every(isRole);
const isAudit = (v: unknown): v is AuditEntry =>
  record(v) &&
  [v.id, v.actor, v.action, v.subject, v.created_at].every(
    (x) => typeof x === "string",
  ) &&
  Number.isFinite(Date.parse(String(v.created_at)));
const list =
  <T>(check: (v: unknown) => v is T) =>
  (v: unknown): v is ListPage<T> =>
    record(v) &&
    typeof v.next === "string" &&
    Array.isArray(v.items) &&
    v.items.every(check);
export const getRoles = (signal?: AbortSignal) =>
  getData(
    "/api/v1/access/roles",
    (v): v is Role[] => Array.isArray(v) && v.every(isRole),
    signal,
  );
export const getPermissions = (signal?: AbortSignal) =>
  getData(
    "/api/v1/access/catalog",
    (v): v is Permission[] =>
      Array.isArray(v) &&
      v.every(
        (p) =>
          record(p) &&
          typeof p.id === "string" &&
          typeof p.label === "string" &&
          ["self", "global", "corporation"].includes(String(p.scope)),
      ),
    signal,
  ).then((permissions) =>
    permissions.map((permission) => ({
      ...permission,
      label: /^钱包分部 [1-7]$/.test(permission.label)
        ? msg("钱包分部 {0}", permission.label.slice(-1))
        : msg(permission.label),
    })),
  );
export const getMembers = (q: string, after: string, signal?: AbortSignal) =>
  getData(
    `/api/v1/access/members?${new URLSearchParams({ q, after })}`,
    list(isMember),
    signal,
  );
export const getAudit = (before: string, signal?: AbortSignal) =>
  getData(
    `/api/v1/access/audit?${new URLSearchParams({ before })}`,
    list(isAudit),
    signal,
  );
async function mutate(
  path: string,
  method: string,
  csrf: string,
  body?: unknown,
) {
  const response = await apiFetch(`/api/v1/access/${path}`, {
    method,
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const payload = await response.json().catch(() => null);
  if (!response.ok || payload?.data?.saved !== true)
    throw new APIError(
      payload?.error?.message ?? msg("操作失败，请重试"),
      response.status,
    );
}
export const saveRole = (r: Role, csrf: string) =>
  mutate(`roles/${encodeURIComponent(r.id)}`, "PUT", csrf, {
    name: r.name,
    grants: r.grants,
    version: r.version,
  });
export const deleteRole = (r: Role, csrf: string) =>
  mutate(
    `roles/${encodeURIComponent(r.id)}?version=${encodeURIComponent(r.version)}`,
    "DELETE",
    csrf,
  );
export const assignRole = (
  user: string,
  role: string,
  enabled: boolean,
  csrf: string,
) =>
  mutate(
    `users/${encodeURIComponent(user)}/roles/${encodeURIComponent(role)}`,
    enabled ? "PUT" : "DELETE",
    csrf,
  );

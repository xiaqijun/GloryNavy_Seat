import { getData } from "@/lib/http";

export interface AuthorizationFact {
  needs_authorization?: boolean;
  character_id: string;
  state: "pending" | "ready" | "retry" | "reauthorize" | "stale";
  corporation: {
    id: string;
    name: string;
    alliance_id: string;
    ceo_id: string;
  };
  roles: string[];
  roles_at_hq: string[];
  roles_at_base: string[];
  roles_at_other: string[];
  synced_at: string;
  valid_until: string;
}
export interface AccountAccess {
  administrator: boolean;
  can_manage?: boolean;
  can_manage_sync?: boolean;
  site_roles: { id: string; name: string }[];
  characters: AuthorizationFact[];
}
const record = (v: unknown): v is Record<string, unknown> =>
  !!v && typeof v === "object";
const id = (v: unknown) => typeof v === "string" && /^\d+$/.test(v);
const strings = (v: unknown) =>
  Array.isArray(v) && v.every((x) => typeof x === "string");
export function isAccountAccess(value: unknown): value is AccountAccess {
  if (
    !record(value) ||
    typeof value.administrator !== "boolean" ||
    (value.can_manage !== undefined && typeof value.can_manage !== "boolean") ||
    (value.can_manage_sync !== undefined &&
      typeof value.can_manage_sync !== "boolean") ||
    !Array.isArray(value.site_roles) ||
    !Array.isArray(value.characters)
  )
    return false;
  return (
    value.site_roles.every(
      (r) =>
        record(r) && typeof r.id === "string" && typeof r.name === "string",
    ) &&
    value.characters.every(
      (f) =>
        record(f) &&
        (f.needs_authorization === undefined ||
          typeof f.needs_authorization === "boolean") &&
        id(f.character_id) &&
        ["pending", "ready", "retry", "reauthorize", "stale"].includes(
          String(f.state),
        ) &&
        record(f.corporation) &&
        id(f.corporation.id) &&
        typeof f.corporation.name === "string" &&
        id(f.corporation.alliance_id) &&
        id(f.corporation.ceo_id) &&
        [f.roles, f.roles_at_hq, f.roles_at_base, f.roles_at_other].every(
          strings,
        ) &&
        typeof f.synced_at === "string" &&
        Number.isFinite(Date.parse(f.synced_at)) &&
        typeof f.valid_until === "string" &&
        Number.isFinite(Date.parse(f.valid_until)),
    )
  );
}
export const getAccountAccess = (signal?: AbortSignal) =>
  getData("/api/v1/access/me", isAccountAccess, signal);

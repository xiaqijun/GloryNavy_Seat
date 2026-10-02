import { apiFetch, APIError, getData } from "@/lib/http";
import { msg } from "@/lib/i18n";

export type Key = {
  id: string;
  name: string;
  prefix: string;
  permissions: ("monitor" | "alert")[];
  status: "creating" | "active" | "revoking" | "revoked" | "sync_error";
  remote_key_id?: string;
  version: number;
  created_at: string;
  updated_at: string;
  revoked_at?: string;
  last_error?: string;
  secret?: string;
};
export type Keys = { items: Key[] };

export type AlertUsage = {
  available_minor: number;
  alert_reserved_minor: number;
  alert_settled_minor: number;
  alert_released_minor: number;
  alert_refunded_minor: number;
  as_of: string;
};

export type AlertPricing = {
  price_version: string;
  unit_seconds: number;
  unit_price_minor: number;
  max_grant_seconds: number;
  grant_ttl_seconds: number;
  version: number;
  configured: boolean;
  charging_enabled: boolean;
  can_edit: boolean;
  updated_at?: string;
};

export type AlertConsumption = {
  id: string;
  grant_id: string;
  interval_id: string;
  started_at: string;
  ended_at: string;
  duration_seconds: number;
  coins_minor: number;
  state: "reserved" | "settled" | "released" | "refunded";
  unit_seconds: number;
  unit_price_minor: number;
  price_version?: string;
  expires_at: string;
};

export type AlertConsumptionPage = {
  items: AlertConsumption[];
  next_cursor: string;
  as_of: string;
};

const object = (v: unknown): v is Record<string, unknown> =>
  !!v && typeof v === "object";
const isKey = (v: unknown): v is Key =>
  object(v) &&
  typeof v.id === "string" &&
  typeof v.name === "string" &&
  typeof v.prefix === "string" &&
  Array.isArray(v.permissions) &&
  v.permissions.every((p) => p === "monitor" || p === "alert") &&
  ["creating", "active", "revoking", "revoked", "sync_error"].includes(
    String(v.status),
  ) &&
  Number.isSafeInteger(v.version) &&
  typeof v.created_at === "string" &&
  typeof v.updated_at === "string";
const isKeys = (v: unknown): v is Keys =>
  object(v) && Array.isArray(v.items) && v.items.every(isKey);

const isUsage = (v: unknown): v is AlertUsage =>
  object(v) &&
  Number.isSafeInteger(v.available_minor) &&
  [
    "alert_reserved_minor",
    "alert_settled_minor",
    "alert_released_minor",
    "alert_refunded_minor",
  ].every((key) => Number.isSafeInteger(v[key]) && Number(v[key]) >= 0) &&
  typeof v.as_of === "string";

const isPricing = (v: unknown): v is AlertPricing =>
  object(v) &&
  typeof v.price_version === "string" &&
  Number.isSafeInteger(v.unit_seconds) &&
  Number.isSafeInteger(v.unit_price_minor) &&
  Number.isSafeInteger(v.max_grant_seconds) &&
  Number.isSafeInteger(v.grant_ttl_seconds) &&
  Number.isSafeInteger(v.version) &&
  typeof v.configured === "boolean" &&
  typeof v.charging_enabled === "boolean" &&
  typeof v.can_edit === "boolean" &&
  (v.updated_at === undefined || typeof v.updated_at === "string") &&
  (v.configured || v.charging_enabled
    ? Number(v.unit_seconds) > 0 &&
      Number(v.unit_price_minor) > 0 &&
      Number(v.max_grant_seconds) > 0 &&
      Number(v.grant_ttl_seconds) >= 60
    : Number(v.unit_seconds) >= 0 &&
      Number(v.unit_price_minor) >= 0 &&
      Number(v.max_grant_seconds) >= 0 &&
      Number(v.grant_ttl_seconds) >= 0);

const isConsumption = (v: unknown): v is AlertConsumption =>
  object(v) &&
  typeof v.id === "string" &&
  typeof v.grant_id === "string" &&
  typeof v.interval_id === "string" &&
  typeof v.started_at === "string" &&
  typeof v.ended_at === "string" &&
  Number.isSafeInteger(v.duration_seconds) &&
  Number(v.duration_seconds) > 0 &&
  Number.isSafeInteger(v.coins_minor) &&
  Number(v.coins_minor) > 0 &&
  ["reserved", "settled", "released", "refunded"].includes(String(v.state)) &&
  Number.isSafeInteger(v.unit_seconds) &&
  Number(v.unit_seconds) > 0 &&
  Number.isSafeInteger(v.unit_price_minor) &&
  Number(v.unit_price_minor) > 0 &&
  (v.price_version === undefined || typeof v.price_version === "string") &&
  typeof v.expires_at === "string";

const isConsumptionPage = (v: unknown): v is AlertConsumptionPage =>
  object(v) &&
  typeof v.next_cursor === "string" &&
  typeof v.as_of === "string" &&
  Array.isArray(v.items) &&
  v.items.every(isConsumption);

export const list = (signal?: AbortSignal) =>
  getData("/api/v1/sentry/keys", isKeys, signal);

export const usage = (signal?: AbortSignal) =>
  getData("/api/v1/sentry/alert-usage", isUsage, signal);

export const pricing = (signal?: AbortSignal) =>
  getData("/api/v1/sentry/alert-pricing", isPricing, signal);

export const consumptions = (
  params: { before?: string; state?: string; from?: string; to?: string },
  signal?: AbortSignal,
) => {
  const query = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value) query.set(key, value);
  }
  return getData(
    `/api/v1/sentry/alert-consumptions?${query.toString()}`,
    isConsumptionPage,
    signal,
  );
};

async function mutate(path: string, csrf: string, method: string, body?: unknown) {
  const r = await apiFetch(path, {
    method,
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const p = await r.json().catch(() => null);
  if (!r.ok) throw new APIError(p?.error?.message || msg("请求失败，请重试"), r.status);
  return p?.data;
}

export const create = (csrf: string, name: string, permissions: string[], requestKey: string) =>
  mutate("/api/v1/sentry/keys", csrf, "POST", {
    name,
    permissions,
    request_key: requestKey,
  }) as Promise<Key>;
export const revoke = (csrf: string, id: string) =>
  mutate(`/api/v1/sentry/keys/${encodeURIComponent(id)}`, csrf, "DELETE");
export const rotate = (csrf: string, id: string) =>
  mutate(`/api/v1/sentry/keys/${encodeURIComponent(id)}/rotate`, csrf, "POST") as Promise<Key>;

export const updatePricing = (
  csrf: string,
  value: Pick<AlertPricing, "price_version" | "unit_seconds" | "unit_price_minor" | "max_grant_seconds" | "grant_ttl_seconds" | "version">,
) => mutate("/api/v1/sentry/alert-pricing", csrf, "PUT", value) as Promise<AlertPricing>;

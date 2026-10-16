import { APIError, apiFetch, getData } from "@/lib/http";

export type CleanupSettings = {
  enabled: boolean;
  corporation_ids: string[];
  version: string;
  next_run_at: string | null;
  run_started_at: string | null;
  last_run_at: string | null;
  removed_count: number;
  skipped_count: number;
  last_removed_count: number;
  last_skipped_count: number;
};
export type CleanupPreview = {
  remove_count: number;
  keep_count: number;
  unknown_count: number;
  items: {
    character_id: string;
    name: string;
    corporation_id: string;
    corporation_name: string;
  }[];
};
const record = (v: unknown): v is Record<string, unknown> =>
  !!v && typeof v === "object";
const id = (v: unknown): v is string =>
  typeof v === "string" && /^[1-9]\d*$/.test(v);
const count = (v: unknown) =>
  typeof v === "number" && Number.isSafeInteger(v) && v >= 0;
const date = (v: unknown) =>
  v === null || (typeof v === "string" && Number.isFinite(Date.parse(v)));
export const isCleanupSettings = (v: unknown): v is CleanupSettings =>
  record(v) &&
  typeof v.enabled === "boolean" &&
  id(v.version) &&
  Array.isArray(v.corporation_ids) &&
  v.corporation_ids.every(id) &&
  [v.next_run_at, v.run_started_at, v.last_run_at].every(date) &&
  [
    v.removed_count,
    v.skipped_count,
    v.last_removed_count,
    v.last_skipped_count,
  ].every(count);
export const isCleanupPreview = (v: unknown): v is CleanupPreview =>
  record(v) &&
  [v.remove_count, v.keep_count, v.unknown_count].every(count) &&
  Array.isArray(v.items) &&
  v.items.every(
    (r) =>
      record(r) &&
      id(r.character_id) &&
      id(r.corporation_id) &&
      typeof r.name === "string" &&
      typeof r.corporation_name === "string",
  );
const path = "/api/v1/identity/corporation-cleanup";
export const getCleanup = (signal?: AbortSignal) =>
  getData(path, isCleanupSettings, signal);
async function write<T>(
  suffix: string,
  method: string,
  value: unknown,
  csrf: string,
  validate: (v: unknown) => v is T,
): Promise<T> {
  const response = await apiFetch(path + suffix, {
    method,
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
    body: JSON.stringify(value),
  });
  const payload = await response.json().catch(() => null);
  if (!response.ok || !validate(payload?.data))
    throw new APIError(
      payload?.error?.message ?? "角色清理服务暂不可用",
      response.status,
    );
  return payload.data;
}
export const saveCleanup = (
  value: Pick<CleanupSettings, "enabled" | "corporation_ids" | "version">,
  csrf: string,
) => write("", "PUT", value, csrf, isCleanupSettings);
export const previewCleanup = (ids: string[], csrf: string) =>
  write("/preview", "POST", { corporation_ids: ids }, csrf, isCleanupPreview);

import { apiFetch } from "@/lib/http";
import { msg } from "@/lib/i18n";
import { getData, APIError } from "@/lib/http";
import type { Draft, Fit, SavedFit } from "./model";
export type Context = {
  characters: { id: string; name: string }[];
  can_edit: boolean;
};
export type Snapshot = {
  status: "pending" | "ready";
  observed_at: string | null;
  fittings: SavedFit[];
  skills: Record<string, number>;
};
export type Named = { id: string; name: string };
const object = (v: unknown): v is Record<string, unknown> =>
  !!v && typeof v === "object";
const id = (v: unknown) => typeof v === "string" && /^[1-9]\d*$/.test(v);
const named = (v: unknown): v is Named & Record<string, unknown> =>
  object(v) && id(v.id) && typeof v.name === "string";
export const isDraft = (v: unknown): v is Draft =>
  object(v) &&
  named(v) &&
  id(v.version) &&
  typeof v.updated_at === "string" &&
  typeof v.can_edit === "boolean" &&
  (v.fit === undefined || isFit(v.fit));
export const isFit = (v: unknown): v is Fit =>
  object(v) &&
  typeof v.name === "string" &&
  id(v.ship_type_id) &&
  ["all5", "all0", "character"].includes(String(v.skill_mode)) &&
  Array.isArray(v.items) &&
  v.items.length <= 512 &&
  v.items.every(
    (i) =>
      object(i) &&
      id(i.type_id) &&
      [
        "high",
        "medium",
        "low",
        "rig",
        "subsystem",
        "service",
        "fighter_tube",
        "fighter_bay",
        "drone_bay",
        "cargo",
      ].includes(String(i.slot)) &&
      Number.isSafeInteger(i.index) &&
      Number(i.index) >= 0 &&
      Number.isSafeInteger(i.quantity) &&
      Number(i.quantity) > 0 &&
      ["offline", "online", "active", "overload"].includes(String(i.state)) &&
      (i.charge_id === undefined || id(i.charge_id)),
  );
const root = "/api/v1/fittings";
export const context = (member: string, signal?: AbortSignal) =>
  getData(
    `${root}/context?${new URLSearchParams({ member })}`,
    (v: unknown): v is Context =>
      object(v) &&
      typeof v.can_edit === "boolean" &&
      Array.isArray(v.characters) &&
      v.characters.every(named),
    signal,
  );
export const snapshot = (
  character: string,
  resource: string,
  signal?: AbortSignal,
) =>
  getData(
    `${root}/saved?${new URLSearchParams({ character_id: character, resource })}`,
    (v: unknown): v is Snapshot =>
      object(v) &&
      ["pending", "ready"].includes(String(v.status)) &&
      (v.observed_at === null || typeof v.observed_at === "string") &&
      Array.isArray(v.fittings) &&
      v.fittings.every(
        (f) =>
          named(f) &&
          object(f) &&
          id(f.ship_type_id) &&
          typeof f.description === "string" &&
          Array.isArray(f.items) &&
          f.items.every(
            (i) =>
              object(i) &&
              id(i.type_id) &&
              typeof i.flag === "string" &&
              Number.isSafeInteger(i.quantity) &&
              Number(i.quantity) > 0,
          ),
      ) &&
      object(v.skills) &&
      Object.entries(v.skills).every(
        ([k, n]) =>
          id(k) && Number.isInteger(n) && Number(n) >= 0 && Number(n) <= 5,
      ),
    signal,
  );
export const drafts = (member: string, before: string, signal?: AbortSignal) =>
  getData(
    `${root}/drafts?${new URLSearchParams({ member, before })}`,
    (v: unknown): v is { items: Draft[]; next_cursor: string } =>
      object(v) &&
      Array.isArray(v.items) &&
      v.items.every(isDraft) &&
      typeof v.next_cursor === "string",
    signal,
  );
export const draft = (id: string) =>
  getData(`${root}/drafts/${encodeURIComponent(id)}`, isDraft);
export const names = (ids: string[], signal?: AbortSignal) =>
  getData(
    `${root}/names?ids=${ids.join(",")}`,
    (v: unknown): v is Named[] => Array.isArray(v) && v.every(named),
    signal,
  );
export const search = (q: string, signal?: AbortSignal) =>
  getData(
    `${root}/types?${new URLSearchParams({ q })}`,
    (v: unknown): v is Named[] => Array.isArray(v) && v.every(named),
    signal,
  );
export async function write(
  path: string,
  method: string,
  csrf: string,
  body: object,
) {
  const r = await apiFetch(root + path, {
    method,
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
    body: JSON.stringify(body),
  });
  const p = await r.json().catch(() => null);
  if (!r.ok)
    throw new APIError(p?.error?.message ?? msg("保存失败，请重试"), r.status);
  if (method === "DELETE") {
    if (p?.data?.saved !== true) throw Error(msg("删除响应异常"));
    return null;
  }
  if (!isDraft(p?.data)) throw Error(msg("保存响应异常，请刷新核对"));
  return p.data;
}

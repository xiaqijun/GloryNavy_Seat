import { apiFetch } from "@/lib/http";
import { msg } from "@/lib/i18n";
import { getData, APIError } from "@/lib/http";
import { isFit } from "./api";
import type { Fit } from "./model";
import type { Requirement } from "@/modules/skills/api";
export type LibraryContext = {
  can_manage: boolean;
  characters: { id: string; name: string; can_save: boolean }[];
  corporations: { id: string; name: string; can_create_skills: boolean }[];
};
export type Entry = {
  id: string;
  corporation_id: string;
  name: string;
  description: string;
  fit: Fit;
  version: string;
  updated_at: string;
  ship_name: string;
  group: string;
  eft: string;
};
export type Import = {
  corporation_id: string;
  description: string;
  eft: string;
  version: string;
  request_key: string;
};
export type GameSave = {
  id: string;
  state: "sending" | "saved" | "failed" | "unknown";
  fitting_id: string | null;
  reason: string;
};
export type Requirements = {
  name: string;
  corporation_id: string;
  build: string;
  requirements: Requirement[];
};
const object = (v: unknown): v is Record<string, unknown> =>
  !!v && typeof v === "object";
const id = (v: unknown) => typeof v === "string" && /^[1-9]\d*$/.test(v);
export const isEntry = (v: unknown): v is Entry =>
  object(v) &&
  id(v.id) &&
  id(v.corporation_id) &&
  id(v.version) &&
  typeof v.name === "string" &&
  typeof v.description === "string" &&
  typeof v.updated_at === "string" &&
  typeof v.ship_name === "string" &&
  typeof v.group === "string" &&
  typeof v.eft === "string" &&
  isFit(v.fit);
export const isSave = (v: unknown): v is GameSave =>
  object(v) &&
  id(v.id) &&
  ["sending", "saved", "failed", "unknown"].includes(String(v.state)) &&
  (v.fitting_id === null || id(v.fitting_id)) &&
  typeof v.reason === "string";
const root = "/api/v1/fittings/library";
export const context = (signal?: AbortSignal) =>
  getData(
    root + "/context",
    (v): v is LibraryContext =>
      object(v) &&
      typeof v.can_manage === "boolean" &&
      Array.isArray(v.characters) &&
      v.characters.every(
        (c) =>
          object(c) &&
          id(c.id) &&
          typeof c.name === "string" &&
          typeof c.can_save === "boolean",
      ) &&
      Array.isArray(v.corporations) &&
      v.corporations.every(
        (c) =>
          object(c) &&
          id(c.id) &&
          typeof c.name === "string" &&
          typeof c.can_create_skills === "boolean",
      ),
    signal,
  );
export const list = (corp: string, signal?: AbortSignal) =>
  getData(
    `${root}?corporation_id=${corp}`,
    (v): v is Entry[] => Array.isArray(v) && v.every(isEntry),
    signal,
  );
export const requirements = (id: string) =>
  getData(
    `${root}/${id}/requirements`,
    (v): v is Requirements =>
      object(v) &&
      typeof v.name === "string" &&
      typeof v.corporation_id === "string" &&
      typeof v.build === "string" &&
      Array.isArray(v.requirements) &&
      v.requirements.every(
        (r) =>
          object(r) &&
          typeof r.skill_id === "string" &&
          Number.isInteger(r.level) &&
          Number(r.level) >= 1 &&
          Number(r.level) <= 5,
      ),
  );
async function write(path: string, method: string, body: object, csrf: string) {
  const r = await apiFetch(root + path, {
    method,
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
    body: JSON.stringify(body),
  });
  const p = await r.json().catch(() => null);
  if (!r.ok)
    throw new APIError(p?.error?.message ?? msg("操作失败，请重试"), r.status);
  return p?.data as unknown;
}
export async function publish(
  id: string,
  body: Import,
  csrf: string,
  remove = false,
) {
  const v = await write(
    id ? `/${id}` : "",
    remove ? "DELETE" : id ? "PUT" : "POST",
    body,
    csrf,
  );
  if (!isEntry(v)) throw Error(msg("方案响应异常"));
  return v;
}
export async function save(
  id: string,
  version: string,
  character: string,
  key: string,
  csrf: string,
) {
  const v = await write(
    `/${id}/save-to-game`,
    "POST",
    { version, character_id: character, request_key: key },
    csrf,
  );
  if (!isSave(v)) throw Error(msg("保存结果未知，请先在游戏中核对"));
  return v;
}

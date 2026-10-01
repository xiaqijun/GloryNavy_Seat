import { apiFetch } from "@/lib/http";
import { msg } from "@/lib/i18n";
import { APIError, getData } from "@/lib/http";
export type Character = { id: string; name: string; corporation_id: string };
export type Corporation = { id: string; name: string; can_manage: boolean };
export type Context = { characters: Character[]; corporations: Corporation[] };
export type Meta = {
  status: "pending" | "blocked" | "ready" | "stale";
  reason: string;
  observed_at: string | null;
  valid_until: string | null;
};
export type Skill = {
  id: string;
  name: string;
  group: string;
  trained: number;
  active: number | null;
  points: number;
  queue_applied: boolean;
};
export type QueueItem = {
  id: string;
  name: string;
  position: number;
  level: number;
  start: string | null;
  finish: string | null;
  state: string;
};
export type Snapshot = {
  skills_meta: Meta;
  queue_meta: Meta;
  skills: Skill[];
  queue: QueueItem[];
  total_sp: number | null;
  unallocated_sp: number | null;
  calculated_at: string;
};
export type SkillType = {
  id: string;
  name: string;
  english: string;
  group: string;
  group_id: string;
};
export type Requirement = { skill_id: string; level: number };
export type Plan = {
  id: string;
  corporation_id: string;
  name: string;
  version: string;
  requirements: Requirement[];
  updated_at: string;
};
export type Edit = {
  corporation_id: string;
  name: string;
  version: string;
  requirements: Requirement[];
  request_key: string;
};
export type Check = {
  remaining_sp: number | null;
  skill_id: string;
  name: string;
  required: number;
  trained: number | null;
  state: "met" | "missing" | "unknown";
};
export type Result = {
  remaining_sp: number | null;
  character: Character;
  state: "met" | "missing" | "unknown";
  met: number;
  total: number;
  checks: Check[];
  observed_at: string | null;
};
const object = (v: unknown): v is Record<string, unknown> =>
  !!v && typeof v === "object";
const id = (v: unknown): v is string =>
  typeof v === "string" && /^[1-9]\d*$/.test(v);
const level = (v: unknown) =>
  Number.isInteger(v) && Number(v) >= 0 && Number(v) <= 5;
const nullableDate = (v: unknown) =>
  v === null || (typeof v === "string" && Number.isFinite(Date.parse(v)));
const character = (v: unknown): v is Character =>
  object(v) &&
  id(v.id) &&
  typeof v.name === "string" &&
  typeof v.corporation_id === "string";
const meta = (v: unknown): v is Meta =>
  object(v) &&
  ["pending", "blocked", "ready", "stale"].includes(String(v.status)) &&
  typeof v.reason === "string" &&
  nullableDate(v.observed_at) &&
  nullableDate(v.valid_until);
const requirement = (v: unknown): v is Requirement =>
  object(v) && id(v.skill_id) && level(v.level) && Number(v.level) > 0;
export const isPlan = (v: unknown): v is Plan =>
  object(v) &&
  id(v.id) &&
  id(v.corporation_id) &&
  id(v.version) &&
  typeof v.name === "string" &&
  typeof v.updated_at === "string" &&
  Array.isArray(v.requirements) &&
  v.requirements.every(requirement);
export const isSnapshot = (v: unknown): v is Snapshot =>
  object(v) &&
  meta(v.skills_meta) &&
  meta(v.queue_meta) &&
  typeof v.calculated_at === "string" &&
  [v.total_sp, v.unallocated_sp].every(
    (n) => n === null || (Number.isSafeInteger(n) && Number(n) >= 0),
  ) &&
  Array.isArray(v.skills) &&
  v.skills.every(
    (s) =>
      object(s) &&
      id(s.id) &&
      typeof s.name === "string" &&
      typeof s.group === "string" &&
      level(s.trained) &&
      (s.active === null || level(s.active)) &&
      Number.isSafeInteger(s.points) &&
      typeof s.queue_applied === "boolean",
  ) &&
  Array.isArray(v.queue) &&
  v.queue.every(
    (q) =>
      object(q) &&
      id(q.id) &&
      typeof q.name === "string" &&
      Number.isInteger(q.position) &&
      level(q.level) &&
      nullableDate(q.start) &&
      nullableDate(q.finish) &&
      [
        "training",
        "scheduled",
        "paused",
        "awaiting_sync",
        "completed",
      ].includes(String(q.state)),
  );
const root = "/api/v1/skills";
export const context = (member: string, signal?: AbortSignal) =>
  getData(
    `${root}/context?${new URLSearchParams({ member })}`,
    (v): v is Context =>
      object(v) &&
      Array.isArray(v.characters) &&
      v.characters.every(character) &&
      Array.isArray(v.corporations) &&
      v.corporations.every(
        (c) =>
          object(c) &&
          id(c.id) &&
          typeof c.name === "string" &&
          typeof c.can_manage === "boolean",
      ),
    signal,
  );
export const snapshot = (id: string, signal?: AbortSignal) =>
  getData(`${root}/characters/${id}`, isSnapshot, signal);
export const catalog = (signal?: AbortSignal) =>
  getData(
    `${root}/catalog`,
    (v): v is { build: string; items: SkillType[] } =>
      object(v) &&
      typeof v.build === "string" &&
      Array.isArray(v.items) &&
      v.items.every(
        (k) =>
          object(k) &&
          id(k.id) &&
          typeof k.name === "string" &&
          typeof k.english === "string" &&
          typeof k.group === "string" &&
          id(k.group_id),
      ),
    signal,
  );
export const plans = (corp: string, signal?: AbortSignal) =>
  getData(
    `${root}/plans?${new URLSearchParams({ corporation_id: corp })}`,
    (v): v is Plan[] => Array.isArray(v) && v.every(isPlan),
    signal,
  );
export const check = (
  planID: string,
  member: string,
  all: boolean,
  after: string,
  signal?: AbortSignal,
) =>
  getData(
    `${root}/plans/${planID}/check?${new URLSearchParams(all ? { scope: "corporation", after } : { member, after })}`,
    (v): v is { items: Result[]; next_cursor: string } =>
      object(v) &&
      typeof v.next_cursor === "string" &&
      Array.isArray(v.items) &&
      v.items.every(
        (r) =>
          object(r) &&
          character(r.character) &&
          ["met", "missing", "unknown"].includes(String(r.state)) &&
          Number.isInteger(r.met) &&
          Number.isInteger(r.total) &&
          nullableDate(r.observed_at) &&
          (r.remaining_sp === null ||
            (Number.isSafeInteger(r.remaining_sp) &&
              Number(r.remaining_sp) >= 0)) &&
          Array.isArray(r.checks) &&
          r.checks.every(
            (c) =>
              object(c) &&
              id(c.skill_id) &&
              typeof c.name === "string" &&
              level(c.required) &&
              (c.trained === null || level(c.trained)) &&
              (c.remaining_sp === null ||
                (Number.isSafeInteger(c.remaining_sp) &&
                  Number(c.remaining_sp) >= 0)) &&
              ["met", "missing", "unknown"].includes(String(c.state)),
          ),
      ),
    signal,
  );
export async function save(
  id: string,
  body: Edit,
  csrf: string,
  remove = false,
) {
  const r = await apiFetch(`${root}/plans${id ? `/${id}` : ""}`, {
    method: remove ? "DELETE" : id ? "PUT" : "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
    body: JSON.stringify({
      corporation_id: body.corporation_id,
      name: body.name,
      version: body.version,
      requirements: body.requirements,
      request_key: body.request_key,
    }),
  });
  const data = await r.json().catch(() => null);
  if (!r.ok)
    throw new APIError(
      data?.error?.message ?? msg("保存失败，请重试"),
      r.status,
    );
  if (!isPlan(data?.data)) throw new APIError(msg("响应格式异常"), r.status);
  return data.data;
}

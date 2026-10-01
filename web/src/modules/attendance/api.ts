import { apiFetch } from "@/lib/http";
import { msg, getLocale } from "@/lib/i18n";
import { APIError, getData } from "@/lib/http";
export type Entity = { id: string; name: string };
export type Context = { corporations: Entity[]; characters: Entity[] };
export type Event = {
  pap_points?: number;
  pap_issued?: boolean;
  id: string;
  corporation_id: string;
  title: string;
  starts_at: string;
  ends_at?: string | null;
  state: "open" | "closed";
  version: string;
  participants: number;
  can_manage: boolean;
  can_convert?: boolean;
};
export type Entry = {
  pap_points?: number;
  solar_system_name?: string;
  solar_system_id?: string | null;
  location_observed_at?: string | null;
  ship_type_id?: string;
  ship_name?: string;
  losses?: number;
  character_id: string;
  name: string;
  account_id: string | null;
  source: "manual" | "fleet";
  present: boolean;
  recorded_at: string;
};
export type Detail = { event: Event; entries: Entry[] };
export type EventList = { events: Event[]; next_cursor: string };
export const states = {
  online: msg("在线"),
  offline: msg("离线"),
  unknown: msg("未知"),
  reauthorize: msg("需授权"),
  missing_scope: msg("缺少授权"),
} as const;
export type Character = Entity & {
  state: keyof typeof states;
  observed_at: string | null;
};
export type Online = {
  since: string;
  until: string;
  estimated: true;
  seconds: number;
  observed_characters: number;
  days: { date: string; seconds: number; samples: number }[];
  members: {
    user_id: string;
    name: string;
    seconds: number;
    samples: number;
    characters: Character[];
  }[];
};
export type Audit = {
  items: {
    id: string;
    actor_id: string;
    action: string;
    loss_id?: string;
    loss_state?: string;
    reason: string;
    recorded: number;
    excluded: number;
    excluded_external?: number | null;
    excluded_unbound?: number | null;
    created_at: string;
    observed_at: string | null;
    character_id: string;
    present: boolean;
  }[];
  next_cursor: string;
};
const obj = (v: unknown): v is Record<string, unknown> =>
  !!v && typeof v === "object";
const id = (v: unknown): v is string =>
  typeof v === "string" && /^[1-9]\d*$/.test(v);
const date = (v: unknown) =>
  typeof v === "string" && Number.isFinite(Date.parse(v));
const count = (v: unknown) =>
  typeof v === "number" && Number.isFinite(v) && v >= 0;
const entity = (v: unknown): v is Entity =>
  obj(v) && id(v.id) && typeof v.name === "string";
export const isContext = (v: unknown): v is Context =>
  obj(v) &&
  Array.isArray(v.corporations) &&
  v.corporations.every(entity) &&
  Array.isArray(v.characters) &&
  v.characters.every(entity);
export const isEvent = (v: unknown): v is Event =>
  obj(v) &&
  id(v.id) &&
  (v.pap_points == null ||
    (count(v.pap_points) && Number.isInteger(v.pap_points))) &&
  (v.pap_issued == null || typeof v.pap_issued === "boolean") &&
  id(v.corporation_id) &&
  typeof v.title === "string" &&
  date(v.starts_at) &&
  (v.ends_at == null || date(v.ends_at)) &&
  ["open", "closed"].includes(String(v.state)) &&
  id(v.version) &&
  count(v.participants) &&
  (v.can_convert == null || typeof v.can_convert === "boolean") &&
  typeof v.can_manage === "boolean";
export const isEventList = (v: unknown): v is EventList =>
  obj(v) &&
  Array.isArray(v.events) &&
  v.events.every(isEvent) &&
  (v.next_cursor === "" || id(v.next_cursor));
export const isDetail = (v: unknown): v is Detail =>
  obj(v) &&
  isEvent(v.event) &&
  Array.isArray(v.entries) &&
  v.entries.every(
    (e) =>
      obj(e) &&
      id(e.character_id) &&
      typeof e.name === "string" &&
      (e.account_id === null || typeof e.account_id === "string") &&
      ["manual", "fleet"].includes(String(e.source)) &&
      typeof e.present === "boolean" &&
      (e.pap_points == null ||
        (count(e.pap_points) && Number.isInteger(e.pap_points))) &&
      (e.solar_system_name == null ||
        typeof e.solar_system_name === "string") &&
      (e.solar_system_id == null || id(e.solar_system_id)) &&
      (e.location_observed_at == null || date(e.location_observed_at)) &&
      date(e.recorded_at),
  );
export const isOnline = (v: unknown): v is Online =>
  obj(v) &&
  date(v.since) &&
  date(v.until) &&
  v.estimated === true &&
  count(v.seconds) &&
  count(v.observed_characters) &&
  Array.isArray(v.days) &&
  v.days.every(
    (d) => obj(d) && date(d.date) && count(d.seconds) && count(d.samples),
  ) &&
  Array.isArray(v.members) &&
  v.members.every(
    (m) =>
      obj(m) &&
      typeof m.user_id === "string" &&
      typeof m.name === "string" &&
      count(m.seconds) &&
      count(m.samples) &&
      Array.isArray(m.characters) &&
      m.characters.every(
        (c) =>
          obj(c) &&
          id(c.id) &&
          typeof c.name === "string" &&
          Object.hasOwn(states, String(c.state)) &&
          (c.observed_at === null || date(c.observed_at)),
      ),
  );
const isAudit = (v: unknown): v is Audit =>
  obj(v) &&
  (v.next_cursor === "" || id(v.next_cursor)) &&
  Array.isArray(v.items) &&
  v.items.every(
    (a) =>
      obj(a) &&
      id(a.id) &&
      typeof a.actor_id === "string" &&
      [
        "capture",
        "manual",
        "close",
        "reopen",
        "auto_close",
        "loss_review",
        "battle_refresh",
      ].includes(String(a.action)) &&
      typeof a.reason === "string" &&
      count(a.recorded) &&
      count(a.excluded) &&
      (a.excluded_external == null || count(a.excluded_external)) &&
      (a.excluded_unbound == null || count(a.excluded_unbound)) &&
      date(a.created_at) &&
      (a.observed_at === null || date(a.observed_at)) &&
      typeof a.character_id === "string" &&
      typeof a.present === "boolean",
  );
const root = "/api/v1/attendance";
export const getContext = (signal?: AbortSignal) =>
  getData(`${root}/context`, isContext, signal);
export const getEvents = (before: string, signal?: AbortSignal) =>
  getData(
    `${root}/events?${new URLSearchParams({ before })}`,
    isEventList,
    signal,
  );
export const getPendingPAP = (signal?: AbortSignal) =>
  getData(`${root}/pap/pending`, isEventList, signal);
export const getEvent = (id: string, signal?: AbortSignal) =>
  getData(`${root}/events/${encodeURIComponent(id)}`, isDetail, signal);
export const getOnline = (
  corp: string,
  days: string,
  member: string,
  signal?: AbortSignal,
) =>
  getData(
    `${root}/online?${new URLSearchParams({ corporation_id: corp, days, member })}`,
    isOnline,
    signal,
  );
export const getAudit = (id: string, after: string, signal?: AbortSignal) =>
  getData(
    `${root}/events/${encodeURIComponent(id)}/audit?${new URLSearchParams({ after })}`,
    isAudit,
    signal,
  );
export async function write<T>(
  path: string,
  csrf: string,
  body: object,
  validate: (v: unknown) => v is T,
): Promise<T> {
  const r = await apiFetch(`${root}${path}`, {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
    body: JSON.stringify(body),
  });
  const p = await r.json().catch(() => null);
  if (!r.ok)
    throw new APIError(
      p?.error?.message ?? msg("保存失败，请重试"),
      r.status,
      p?.request_id,
    );
  if (!validate(p?.data))
    throw new APIError(msg("响应格式异常，请刷新核对结果"), r.status);
  return p.data;
}
export const isChanged = (
  v: unknown,
): v is {
  event: Event;
  recorded: number;
  excluded: number;
  excluded_external?: number | null;
  excluded_unbound?: number | null;
  auto_closed?: boolean;
} =>
  obj(v) &&
  isEvent(v.event) &&
  count(v.recorded) &&
  count(v.excluded) &&
  (v.excluded_external == null || count(v.excluded_external)) &&
  (v.excluded_unbound == null || count(v.excluded_unbound)) &&
  (v.auto_closed == null || typeof v.auto_closed === "boolean");
export const duration = (seconds: number) =>
  `${(seconds / 3600).toLocaleString(getLocale(), { maximumFractionDigits: 1 })} h`;
export const formatDate = (raw: string) =>
  new Intl.DateTimeFormat(getLocale(), {
    timeZone: "Asia/Shanghai",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  }).format(new Date(raw));

export function captureSummary(r: {
  recorded: number;
  excluded: number;
  excluded_external?: number | null;
  excluded_unbound?: number | null;
  auto_closed?: boolean;
}) {
  if (r.auto_closed) return msg("舰队已解散，活动已自动结束");
  const reasons: string[] = [];
  if (r.excluded_external != null && r.excluded_unbound != null) {
    if (r.excluded_external)
      reasons.push(msg("外团 {0} 名", r.excluded_external));
    if (r.excluded_unbound)
      reasons.push(msg("未绑定 {0} 名", r.excluded_unbound));
  } else if (r.excluded) {
    reasons.push(msg("{0} 名（原因未记录）", r.excluded));
  }
  return msg(
    "已读取 {0} 名军团角色{1}",
    r.recorded,
    reasons.length ? msg("，跳过{0}", reasons.join("、")) : "",
  );
}

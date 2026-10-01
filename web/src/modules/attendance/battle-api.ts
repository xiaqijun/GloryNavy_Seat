import { msg } from "@/lib/i18n";
import { getData } from "@/lib/http";
export type BattleItem = {
  type_id: string;
  name: string;
  slot: string;
  quantity: number;
  destroyed: number;
  dropped: number;
};
export type Fitting = {
  ship_item_id: string;
  ship_type_id: string;
  ship_observed_at: string;
  assets_observed_at: string;
  assets_content_at: string;
  items: BattleItem[];
};
export type Ship = {
  id: string;
  ship_type_id: string;
  ship_name: string;
  solar_system_name?: string;
  solar_system_id: string;
  observed_at: string;
  joined_at: string | null;
  state: string;
  fitting: Fitting | null;
};
export type Loss = {
  id: string;
  ship_type_id: string;
  ship_name: string;
  solar_system_name?: string;
  solar_system_id: string;
  occurred_at: string;
  state: "candidate" | "confirmed" | "rejected";
  version: string;
  items: BattleItem[];
};
export type Battle = {
  ships: Ship[];
  losses: Loss[];
  tasks: {
    kind: "fitting" | "losses";
    state: string;
    reason: string;
    checked_at: string | null;
    next_due_at: string;
  }[];
  truncated: boolean;
};
const obj = (v: unknown): v is Record<string, unknown> =>
  !!v && typeof v === "object";
const id = (v: unknown) => typeof v === "string" && /^[1-9]\d*$/.test(v);
const date = (v: unknown) =>
  typeof v === "string" && Number.isFinite(Date.parse(v));
const count = (v: unknown) =>
  typeof v === "number" && Number.isSafeInteger(v) && v >= 0;
const items = (v: unknown) =>
  Array.isArray(v) &&
  v.every(
    (i) =>
      obj(i) &&
      id(i.type_id) &&
      typeof i.name === "string" &&
      typeof i.slot === "string" &&
      count(i.quantity) &&
      count(i.destroyed) &&
      count(i.dropped),
  );
const fitting = (v: unknown) =>
  v === null ||
  (obj(v) &&
    id(v.ship_item_id) &&
    id(v.ship_type_id) &&
    date(v.ship_observed_at) &&
    date(v.assets_observed_at) &&
    date(v.assets_content_at) &&
    items(v.items));
export const isBattle = (v: unknown): v is Battle =>
  obj(v) &&
  typeof v.truncated === "boolean" &&
  Array.isArray(v.ships) &&
  v.ships.every(
    (s) =>
      obj(s) &&
      id(s.id) &&
      id(s.ship_type_id) &&
      typeof s.ship_name === "string" &&
      (s.solar_system_name == null ||
        typeof s.solar_system_name === "string") &&
      typeof s.solar_system_id === "string" &&
      date(s.observed_at) &&
      (s.joined_at === null || date(s.joined_at)) &&
      typeof s.state === "string" &&
      fitting(s.fitting),
  ) &&
  Array.isArray(v.losses) &&
  v.losses.every(
    (l) =>
      obj(l) &&
      id(l.id) &&
      id(l.ship_type_id) &&
      typeof l.ship_name === "string" &&
      (l.solar_system_name == null ||
        typeof l.solar_system_name === "string") &&
      id(l.solar_system_id) &&
      date(l.occurred_at) &&
      ["candidate", "confirmed", "rejected"].includes(String(l.state)) &&
      id(l.version) &&
      items(l.items),
  ) &&
  Array.isArray(v.tasks) &&
  v.tasks.every(
    (t) =>
      obj(t) &&
      ["fitting", "losses"].includes(String(t.kind)) &&
      typeof t.state === "string" &&
      typeof t.reason === "string" &&
      (t.checked_at === null || date(t.checked_at)) &&
      date(t.next_due_at),
  );
export const getBattle = (
  event: string,
  character: string,
  signal?: AbortSignal,
) =>
  getData(
    `/api/v1/attendance/events/${encodeURIComponent(event)}/characters/${encodeURIComponent(character)}/battle`,
    isBattle,
    signal,
  );
export const isSaved = (v: unknown): v is { saved: true } =>
  obj(v) && v.saved === true;
export const evidenceStates: Record<string, string> = {
  pending: msg("等待采集"),
  running: msg("采集中"),
  ready: msg("已采集"),
  failed: msg("采集失败"),
  blocked: msg("采集暂停"),
  missing_scope: msg("需要更新授权"),
  reauthorize: msg("需要重新授权"),
  binding_changed: msg("角色绑定已变化"),
  capture_expired: msg("未取得当时装配"),
  ship_changed: msg("采集时已换船"),
  assets_unavailable: msg("资产中未找到装配"),
  assets_changed: msg("资产分页已变化"),
  assets_incomplete: msg("资产数据不完整"),
  esi_unavailable: msg("ESI 暂不可用"),
  rate_limited: msg("等待 ESI 配额"),
  forbidden: msg("ESI 拒绝访问"),
};

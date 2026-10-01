import { getData } from "@/lib/http";
export type PAPRow = {
  event_id: string;
  character_id: string;
  account_id: string;
  name: string;
  title: string;
  starts_at: string;
  points: number;
};
export type PAPReport = {
  points: number;
  events: number;
  participations: number;
  rows: PAPRow[];
  more: boolean;
};
export type PAPHistory = {
  items: {
    id: string;
    character_id: string;
    name: string;
    delta: number;
    balance: number;
    reason: string;
    created_at: string;
  }[];
  next_cursor: string;
};
const obj = (v: unknown): v is Record<string, unknown> =>
  !!v && typeof v === "object";
const id = (v: unknown) => typeof v === "string" && /^[1-9]\d*$/.test(v);
const n = (v: unknown): v is number =>
  typeof v === "number" && Number.isSafeInteger(v) && v >= 0;
const date = (v: unknown) =>
  typeof v === "string" && Number.isFinite(Date.parse(v));
export const isPAPReport = (v: unknown): v is PAPReport =>
  obj(v) &&
  n(v.points) &&
  n(v.events) &&
  n(v.participations) &&
  typeof v.more === "boolean" &&
  Array.isArray(v.rows) &&
  v.rows.every(
    (r) =>
      obj(r) &&
      id(r.event_id) &&
      id(r.character_id) &&
      typeof r.account_id === "string" &&
      typeof r.name === "string" &&
      typeof r.title === "string" &&
      date(r.starts_at) &&
      n(r.points),
  );
export const isPAPHistory = (v: unknown): v is PAPHistory =>
  obj(v) &&
  (v.next_cursor === "" || id(v.next_cursor)) &&
  Array.isArray(v.items) &&
  v.items.every(
    (r) =>
      obj(r) &&
      id(r.id) &&
      id(r.character_id) &&
      typeof r.name === "string" &&
      typeof r.delta === "number" &&
      Number.isSafeInteger(r.delta) &&
      r.delta !== 0 &&
      n(r.balance) &&
      typeof r.reason === "string" &&
      date(r.created_at),
  );
export const getPAP = (
  corp: string,
  period: string,
  page: number,
  signal?: AbortSignal,
) =>
  getData(
    `/api/v1/attendance/pap?${new URLSearchParams({ corporation_id: corp, period, page: String(page) })}`,
    isPAPReport,
    signal,
  );
export const getPAPHistory = (
  id: string,
  after: string,
  signal?: AbortSignal,
) =>
  getData(
    `/api/v1/attendance/events/${encodeURIComponent(id)}/pap?${new URLSearchParams({ after })}`,
    isPAPHistory,
    signal,
  );
export const isSaved = (v: unknown): v is { saved: true } =>
  obj(v) && v.saved === true;

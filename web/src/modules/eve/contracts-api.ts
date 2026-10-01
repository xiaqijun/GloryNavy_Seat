import { gameLabels } from "@/lib/eve-terminology";
import { msg, getLocale } from "@/lib/i18n";
import { getData } from "@/lib/http";

export type Owner = {
  kind: "character" | "corporation";
  id: string;
  name: string;
};
export type Entity = {
  id: string;
  name: string;
  category: string;
  name_language?: "zh" | "en" | "";
};
export type Contract = {
  id: string;
  title: string;
  summary?: string;
  trade_direction?: "sell" | "buy" | "exchange" | "transport" | "unknown";
  type: string;
  status: string;
  availability: string;
  for_corporation: boolean;
  issuer: Entity;
  assignee: Entity;
  acceptor: Entity;
  start: Entity;
  end: Entity;
  price: string | null;
  reward: string | null;
  collateral: string | null;
  buyout: string | null;
  volume: string | null;
  days_to_complete: string | null;
  date_issued: string;
  date_expired: string;
  date_accepted: string;
  date_completed: string;
  checked_at: string;
};
export type DetailState = {
  part: "items" | "bids";
  state: string;
  reason: string;
  updated_at: string | null;
};
export type Item = {
  id: string;
  type: Entity;
  quantity: string;
  included: boolean;
  singleton: boolean;
  raw_quantity: number | null;
};
export type Bid = { id: string; bidder: Entity; amount: string; date: string };
export type Page<T> = { items: T[]; next_cursor: string };
const record = (v: unknown): v is Record<string, unknown> =>
  v !== null && typeof v === "object";
const id = (v: unknown): v is string =>
  typeof v === "string" && /^(0|[1-9]\d*)$/.test(v);
const decimal = (v: unknown): v is string =>
  typeof v === "string" && /^-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?$/.test(v);
const entity = (v: unknown): v is Entity =>
  record(v) &&
  id(v.id) &&
  typeof v.name === "string" &&
  typeof v.category === "string";
export const isContract = (v: unknown): v is Contract =>
  record(v) &&
  id(v.id) &&
  (v.summary === undefined || typeof v.summary === "string") &&
  (v.trade_direction === undefined ||
    (typeof v.trade_direction === "string" &&
      Object.hasOwn(tradeDirections, v.trade_direction))) &&
  [
    "title",
    "type",
    "status",
    "availability",
    "date_issued",
    "date_expired",
    "date_accepted",
    "date_completed",
    "checked_at",
  ].every((k) => typeof v[k] === "string") &&
  typeof v.for_corporation === "boolean" &&
  [v.issuer, v.assignee, v.acceptor, v.start, v.end].every(entity) &&
  [
    v.price,
    v.reward,
    v.collateral,
    v.buyout,
    v.volume,
    v.days_to_complete,
  ].every((n) => n === null || decimal(n));
const page =
  <T>(check: (v: unknown) => v is T) =>
  (v: unknown): v is Page<T> =>
    record(v) &&
    Array.isArray(v.items) &&
    v.items.every(check) &&
    (v.next_cursor === "" || id(v.next_cursor));
export function getContractOwners(signal?: AbortSignal, member = "") {
  return getData(
    `/api/v1/eve/contracts/owners${member ? `?${new URLSearchParams({ member })}` : ""}`,
    (v): v is { owners: Owner[] } =>
      record(v) &&
      Array.isArray(v.owners) &&
      v.owners.every(
        (o) =>
          record(o) &&
          ["character", "corporation"].includes(String(o.kind)) &&
          id(o.id) &&
          typeof o.name === "string",
      ),
    signal,
  );
}
const root = (owner: Owner) =>
  `/api/v1/eve/contracts/${owner.kind}/${encodeURIComponent(owner.id)}`;
export const getContracts = (
  owner: Owner,
  params: URLSearchParams,
  signal?: AbortSignal,
) => getData(`${root(owner)}?${params}`, page(isContract), signal);
export function getContract(
  owner: Owner,
  contract: string,
  signal?: AbortSignal,
) {
  return getData(
    `${root(owner)}/${encodeURIComponent(contract)}`,
    (v): v is { contract: Contract; details: DetailState[] } =>
      record(v) &&
      isContract(v.contract) &&
      Array.isArray(v.details) &&
      v.details.every(
        (d) =>
          record(d) &&
          ["items", "bids"].includes(String(d.part)) &&
          typeof d.state === "string" &&
          typeof d.reason === "string" &&
          (d.updated_at === null || typeof d.updated_at === "string"),
      ),
    signal,
  );
}
export const getItems = (
  owner: Owner,
  contract: string,
  after: string,
  signal?: AbortSignal,
) =>
  getData(
    `${root(owner)}/${encodeURIComponent(contract)}/items?after=${encodeURIComponent(after)}`,
    page(
      (v): v is Item =>
        record(v) &&
        id(v.id) &&
        entity(v.type) &&
        decimal(v.quantity) &&
        typeof v.included === "boolean" &&
        typeof v.singleton === "boolean" &&
        (v.raw_quantity === null ||
          (typeof v.raw_quantity === "number" &&
            Number.isSafeInteger(v.raw_quantity))),
    ),
    signal,
  );
export const getBids = (
  owner: Owner,
  contract: string,
  before: string,
  signal?: AbortSignal,
) =>
  getData(
    `${root(owner)}/${encodeURIComponent(contract)}/bids?before=${encodeURIComponent(before)}`,
    page(
      (v): v is Bid =>
        record(v) &&
        id(v.id) &&
        entity(v.bidder) &&
        decimal(v.amount) &&
        typeof v.date === "string",
    ),
    signal,
  );
export const contractTypes: Record<string, string> =
  gameLabels("contractTypes");
export const tradeDirections = {
  sell: msg("出售"),
  buy: msg("求购"),
  exchange: msg("交换"),
  transport: msg("运输"),
  unknown: msg("待确认"),
};
export const contractStatuses: Record<string, string> =
  gameLabels("contractStatuses");
export const title = (c: Contract) =>
  c.title.trim() || c.summary?.trim() || contractTypes[c.type] || c.type;
export const entityName = (e: Entity, empty = msg("未指定")) =>
  e.name || (e.id === "0" ? empty : `#${e.id}`);
// Format decimal strings without passing ISK through a floating point number.
export function amount(value: string | null) {
  if (value === null) return "—";
  const match = /^(-?)(\d+)(?:\.(\d+))?(?:[eE]([+-]?\d+))?$/.exec(value);
  if (!match) return "—";
  const exponent = Number(match[4] ?? 0);
  if (Math.abs(exponent) > 100) return value;
  const digits = match[2] + (match[3] ?? "");
  const point = match[2].length + exponent;
  const integer = (
    point <= 0 ? "0" : digits.slice(0, point).padEnd(point, "0")
  ).replace(/^0+(?=\d)/, "");
  const fraction = (
    point <= 0 ? "0".repeat(-point) + digits : digits.slice(point)
  ).replace(/0+$/, "");
  return (
    match[1] +
    integer.replace(/\B(?=(\d{3})+(?!\d))/g, ",") +
    (fraction ? "." + fraction : "")
  );
}
export const contractDate = (value: string) =>
  value && Number.isFinite(Date.parse(value))
    ? new Intl.DateTimeFormat(getLocale(), {
        month: "2-digit",
        day: "2-digit",
        year: "numeric",
        hour: "2-digit",
        minute: "2-digit",
        hour12: false,
      }).format(new Date(value))
    : "—";

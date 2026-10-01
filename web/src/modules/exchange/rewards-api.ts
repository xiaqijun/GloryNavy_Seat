import { getData } from "@/lib/http";
import { isPhysical, type PhysicalContent } from "./catalog-api";
export type Reward = {
  pricing?: {
    automatic: boolean;
    status: string;
    checked_at: string | null;
    next_at: string;
  };
  content?: PhysicalContent;
  id: string;
  type_id: string;
  name: string;
  quantity: number;
  isk_value: number;
  coins_minor: number;
  stock: number;
  enabled: boolean;
  version: string;
};
export type Shop = {
  admin: boolean;
  isk_per_coin: number;
  version: string;
  earned_minor: number;
  reserved_minor: number;
  spent_minor: number;
  available_minor: number;
  rewards: Reward[];
  next_cursor: string;
};
export type Order = {
  reference?: string;
  delivery?: {
    status: string;
    checked_at: string | null;
    contracts: { id: string; status: string }[];
  };
  content?: PhysicalContent;
  id: string;
  version: string;
  type_id: string;
  name: string;
  quantity: number;
  recipient_id: string;
  recipient_name: string;
  coins_minor: number;
  isk_per_coin: number;
  isk_value: number;
  state: "pending" | "cancel_requested" | "fulfilled" | "cancelled";
  can_request_cancel?: boolean;
  note: string;
  created_at: string;
};
export type Orders = { items: Order[]; next_cursor: string };
const obj = (v: unknown): v is Record<string, unknown> =>
  !!v && typeof v === "object";
const id = (v: unknown): v is string =>
  typeof v === "string" && /^[1-9]\d*$/.test(v);
const integer = (v: unknown): v is number =>
  typeof v === "number" && Number.isSafeInteger(v);
const count = (v: unknown) => integer(v) && v >= 0;
const cursor = (v: unknown) => v === "" || id(v);
export const isReward = (v: unknown): v is Reward =>
  obj(v) &&
  (v.content === undefined || isPhysical(v.content)) &&
  id(v.id) &&
  (id(v.type_id) ||
    (v.type_id === "0" &&
      isPhysical(v.content) &&
      (v.content.isk_minor || 0) > 0)) &&
  id(v.version) &&
  typeof v.name === "string" &&
  count(v.quantity) &&
  count(v.isk_value) &&
  count(v.coins_minor) &&
  count(v.stock) &&
  typeof v.enabled === "boolean";
export const isShop = (v: unknown): v is Shop =>
  obj(v) &&
  typeof v.admin === "boolean" &&
  id(v.version) &&
  count(v.isk_per_coin) &&
  count(v.earned_minor) &&
  count(v.reserved_minor) &&
  count(v.spent_minor) &&
  integer(v.available_minor) &&
  v.available_minor ===
    Number(v.earned_minor) - Number(v.reserved_minor) - Number(v.spent_minor) &&
  cursor(v.next_cursor) &&
  Array.isArray(v.rewards) &&
  v.rewards.every(isReward);
export const isOrders = (v: unknown): v is Orders =>
  obj(v) &&
  cursor(v.next_cursor) &&
  Array.isArray(v.items) &&
  v.items.every(
    (r) =>
      obj(r) &&
      (r.content === undefined || isPhysical(r.content)) &&
      (r.reference === undefined || typeof r.reference === "string") &&
      (r.delivery === undefined ||
        (obj(r.delivery) &&
          typeof r.delivery.status === "string" &&
          (r.delivery.checked_at === null ||
            typeof r.delivery.checked_at === "string") &&
          Array.isArray(r.delivery.contracts) &&
          r.delivery.contracts.every(
            (c: unknown) => obj(c) && id(c.id) && typeof c.status === "string",
          ))) &&
      id(r.id) &&
      id(r.version) &&
      (id(r.type_id) ||
        (r.type_id === "0" &&
          isPhysical(r.content) &&
          (r.content.isk_minor || 0) > 0)) &&
      id(r.recipient_id) &&
      typeof r.name === "string" &&
      typeof r.recipient_name === "string" &&
      typeof r.note === "string" &&
      count(r.quantity) &&
      count(r.coins_minor) &&
      count(r.isk_per_coin) &&
      count(r.isk_value) &&
      ["pending", "cancel_requested", "fulfilled", "cancelled"].includes(
        String(r.state),
      ) &&
      (r.can_request_cancel === undefined ||
        typeof r.can_request_cancel === "boolean") &&
      typeof r.created_at === "string" &&
      Number.isFinite(Date.parse(r.created_at)),
  );
export const isClaimed = (v: unknown): v is { id: string } =>
  obj(v) && id(v.id);
const root = "/api/v1/exchange/rewards";
export const getShop = (
  after: string,
  signal?: AbortSignal,
  includeUnlisted = false,
) =>
  getData(
    `${root}?${new URLSearchParams({ after, scope: includeUnlisted ? "all" : "listed" })}`,
    isShop,
    signal,
  );
export async function getRewardSettings(signal?: AbortSignal) {
  const shop = await getShop("", signal, true);
  let after = shop.next_cursor;
  while (after) {
    const page = await getShop(after, signal, true);
    shop.rewards.push(...page.rewards);
    after = page.next_cursor;
  }
  return shop;
}
export const getOrders = (all: boolean, before: string, signal?: AbortSignal) =>
  getData(
    `${root}/orders?${new URLSearchParams({ scope: all ? "all" : "mine", before })}`,
    isOrders,
    signal,
  );
export const getTypes = (q: string, signal?: AbortSignal) =>
  getData(
    `${root}/types?${new URLSearchParams({ q })}`,
    (v: unknown): v is { id: string; name: string }[] =>
      Array.isArray(v) &&
      v.every((r) => obj(r) && id(r.id) && typeof r.name === "string"),
    signal,
  );

export type RewardValuation = {
  version: string;
  isk_value: number;
  mid: string;
  complete: boolean;
  missing_types: number;
  observed_at: string | null;
};
export const getRewardValuation = (reward: Reward, signal?: AbortSignal) =>
  getData(
    root + "/" + reward.id + "/valuation?version=" + reward.version,
    (v: unknown): v is RewardValuation =>
      obj(v) &&
      v.version === reward.version &&
      Number.isSafeInteger(v.isk_value) &&
      Number(v.isk_value) >= 0 &&
      Number(v.isk_value) <= 1e12 &&
      typeof v.mid === "string" &&
      /^\d+(\.\d+)?$/.test(v.mid) &&
      typeof v.complete === "boolean" &&
      (!v.complete || Number(v.isk_value) > 0) &&
      Number.isSafeInteger(v.missing_types) &&
      Number(v.missing_types) >= 0 &&
      (v.observed_at === null ||
        (typeof v.observed_at === "string" &&
          Number.isFinite(Date.parse(v.observed_at)))),
    signal,
  );

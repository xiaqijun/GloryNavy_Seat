import { useQuery } from "@tanstack/react-query";
import { getModuleCatalog } from "@/app/catalog";
import { getData } from "@/lib/http";
import type { Case } from "@/modules/welfare/api";
import type { Order } from "@/modules/exchange/rewards-api";

export type ApprovalContext = {
  allowed: boolean;
  sources: string[];
  corporations: { id: string; name: string }[];
  unavailable: string[];
  people: { id: string; name: string }[];
};
export type QueueItem = {
  source: "welfare" | "exchange";
  id: string;
  version: string;
  account_id: string;
  applicant: string;
  corporation_id: string;
  kind: string;
  state: string;
  status: string;
  recipient: string;
  title: string;
  reference: string;
  amount_minor: number;
  unit: string;
  time: string;
  action: string;
  actions: string[];
  payload: Case | Order;
};
export type Queue = {
  items: QueueItem[];
  counts: Record<string, number>;
  next_cursor: string;
  unavailable: string[];
};
export function lossQueueAmount(
  item: QueueItem,
): { amount: number; label: "quote" | "approved" | "pending" } | null {
  if (item.source !== "welfare" || !["srp", "solo"].includes(item.kind))
    return null;
  if (!["submitted", "information", "external"].includes(item.state)) {
    return { amount: item.amount_minor, label: "approved" };
  }
  const detail = (item.payload as Case).detail;
  const quote =
    item.kind === "solo"
      ? detail?.valuation?.state === "ready"
        ? detail.valuation.amount_minor
        : 0
      : detail?.base_minor ?? 0;
  return quote > 0
    ? { amount: quote, label: "quote" }
    : { amount: 0, label: "pending" };
}
const object = (v: unknown): v is Record<string, unknown> =>
  !!v && typeof v === "object";
const strings = (v: unknown): v is string[] =>
  Array.isArray(v) && v.every((s) => typeof s === "string");
const isContext = (v: unknown): v is ApprovalContext =>
  object(v) &&
  typeof v.allowed === "boolean" &&
  strings(v.sources) &&
  strings(v.unavailable) &&
  Array.isArray(v.people) &&
  Array.isArray(v.corporations) &&
  v.corporations.every(
    (c) => object(c) && typeof c.id === "string" && typeof c.name === "string",
  );
export const isItem = (v: unknown): v is QueueItem =>
  object(v) &&
  ["welfare", "exchange"].includes(String(v.source)) &&
  [
    "id",
    "version",
    "account_id",
    "corporation_id",
    "kind",
    "state",
    "status",
    "recipient",
    "title",
    "reference",
    "unit",
    "time",
    "action",
  ].every((k) => typeof v[k] === "string") &&
  typeof v.amount_minor === "number" &&
  strings(v.actions) &&
  object(v.payload);
const isQueue = (v: unknown): v is Queue =>
  object(v) &&
  Array.isArray(v.items) &&
  v.items.every(isItem) &&
  object(v.counts) &&
  Object.values(v.counts).every((n) => typeof n === "number") &&
  typeof v.next_cursor === "string" &&
  strings(v.unavailable);
export const getContext = (signal?: AbortSignal) =>
  getData("/api/v1/approval/context", isContext, signal);
export const getQueue = (params: URLSearchParams, signal?: AbortSignal) =>
  getData(`/api/v1/approval/items?${params}`, isQueue, signal);
export const getItem = (source: string, id: string, signal?: AbortSignal) =>
  getData(
    `/api/v1/approval/items/${encodeURIComponent(source)}/${encodeURIComponent(id)}`,
    isItem,
    signal,
  );
export const approvalLink = (source: string, id: string) =>
  `/approvals?source=${encodeURIComponent(source)}&id=${encodeURIComponent(id)}`;
export function useApprovalEnabled() {
  const q = useQuery({
    queryKey: ["host", "modules"],
    queryFn: ({ signal }) => getModuleCatalog(signal),
  });
  return !!q.data?.some((m) => m.id === "approval");
}

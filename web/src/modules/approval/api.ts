import { useQuery } from "@tanstack/react-query";
import { getModuleCatalog } from "@/app/catalog";
import { getData } from "@/lib/http";
import type { Case } from "@/modules/welfare/api";
import type { Order } from "@/modules/exchange/rewards-api";

export type LoanApproval = {
  id: string;
  public_id: string;
  version: string;
  pool_id: string;
  pool_name: string;
  corporation_id: string;
  borrower_account_id: string;
  borrower_character_id: string;
  principal_minor: number;
  interest_minor: number;
  total_due_minor: number;
  installment_count: number;
  interval_days: number;
  first_due_at: string;
  state: string;
  review_note: string;
  created_at: string;
};

export type ApprovalContext = {
  allowed: boolean;
  sources: string[];
  capabilities?: Record<string, {
    approve: boolean;
    reject: boolean;
    cancel_review: boolean;
    fulfill: boolean;
    batch_settle: boolean;
    corporation_filter: boolean;
    applicant_filter: boolean;
    amount: boolean;
    detail_kind: string;
  }>;
  corporations: { id: string; name: string }[];
  unavailable: string[];
  people: { id: string; name: string }[];
};
export type QueueItem = {
  source: string;
  id: string;
  version: string;
  summary_version?: string;
  detail_kind?: string;
  source_status?: "available" | "stale" | "unavailable" | string;
  stale?: boolean;
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
  payload: Case | Order | LoanApproval | Record<string, unknown>;
};
export type Queue = {
  items: QueueItem[];
  counts: Record<string, number>;
  next_cursor: string;
  unavailable: string[];
  source_status?: Record<string, string>;
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
const isCapabilities = (v: unknown): v is ApprovalContext["capabilities"] =>
  !v || (object(v) && Object.values(v).every((cap) =>
    object(cap) &&
    ["approve", "reject", "cancel_review", "fulfill", "batch_settle", "corporation_filter", "applicant_filter", "amount"].every((key) => typeof cap[key] === "boolean") &&
    typeof cap.detail_kind === "string",
  ));
const isContext = (v: unknown): v is ApprovalContext =>
  object(v) &&
  typeof v.allowed === "boolean" &&
  strings(v.sources) &&
  isCapabilities(v.capabilities) &&
  strings(v.unavailable) &&
  Array.isArray(v.people) &&
  Array.isArray(v.corporations) &&
  v.corporations.every(
    (c) => object(c) && typeof c.id === "string" && typeof c.name === "string",
  );
export const isItem = (v: unknown): v is QueueItem =>
  object(v) &&
  ["welfare", "exchange", "loan"].includes(String(v.source)) &&
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
export function getContext(signal?: AbortSignal): Promise<ApprovalContext>;
export function getContext(includePeople: boolean, signal?: AbortSignal): Promise<ApprovalContext>;
export function getContext(includePeopleOrSignal: boolean | AbortSignal = false, signal?: AbortSignal) {
  const includePeople = typeof includePeopleOrSignal === "boolean" ? includePeopleOrSignal : false;
  const requestSignal = typeof includePeopleOrSignal === "boolean" ? signal : includePeopleOrSignal;
  return getData(`/api/v1/approval/context${includePeople ? "?include_people=true" : ""}`, isContext, requestSignal);
}
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

import { apiFetch, APIError, getData } from "@/lib/http";
import { msg } from "@/lib/i18n";

export type SettlementItem = {
  id: string;
  batch_id: string;
  source: "welfare" | "exchange";
  source_id: string;
  state: "pending" | "processing" | "completed" | "failed";
  attempts: number;
  last_error?: string;
  created_at: string;
  updated_at: string;
};
export type SettlementBatch = {
  id: string;
  request_key: string;
  actor_id: string;
  note: string;
  state: "pending" | "processing" | "completed" | "partial" | "failed";
  total_count: number;
  completed_count: number;
  failed_count: number;
  created_at: string;
  started_at?: string;
  finished_at?: string;
  next_run_at: string;
  version: string;
  settlement_reference?: string;
  account_id?: string;
  isk_minor: number;
  items: { type_id: string; quantity: string }[];
  recipient_ids: string[];
  recipient_names?: string[];
  delivery_status?: "awaiting_acceptance";
  contract_id?: string;
  contract_recipient_id?: string;
  entries?: SettlementItem[];
};
export type SettlementView = { batch: SettlementBatch; items: SettlementItem[] };
const obj = (v: unknown): v is Record<string, any> => !!v && typeof v === "object";
const str = (v: unknown): v is string => typeof v === "string";
const isItem = (v: unknown): v is SettlementItem =>
  obj(v) && str(v.id) && str(v.batch_id) && ["welfare", "exchange"].includes(v.source) && str(v.source_id) && ["pending", "processing", "completed", "failed"].includes(v.state) && Number.isInteger(v.attempts) && str(v.created_at) && str(v.updated_at);
const isBatch = (v: unknown): v is SettlementBatch =>
  obj(v) && str(v.id) && str(v.request_key) && str(v.actor_id) && str(v.note) && ["pending", "processing", "completed", "partial", "failed"].includes(v.state) && Number.isInteger(v.total_count) && Number.isInteger(v.completed_count) && Number.isInteger(v.failed_count) && str(v.created_at) && str(v.next_run_at) && str(v.version) && (v.settlement_reference === undefined || str(v.settlement_reference)) && Number.isInteger(v.isk_minor) && Array.isArray(v.items) && Array.isArray(v.recipient_ids) && (v.recipient_names === undefined || Array.isArray(v.recipient_names) && v.recipient_names.every(str)) && (v.delivery_status === undefined || v.delivery_status === "awaiting_acceptance") && (v.entries === undefined || Array.isArray(v.entries) && v.entries.every(isItem));
const isView = (v: unknown): v is SettlementView => obj(v) && isBatch(v.batch) && Array.isArray(v.items) && v.items.every(isItem);
export const list = (signal?: AbortSignal) => getData("/api/v1/welfare/settlements", (v): v is { items: SettlementBatch[] } => obj(v) && Array.isArray(v.items) && v.items.every(isBatch), signal);
export const detail = (id: string, signal?: AbortSignal) => getData(`/api/v1/welfare/settlements/${encodeURIComponent(id)}`, isView, signal);
export async function create(csrf: string, items: { source: "welfare" | "exchange"; id: string }[], note = ""): Promise<SettlementView> {
  const res = await apiFetch("/api/v1/welfare/settlements", { method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf }, body: JSON.stringify({ request_key: crypto.randomUUID(), note, items }) });
  const payload = await res.json().catch(() => null);
  if (!res.ok) throw new APIError(payload?.error?.message ?? msg("批量结算失败，请重试"), res.status, payload?.request_id);
  if (!isView(payload?.data)) throw new APIError(msg("返回格式异常，请刷新核对"), res.status);
  return payload.data;
}
export async function retry(id: string, csrf: string): Promise<void> {
  const res = await apiFetch(`/api/v1/welfare/settlements/${encodeURIComponent(id)}/retry`, { method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf }, body: "{}" });
  const payload = await res.json().catch(() => null);
  if (!res.ok) throw new APIError(payload?.error?.message ?? msg("重试失败，请重试"), res.status, payload?.request_id);
}

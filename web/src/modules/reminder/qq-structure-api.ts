import { apiFetch, APIError, getData } from "@/lib/http";
import { msg } from "@/lib/i18n";

export interface QQStructureSettings {
  enabled: boolean;
  group_openid: string;
  corporation_id: string;
  corporation_name: string;
  revision: string;
  bot_ready: boolean;
  bot_changed: boolean;
}
export interface QQStructureDelivery {
  id: string;
  body: string;
  kind: string;
  status: string;
  error_code: number;
  created_at: string;
}
const isSettings = (value: unknown): value is QQStructureSettings => {
  if (!value || typeof value !== "object") return false;
  const v = value as QQStructureSettings;
  return typeof v.enabled === "boolean" && typeof v.group_openid === "string" && typeof v.corporation_name === "string" && /^\d+$/.test(v.corporation_id) && /^\d+$/.test(v.revision) && typeof v.bot_ready === "boolean" && typeof v.bot_changed === "boolean";
};
const isDeliveries = (value: unknown): value is QQStructureDelivery[] => Array.isArray(value) && value.every((v) => v && typeof v.id === "string" && typeof v.body === "string" && typeof v.kind === "string" && typeof v.status === "string" && Number.isInteger(v.error_code) && typeof v.created_at === "string");
export const settings = (signal?: AbortSignal) => getData("/api/v1/community/qq/structure/settings", isSettings, signal);
export const deliveries = (signal?: AbortSignal) => getData("/api/v1/community/qq/structure/deliveries", isDeliveries, signal);
export async function save(value: Pick<QQStructureSettings, "enabled" | "group_openid" | "corporation_id" | "revision">, csrf: string) {
  const response = await apiFetch("/api/v1/community/qq/structure/settings", { method: "PUT", credentials: "same-origin", headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf }, body: JSON.stringify(value) });
  const body = await response.json().catch(() => null);
  if (!response.ok) throw new APIError(body?.error?.message ?? msg("操作失败，请稍后重试"), response.status);
  if (!isSettings(body?.data)) throw new Error(msg("响应格式无效"));
  return body.data;
}

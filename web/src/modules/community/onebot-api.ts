import { apiFetch, APIError, getData } from "@/lib/http";
import { msg } from "@/lib/i18n";

export interface OneBotGroup { group_id: string; name: string }
export interface OneBotSettings {
  enabled: boolean;
  api_base: string;
  bot_qq: string;
  token_configured: boolean;
  secret_configured: boolean;
  revision: string;
  groups: OneBotGroup[];
}
export interface OneBotStatus { online: boolean; bot_qq: string; nickname: string; groups: OneBotGroup[] }
const isGroups = (v: unknown): v is OneBotGroup[] => Array.isArray(v) && v.every((item) => item && typeof item.group_id === "string" && /^[1-9]\d{4,11}$/.test(item.group_id) && typeof item.name === "string");
export const isOneBotSettings = (v: unknown): v is OneBotSettings => {
  if (!v || typeof v !== "object") return false;
  const s = v as Partial<OneBotSettings>;
  return typeof s.enabled === "boolean" && typeof s.api_base === "string" && typeof s.bot_qq === "string" && typeof s.token_configured === "boolean" && typeof s.secret_configured === "boolean" && typeof s.revision === "string" && /^\d+$/.test(s.revision) && isGroups(s.groups);
};
const isStatus = (v: unknown): v is OneBotStatus => {
  if (!v || typeof v !== "object") return false;
  const s = v as Partial<OneBotStatus>;
  return typeof s.online === "boolean" && typeof s.bot_qq === "string" && typeof s.nickname === "string" && isGroups(s.groups);
};
export const getOneBotSettings = (signal?: AbortSignal) => getData("/api/v1/community/qq/onebot/settings", isOneBotSettings, signal);
export const getOneBotStatus = () => getData("/api/v1/community/qq/onebot/status", isStatus);
export async function saveOneBotSettings(input: Pick<OneBotSettings, "enabled" | "api_base" | "bot_qq" | "revision" | "groups"> & { token: string; secret: string }, csrf: string) {
  const response = await apiFetch("/api/v1/community/qq/onebot/settings", { method: "PUT", credentials: "same-origin", headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf }, body: JSON.stringify(input) });
  const payload = await response.json().catch(() => null);
  if (!response.ok || !isOneBotSettings(payload?.data)) throw new APIError(payload?.error?.message ?? msg("保存失败，请稍后重试"), response.status);
  return payload.data as OneBotSettings;
}
export async function testOneBotMention(groupID: string, csrf: string) {
  const response = await apiFetch("/api/v1/community/qq/onebot/test", { method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf }, body: JSON.stringify({ group_id: groupID }) });
  const payload = await response.json().catch(() => null);
  if (!response.ok || payload?.data?.sent !== true) throw new APIError(payload?.error?.message ?? msg("测试消息发送失败"), response.status);
}

import { apiFetch, APIError, getData } from "@/lib/http";
import { msg } from "@/lib/i18n";
export interface Character { id: string; name: string; ready: boolean }
export interface ReminderEvent { id: string; name: string }
export interface Settings { events: string[]; available_events: ReminderEvent[]; enabled: boolean; sender_id: string; sender_name: string; revision: string; status: string; characters: Character[] }
export interface Delivery { id: string; channel: string; event: string; sender_id: string; recipient_id: string; recipient_name: string; notice_count: number; state: string; reason: string; message_id: string; created_at: string }
export function isSettings(v: unknown): v is Settings {
  if (!v || typeof v !== "object") return false;
  const s = v as Settings;
  return Array.isArray(s.events) && s.events.every(e => typeof e === "string") && Array.isArray(s.available_events) && s.available_events.every(e => typeof e.id === "string" && typeof e.name === "string") && typeof s.enabled === "boolean" && typeof s.sender_id === "string" && typeof s.sender_name === "string" && typeof s.revision === "string" && typeof s.status === "string" && Array.isArray(s.characters) && s.characters.every(c => typeof c.id === "string" && typeof c.name === "string" && typeof c.ready === "boolean");
}
export function isDeliveries(v: unknown): v is Delivery[] {
  return Array.isArray(v) && v.every(d => d && typeof d.id === "string" && typeof d.event === "string" && d.channel === "eve_mail" && typeof d.state === "string" && typeof d.reason === "string" && typeof d.recipient_name === "string" && typeof d.recipient_id === "string" && typeof d.sender_id === "string" && typeof d.message_id === "string" && Number.isInteger(d.notice_count) && typeof d.created_at === "string");
}
export const settings = (signal?: AbortSignal) => getData("/api/v1/reminder/settings", isSettings, signal);
export const deliveries = (before = "", signal?: AbortSignal) => getData(`/api/v1/reminder/deliveries?before=${encodeURIComponent(before)}`, isDeliveries, signal);
async function write(path: string, csrf: string, method: string, body?: unknown) {
  const r = await apiFetch(path, { method, credentials: "same-origin", headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf }, body: body === undefined ? undefined : JSON.stringify(body) });
  const v = await r.json().catch(() => null);
  if (!r.ok) throw new APIError(v?.error?.message ?? msg("操作失败，请稍后重试"), r.status);
  return v?.data as unknown;
}
export async function save(value: Pick<Settings, "enabled" | "sender_id" | "revision" | "events">, csrf: string) {
  const v = await write("/api/v1/reminder/settings", csrf, "PUT", value);
  if (!isSettings(v)) throw new Error(msg("响应格式无效"));
  return v;
}
export async function authorize(id: string, csrf: string) {
  const v = await write(`/api/v1/eve/characters/${encodeURIComponent(id)}/reauthorize?purpose=reminder-mail`, csrf, "POST") as { url?: string };
  if (typeof v?.url !== "string") throw new Error(msg("响应格式无效"));
  const u = new URL(v.url);
  if (u.origin !== "https://login.eveonline.com") throw new Error(msg("响应格式无效"));
  window.location.assign(u.href);
}
export const retry = (id: string, csrf: string) => write(`/api/v1/reminder/deliveries/${encodeURIComponent(id)}/retry`, csrf, "POST");

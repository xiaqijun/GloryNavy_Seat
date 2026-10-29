import { apiFetch, APIError, getData } from "@/lib/http";
import { msg } from "@/lib/i18n";
import type { ReminderEvent } from "./api";

export interface EmailSettings { enabled: boolean; host: string; port: number; implicit_tls: boolean; username: string; sender: string; secret_configured: boolean; revision: string; events: string[]; available_events: ReminderEvent[] }
export interface EmailAccount { email: string; verified: boolean; enabled: boolean; revision: string; pending_email: string }
export interface EmailDelivery { id: string; recipient: string; event: string; notice_count: number; state: string; reason: string; created_at: string }
const isSettings = (v: unknown): v is EmailSettings => { const x = v as EmailSettings; return !!x && typeof x.enabled === "boolean" && typeof x.host === "string" && Number.isInteger(x.port) && typeof x.implicit_tls === "boolean" && typeof x.username === "string" && typeof x.sender === "string" && typeof x.secret_configured === "boolean" && typeof x.revision === "string" && Array.isArray(x.events) && Array.isArray(x.available_events); };
const isAccount = (v: unknown): v is EmailAccount => { const x = v as EmailAccount; return !!x && typeof x.email === "string" && typeof x.verified === "boolean" && typeof x.enabled === "boolean" && typeof x.revision === "string" && typeof x.pending_email === "string"; };
const isDeliveries = (v: unknown): v is EmailDelivery[] => Array.isArray(v) && v.every(x => x && typeof x.id === "string" && typeof x.recipient === "string" && typeof x.event === "string" && Number.isInteger(x.notice_count) && typeof x.state === "string" && typeof x.created_at === "string");
async function write(path: string, method: string, value: unknown, csrf: string) {
  const r = await apiFetch(path, { method, credentials: "same-origin", headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf }, body: JSON.stringify(value) });
  const v = await r.json().catch(() => null); if (!r.ok) throw new APIError(v?.error?.message ?? msg("操作失败，请稍后重试"), r.status); return v?.data as unknown;
}
export const settings = (signal?: AbortSignal) => getData("/api/v1/reminder/email/settings", isSettings, signal);
export const account = (signal?: AbortSignal) => getData("/api/v1/reminder/email/account", isAccount, signal);
export const deliveries = (signal?: AbortSignal) => getData("/api/v1/reminder/email/deliveries", isDeliveries, signal);
export async function saveSettings(value: Omit<EmailSettings, "available_events" | "secret_configured"> & { secret: string }, csrf: string) { const v = await write("/api/v1/reminder/email/settings", "PUT", value, csrf); if (!isSettings(v)) throw new Error(msg("响应格式无效")); return v; }
export async function saveAccount(revision: string, enabled: boolean, csrf: string) { const v = await write("/api/v1/reminder/email/account", "PUT", { revision, enabled }, csrf); if (!isAccount(v)) throw new Error(msg("响应格式无效")); return v; }
export const challenge = (address: string, csrf: string) => write("/api/v1/reminder/email/challenge", "POST", { address }, csrf);
export async function verify(code: string, csrf: string) { const v = await write("/api/v1/reminder/email/verify", "POST", { code }, csrf); if (!isAccount(v)) throw new Error(msg("响应格式无效")); return v; }
export const retry = (id: string, csrf: string) => write(`/api/v1/reminder/email/deliveries/${encodeURIComponent(id)}/retry`, "POST", {}, csrf);

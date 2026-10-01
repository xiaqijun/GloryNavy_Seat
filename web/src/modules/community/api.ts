import { apiFetch } from "@/lib/http";
import { msg } from "@/lib/i18n";
import { APIError, getData } from "@/lib/http";
export interface Binding {
  value: string;
  version: string;
  confirmation: "unfilled" | "pending" | "confirmed";
}
export interface Profile {
  version: string;
  complete: boolean;
  qq: Binding;
  kook: Binding;
}
export interface Details {
  qq_number: string;
  kook_name: string;
  version: string;
}
const isBinding = (v: unknown): v is Binding => {
  if (!v || typeof v !== "object") return false;
  const b = v as Partial<Binding>;
  return (
    typeof b.value === "string" &&
    typeof b.version === "string" &&
    /^\d+$/.test(b.version) &&
    ["unfilled", "pending", "confirmed"].includes(b.confirmation ?? "")
  );
};
export function isProfile(v: unknown): v is Profile {
  if (!v || typeof v !== "object") return false;
  const p = v as Partial<Profile>;
  return (
    typeof p.version === "string" &&
    /^\d+$/.test(p.version) &&
    typeof p.complete === "boolean" &&
    isBinding(p.qq) &&
    isBinding(p.kook)
  );
}
export const getProfile = (signal?: AbortSignal) =>
  getData("/api/v1/community/profile", isProfile, signal);
export async function saveProfile(details: Details, csrf: string) {
  const response = await apiFetch("/api/v1/community/profile", {
    method: "PUT",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
    body: JSON.stringify(details),
  });
  const payload = await response.json().catch(() => null);
  if (!response.ok || !isProfile(payload?.data))
    throw new APIError(
      payload?.error?.message ?? msg("保存失败，请稍后重试"),
      response.status,
    );
  return payload.data as Profile;
}
export interface QQChallenge {
  code: string;
  expires_at: string;
}
const isQQChallenge = (v: unknown): v is QQChallenge => {
  if (!v || typeof v !== "object") return false;
  const c = v as Partial<QQChallenge>;
  return typeof c.code === "string" && /^[A-Z2-9]{8}$/.test(c.code) && typeof c.expires_at === "string";
};
export async function createQQChallenge(csrf: string) {
  const response = await apiFetch("/api/v1/community/qq/challenge", {
    method: "POST",
    credentials: "same-origin",
    headers: { "X-CSRF-Token": csrf },
  });
  const payload = await response.json().catch(() => null);
  if (!response.ok || !isQQChallenge(payload?.data))
    throw new APIError(payload?.error?.message ?? msg("绑定码生成失败，请稍后重试"), response.status);
  return payload.data as QQChallenge;
}

export interface QQGroupApplication {
  id: number;
  group_openid: string;
  qq_number: string;
  status: "pending" | "approving" | "approved" | "bound" | "rejected" | "expired" | "failed";
  member_openid?: string;
  join_request_id?: string;
  failure_reason?: string;
  created_at: string;
  expires_at: string;
  approved_at?: string;
  bound_at?: string;
}
export interface QQGroupApplicationChallenge extends QQGroupApplication {
  code: string;
}
const isQQGroupApplication = (v: unknown): v is QQGroupApplication => {
  if (!v || typeof v !== "object") return false;
  const a = v as Partial<QQGroupApplication>;
  return typeof a.id === "number" && typeof a.group_openid === "string" && typeof a.qq_number === "string" &&
    ["pending", "approving", "approved", "bound", "rejected", "expired", "failed"].includes(a.status ?? "") &&
    typeof a.created_at === "string" && typeof a.expires_at === "string";
};
const isQQGroupApplicationData = (v: unknown): v is QQGroupApplication | null => v === null || isQQGroupApplication(v);
const isQQGroupApplicationChallenge = (v: unknown): v is QQGroupApplicationChallenge =>
  isQQGroupApplication(v) && typeof (v as QQGroupApplicationChallenge).code === "string";
export const getQQGroupApplication = (signal?: AbortSignal) =>
  getData("/api/v1/community/qq/group/application", isQQGroupApplicationData, signal);
export async function createQQGroupApplication(csrf: string, groupOpenID = "") {
  const response = await apiFetch("/api/v1/community/qq/group/application", {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
    body: JSON.stringify({ group_openid: groupOpenID }),
  });
  const payload = await response.json().catch(() => null);
  if (!response.ok || !isQQGroupApplicationChallenge(payload?.data))
    throw new APIError(payload?.error?.message ?? msg("入群申请提交失败，请稍后重试"), response.status);
  return payload.data;
}

export async function listQQGroupApplications(
  signal?: AbortSignal,
  filters: { group_openid?: string; status?: QQGroupApplication["status"]; limit?: number } = {},
) {
  const params = new URLSearchParams();
  if (filters.group_openid) params.set("group_openid", filters.group_openid);
  if (filters.status) params.set("status", filters.status);
  if (filters.limit) params.set("limit", String(filters.limit));
  const suffix = params.toString() ? `?${params.toString()}` : "";
  return getData(
    `/api/v1/community/qq/group/applications${suffix}`,
    (value: unknown): value is { items: QQGroupApplication[] } =>
      !!value && typeof value === "object" && Array.isArray((value as { items?: unknown }).items) &&
      (value as { items: unknown[] }).items.every(isQQGroupApplication),
    signal,
  );
}

export async function syncQQGroupApplications(csrf: string) {
  const response = await apiFetch("/api/v1/community/qq/group/sync", {
    method: "POST",
    credentials: "same-origin",
    headers: { "X-CSRF-Token": csrf },
  });
  const payload = await response.json().catch(() => null);
  if (!response.ok) {
    throw new APIError(payload?.error?.message ?? msg("入群申请同步失败，请稍后重试"), response.status);
  }
}

export interface QQGroupSetting {
  group_openid: string;
  label: string;
  enabled: boolean;
}
export interface QQGroupSettings {
  items: QQGroupSetting[];
  initialized: boolean;
}
const isQQGroupSetting = (v: unknown): v is QQGroupSetting => {
  if (!v || typeof v !== "object") return false;
  const item = v as Partial<QQGroupSetting>;
  return typeof item.group_openid === "string" &&
    typeof item.label === "string" && typeof item.enabled === "boolean";
};
export const isQQGroupSettings = (v: unknown): v is QQGroupSettings =>
  !!v && typeof v === "object" && typeof (v as QQGroupSettings).initialized === "boolean" &&
  Array.isArray((v as QQGroupSettings).items) &&
  (v as QQGroupSettings).items.every(isQQGroupSetting);
export const getQQGroupSettings = (signal?: AbortSignal) =>
  getData("/api/v1/community/qq/group/settings", isQQGroupSettings, signal);
export interface QQGroupOptions {
  items: QQGroupSetting[];
}
export const isQQGroupOptions = (v: unknown): v is QQGroupOptions =>
  !!v && typeof v === "object" && Array.isArray((v as QQGroupOptions).items) &&
  (v as QQGroupOptions).items.every(isQQGroupSetting);
export const getQQGroupOptions = (signal?: AbortSignal) =>
  getData("/api/v1/community/qq/group/options", isQQGroupOptions, signal);
export async function saveQQGroupSettings(items: QQGroupSetting[], csrf: string) {
  const response = await apiFetch("/api/v1/community/qq/group/settings", {
    method: "PUT",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
    body: JSON.stringify({ items }),
  });
  const payload = await response.json().catch(() => null);
  if (!response.ok || !isQQGroupSettings(payload?.data))
    throw new APIError(payload?.error?.message ?? msg("保存失败，请稍后重试"), response.status);
  return payload.data as QQGroupSettings;
}

export interface QQBotSettings {
  app_id: string;
  api_base: string;
  secret_configured: boolean;
  initialized: boolean;
}
const isQQBotSettings = (v: unknown): v is QQBotSettings => {
  if (!v || typeof v !== "object") return false;
  const settings = v as Partial<QQBotSettings>;
  return typeof settings.app_id === "string" && typeof settings.api_base === "string" &&
    typeof settings.secret_configured === "boolean" && typeof settings.initialized === "boolean";
};
export const getQQBotSettings = (signal?: AbortSignal) =>
  getData("/api/v1/community/qq/bot/settings", isQQBotSettings, signal);
export async function saveQQBotSettings(settings: { app_id: string; api_base: string; secret: string }, csrf: string) {
  const response = await apiFetch("/api/v1/community/qq/bot/settings", {
    method: "PUT",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
    body: JSON.stringify(settings),
  });
  const payload = await response.json().catch(() => null);
  if (!response.ok || !isQQBotSettings(payload?.data))
    throw new APIError(payload?.error?.message ?? msg("保存失败，请稍后重试"), response.status);
  return payload.data as QQBotSettings;
}
export function validateDetails(qq: string, kook: string) {
  return {
    qq: /^[1-9]\d{4,11}$/.test(qq.trim())
      ? ""
      : msg("请输入 5–12 位 QQ 号，不能以 0 开头"),
    kook:
      Array.from(kook.trim()).length >= 1 &&
      Array.from(kook.trim()).length <= 64 &&
      !/[\p{Cc}]/u.test(kook.trim())
        ? ""
        : msg("请输入 1–64 个字符的 KOOK 昵称，不含控制字符"),
  };
}

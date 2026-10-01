import { apiFetch } from "@/lib/http";
import { msg, getLocale } from "@/lib/i18n";
import { getData, APIError } from "@/lib/http";
export type Context = {
  characters: { id: string; name: string }[];
  sources: {
    id: string;
    name: string;
    minor_per_unit: number;
    mode?: "manual" | "automatic";
    version: string;
  }[];
};
const object = (v: unknown): v is Record<string, unknown> =>
  !!v && typeof v === "object";
const id = (v: unknown) => typeof v === "string" && /^[1-9]\d*$/.test(v);
export const isSaved = (v: unknown): v is { saved: boolean } =>
  object(v) && v.saved === true;
export const getContext = (signal?: AbortSignal) =>
  getData(
    "/api/v1/exchange/context",
    (v: unknown): v is Context =>
      object(v) &&
      Array.isArray(v.characters) &&
      v.characters.every(
        (c) => object(c) && id(c.id) && typeof c.name === "string",
      ) &&
      Array.isArray(v.sources) &&
      v.sources.every(
        (c) =>
          object(c) &&
          typeof c.id === "string" &&
          typeof c.name === "string" &&
          (c.mode == null || c.mode === "manual" || c.mode === "automatic") &&
          id(c.version) &&
          Number.isSafeInteger(c.minor_per_unit) &&
          Number(c.minor_per_unit) >= 0,
      ),
    signal,
  );
export const formatDate = (v: string) =>
  new Intl.DateTimeFormat(getLocale(), {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(v));

const root = "/api/v1/exchange";
export async function write<T>(
  path: string,
  csrf: string,
  body: object,
  validate: (v: unknown) => v is T,
): Promise<T> {
  const r = await apiFetch(`${root}${path}`, {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
    body: JSON.stringify(body),
  });
  const p = await r.json().catch(() => null);
  if (!r.ok)
    throw new APIError(
      p?.error?.message ?? msg("保存失败，请重试"),
      r.status,
      p?.request_id,
    );
  if (!validate(p?.data))
    throw new APIError(msg("响应格式异常，请刷新核对结果"), r.status);
  return p.data;
}

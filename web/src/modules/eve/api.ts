import { apiFetch } from "@/lib/http";
import { msg } from "@/lib/i18n";
import { APIError, getData } from "@/lib/http";

export async function startCharacterFlow(csrf: string, characterID?: string) {
  const path = characterID
    ? `/api/v1/eve/characters/${encodeURIComponent(characterID)}/reauthorize`
    : "/api/v1/eve/characters/link";
  const response = await apiFetch(path, {
    method: "POST",
    credentials: "same-origin",
    redirect: "error",
    headers: { "X-CSRF-Token": csrf },
  });
  const payload = await response.json().catch(() => null);
  if (!response.ok || typeof payload?.data?.url !== "string")
    throw new APIError(
      payload?.error?.message ?? msg("暂时无法前往 EVE，请重试"),
      response.status,
    );
  const target = new URL(payload.data.url);
  if (target.origin !== "https://login.eveonline.com")
    throw new APIError(msg("授权地址无效"), 502);
  window.location.assign(target.href);
}
export const getEVEStatus = (signal?: AbortSignal) =>
  getData(
    "/api/v1/eve/status",
    (
      value,
    ): value is {
      configured: boolean;
      corporation_roles?: boolean;
      scopes?: string[];
    } =>
      !!value &&
      typeof value === "object" &&
      "configured" in value &&
      typeof value.configured === "boolean" &&
      (!("corporation_roles" in value) ||
        typeof value.corporation_roles === "boolean") &&
      (!("scopes" in value) ||
        (Array.isArray(value.scopes) &&
          value.scopes.every((s) => typeof s === "string"))),
    signal,
  );

export const getLoginStatus = (signal?: AbortSignal) =>
  getData(
    "/api/v1/eve/login-status",
    (v): v is { configured: boolean } =>
      !!v &&
      typeof v === "object" &&
      "configured" in v &&
      typeof v.configured === "boolean",
    signal,
  );

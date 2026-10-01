import { apiFetch } from "@/lib/http";
import { msg } from "@/lib/i18n";
import { APIError, getData } from "@/lib/http";

export interface Session {
  user_id: string;
  character: { id: string; name: string };
  main_character?: { id: string; name: string };
  csrf_token: string;
  expires_at: string;
}
export interface SessionState {
  authenticated: boolean;
  session: Session | null;
}

export interface BoundCharacter {
  id: string;
  name: string;
  status: "active" | "blocked";
  is_main: boolean;
}
export const getCharacters = (signal?: AbortSignal) =>
  getData(
    "/api/v1/identity/characters",
    (value): value is { characters: BoundCharacter[] } => {
      if (
        !value ||
        typeof value !== "object" ||
        !("characters" in value) ||
        !Array.isArray(value.characters)
      )
        return false;
      return value.characters.every((c: unknown) => {
        if (!c || typeof c !== "object") return false;
        const row = c as Partial<BoundCharacter>;
        return (
          typeof row.id === "string" &&
          /^[1-9]\d*$/.test(row.id) &&
          typeof row.name === "string" &&
          typeof row.is_main === "boolean" &&
          ["active", "blocked"].includes(row.status ?? "")
        );
      });
    },
    signal,
  );

export async function changeCharacter(
  id: string,
  action: "main" | "unlink",
  csrf: string,
) {
  const response = await apiFetch(
    `/api/v1/identity/characters/${encodeURIComponent(id)}${action === "main" ? "/main" : ""}`,
    {
      method: action === "main" ? "POST" : "DELETE",
      credentials: "same-origin",
      headers: { "X-CSRF-Token": csrf },
    },
  );
  const payload = await response.json().catch(() => null);
  if (!response.ok || payload?.data?.updated !== true)
    throw new APIError(
      payload?.error?.message ?? msg("角色操作失败，请重试"),
      response.status,
    );
}
function isSessionState(value: unknown): value is SessionState {
  if (!value || typeof value !== "object") return false;
  const data = value as Partial<SessionState>;
  if (data.authenticated === false) return data.session === null;
  const s = data.session;
  return (
    data.authenticated === true &&
    !!s &&
    typeof s.user_id === "string" &&
    typeof s.character?.name === "string" &&
    typeof s.character.id === "string" &&
    /^\d+$/.test(s.character.id) &&
    (s.main_character === undefined ||
      (typeof s.main_character.name === "string" &&
        typeof s.main_character.id === "string" &&
        /^[1-9]\d*$/.test(s.main_character.id))) &&
    typeof s.csrf_token === "string" &&
    s.csrf_token.length > 0 &&
    typeof s.expires_at === "string" &&
    Number.isFinite(Date.parse(s.expires_at))
  );
}
export const getSession = (signal?: AbortSignal) =>
  getData("/api/v1/identity/session", isSessionState, signal);
export async function logout(csrfToken: string) {
  const response = await apiFetch("/api/v1/identity/logout", {
    method: "POST",
    credentials: "same-origin",
    headers: { "X-CSRF-Token": csrfToken },
  });
  if (!response.ok && response.status !== 401) {
    const body = await response.json().catch(() => null);
    throw new APIError(
      body?.error?.message ?? msg("退出失败，请稍后重试"),
      response.status,
      body?.request_id,
    );
  }
}

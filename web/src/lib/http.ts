import { getLocale, msg } from "@/lib/i18n";

// Only same-origin business API requests use this helper. ESI remains server-owned.
export function apiFetch(path: string, options: RequestInit = {}) {
  const headers = new Headers(options.headers);
  headers.set("Accept-Language", getLocale());
  return fetch(path, { ...options, headers });
}
export class APIError extends Error {
  status: number;
  requestId?: string;
  constructor(message: string, status: number, requestId?: string) {
    super(msg(message));
    this.status = status;
    this.requestId = requestId;
  }
}

export async function getData<T>(
  path: string,
  validate: (value: unknown) => value is T,
  signal?: AbortSignal,
): Promise<T> {
  const response = await apiFetch(path, { signal, credentials: "same-origin" });
  const payload = await response.json().catch(() => null);
  if (!response.ok) {
    throw new APIError(
      payload?.error?.message ?? msg("暂时无法连接服务，请稍后重试"),
      response.status,
      payload?.request_id,
    );
  }
  if (!validate(payload?.data)) {
    throw new APIError(
      msg("服务响应格式异常（接口：{0}），请联系管理员", path),
      response.status,
      payload?.request_id,
    );
  }
  return payload.data;
}

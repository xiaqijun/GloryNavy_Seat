import { afterEach, expect, it, vi } from "vitest";
import { getSession, logout } from "./api";
afterEach(() => vi.unstubAllGlobals());
it("requires a complete session and string character IDs", async () => {
  for (const data of [
    { authenticated: true, session: null },
    { authenticated: true, session: { character: { id: 123 } } },
    { authenticated: false },
  ]) {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(new Response(JSON.stringify({ data }))),
    );
    await expect(getSession()).rejects.toThrow("服务响应格式异常");
  }
});
it("recognizes an anonymous session", async () => {
  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockResolvedValue(
        new Response(
          JSON.stringify({ data: { authenticated: false, session: null } }),
        ),
      ),
  );
  await expect(getSession()).resolves.toEqual({
    authenticated: false,
    session: null,
  });
});
it("sends CSRF protection on logout and surfaces server failures", async () => {
  const fetch = vi.fn().mockResolvedValue(
    new Response(JSON.stringify({ error: { message: "退出失败" } }), {
      status: 503,
    }),
  );
  vi.stubGlobal("fetch", fetch);
  await expect(logout("csrf")).rejects.toThrow("退出失败");
  expect(fetch).toHaveBeenCalledWith(
    "/api/v1/identity/logout",
    expect.objectContaining({
      method: "POST",
      headers: expect.any(Headers),
    }),
  );
  const headers = new Headers(fetch.mock.calls[0][1].headers);
  expect(headers.get("X-CSRF-Token")).toBe("csrf");
  expect(headers.get("Accept-Language")).toBe("zh-CN");
});

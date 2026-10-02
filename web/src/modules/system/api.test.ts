import { afterEach, describe, expect, it, vi } from "vitest";
import { getSystemStatus } from "./api";
afterEach(() => vi.unstubAllGlobals());
describe("system status contract", () => {
  it("rejects malformed successful responses", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue(
          new Response(JSON.stringify({ data: { database: "ready" } })),
        ),
    );
    await expect(getSystemStatus()).rejects.toMatchObject({
      message: expect.stringContaining("/api/v1/system/status"),
    });
  });
  it("preserves a failure request ID for troubleshooting", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            error: { message: "数据库暂未就绪" },
            request_id: "test-id",
          }),
          { status: 503 },
        ),
      ),
    );
    await expect(getSystemStatus()).rejects.toMatchObject({
      status: 503,
      requestId: "test-id",
      message: "数据库暂未就绪",
    });
  });
  it("handles a non-JSON proxy failure", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(new Response("bad gateway", { status: 502 })),
    );
    await expect(getSystemStatus()).rejects.toMatchObject({ status: 502 });
  });
});

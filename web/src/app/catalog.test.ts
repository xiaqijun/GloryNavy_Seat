import { afterEach, expect, it, vi } from "vitest";
import { getModuleCatalog } from "./catalog";

afterEach(() => vi.unstubAllGlobals());
it("rejects duplicate and malformed server catalogs", async () => {
  const valid = { id: "system", version: "0.1.0", api_version: 1 };
  for (const data of [
    [valid, valid],
    [{ ...valid, api_version: "1" }],
    [{ ...valid, version: null }],
    {},
  ]) {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(new Response(JSON.stringify({ data }))),
    );
    await expect(getModuleCatalog()).rejects.toThrow("服务响应格式异常");
  }
});

it("accepts backend-only modules without executing any external code", async () => {
  const data = [{ id: "bot", version: "0.1.0", api_version: 1 }];
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue(new Response(JSON.stringify({ data }))),
  );
  await expect(getModuleCatalog()).resolves.toEqual(data);
});

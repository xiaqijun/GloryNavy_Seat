import { describe, expect, it, vi } from "vitest";
import { Activity } from "lucide-react";
import {
  registerModules,
  selectPages,
  type FrontendModule,
} from "./module-registry";

function example(id: string): FrontendModule {
  return {
    id,
    apiVersion: 1,
    pages: [
      {
        id: `${id}.home`,
        path: `/${id}`,
        label: id,
        icon: Activity,
        load: vi.fn(async () => ({ default: () => null })),
      },
    ],
  };
}

describe("frontend module registry", () => {
  it("deduplicates page preloads and allows retry after a failed preload", async () => {
    const module = example("system");
    const failed = new Error("chunk unavailable");
    module.pages[0].load = vi.fn()
      .mockRejectedValueOnce(failed)
      .mockResolvedValue({ default: () => null });
    const page = registerModules([module])[0].pages[0];
    const first = page.preload();
    expect(page.preload()).toBe(first);
    await expect(first).rejects.toBe(failed);
    await expect(page.preload()).resolves.toHaveProperty("default");
    await page.preload();
    expect(module.pages[0].load).toHaveBeenCalledTimes(2);
  });
  it("uses the active catalog for both pages and navigation without loading disabled pages", () => {
    const system = example("system");
    const extra = example("extra");
    const pages = selectPages(registerModules([system, extra]), [
      { id: "system", api_version: 1, version: "0.1.0" },
    ]);
    expect(pages.map((page) => page.path)).toEqual(["/system"]);
    expect(system.pages[0].load).not.toHaveBeenCalled();
    expect(extra.pages[0].load).not.toHaveBeenCalled();
  });

  it("rejects unavailable required modules and incompatible contracts", () => {
    const module = example("system");
    module.required = true;
    const registry = registerModules([module]);
    expect(() => selectPages(registry, [])).toThrow(
      "Required module unavailable",
    );
    expect(() =>
      selectPages(registry, [
        { id: "system", api_version: 2, version: "1.0.0" },
      ]),
    ).toThrow("Incompatible module");
  });

  it("rejects duplicate module IDs and paths instead of silently replacing a page", () => {
    expect(() =>
      registerModules([example("system"), example("system")]),
    ).toThrow("Invalid frontend module");
    const extra = example("extra");
    extra.pages[0].path = "/system";
    expect(() => registerModules([example("system"), extra])).toThrow(
      "duplicate page",
    );
    extra.pages[0].path = "/*";
    expect(() => registerModules([extra])).toThrow("Invalid");
  });
});

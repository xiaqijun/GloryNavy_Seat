import { afterEach, describe, expect, it, vi } from "vitest";
import { translate } from "./i18n";
import { english } from "./locales/en";
import { readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";
import ts from "typescript";

afterEach(() => {
  vi.unstubAllGlobals();
  vi.resetModules();
});

describe("interface localization", () => {
  it("provides translations for fixed UI messages across modules", () => {
    const files = (dir: string): string[] =>
      readdirSync(dir, { withFileTypes: true }).flatMap((e) =>
        e.isDirectory() ? files(join(dir, e.name)) : [join(dir, e.name)],
      );
    const missing: string[] = [];
    for (const file of files("src")) {
      if (!/\.tsx?$/.test(file) || file.includes(".test.")) continue;
      const ast = ts.createSourceFile(
        file,
        readFileSync(file, "utf8"),
        ts.ScriptTarget.Latest,
        true,
      );
      const visit = (node: ts.Node) => {
        if (
          ts.isCallExpression(node) &&
          ts.isIdentifier(node.expression) &&
          node.expression.text === "msg"
        ) {
          const source = node.arguments[0];
          if (
            source &&
            ts.isStringLiteral(source) &&
            /[\u3400-\u9fff]/.test(source.text) &&
            !Object.hasOwn(english, source.text)
          )
            missing.push(`${file}: ${source.text}`);
        }
        ts.forEachChild(node, visit);
      };
      visit(ast);
    }
    expect(missing).toEqual([]);
  });
  it("preserves Chinese, unknown content and interpolation values", () => {
    expect(translate("en", "constructor")).toBe("constructor");
    expect(translate("zh-CN", "还差 {0} SP", "12,345")).toBe("还差 12,345 SP");
    expect(translate("en", "还差 {0} SP", "12,345")).toBe(
      "12,345 SP remaining",
    );
    expect(translate("en", "查看记录 {0}", "9007199254740993")).toBe(
      "View record 9007199254740993",
    );
    expect(translate("en", "Hajimi1 玩家自填备注")).toBe(
      "Hajimi1 玩家自填备注",
    );
    expect(translate("en", "移除{0}", "<script>{1}</script>")).toBe(
      "Remove <script>{1}</script>",
    );
  });

  it("retains every numbered parameter in English messages", () => {
    const params = (s: string) =>
      [...new Set(s.match(/\{\d+\}/g) ?? [])].sort();
    for (const [source, target] of Object.entries(english)) {
      expect(params(target), source).toEqual(params(source));
      expect(target, source).not.toMatch(/[\u3400-\u9fff]/);
    }
  });

  it("remembers language while preserving the current route and filters", async () => {
    const storage = new Map<string, string>();
    const replace = vi.fn();
    const reload = vi.fn();
    const historyState = { key: "route-key" };
    vi.stubGlobal("window", {
      history: { state: historyState, replaceState: replace },
      location: {
        href: "http://localhost/wallet?member=42&owner=77#history",
        reload,
      },
      localStorage: {
        getItem: (k: string) => storage.get(k),
        setItem: (k: string, v: string) => storage.set(k, v),
      },
    });
    const { changeLocale, localeStorageKey } = await import("./i18n");
    changeLocale("en");
    expect(storage.get(localeStorageKey)).toBe("en");
    expect(replace).toHaveBeenCalledWith(
      historyState,
      "",
      "http://localhost/wallet?member=42&owner=77#history",
    );
    expect(reload).toHaveBeenCalledOnce();
    vi.resetModules();
    expect((await import("./i18n")).getLocale()).toBe("en");
  });

  it("rejects unknown locales and works when preference storage is blocked", async () => {
    const replace = vi.fn();
    const reload = vi.fn();
    vi.stubGlobal("window", {
      history: { state: null, replaceState: replace },
      location: {
        href: "http://localhost/wallet?lang=invalid&member=42",
        reload,
      },
      localStorage: {
        getItem: () => {
          throw Error("blocked");
        },
        setItem: () => {
          throw Error("blocked");
        },
      },
    });
    const { getLocale, changeLocale } = await import("./i18n");
    expect(getLocale()).toBe("zh-CN");
    changeLocale("en");
    expect(replace).toHaveBeenCalledWith(
      null,
      "",
      "http://localhost/wallet?lang=en&member=42",
    );
    expect(reload).toHaveBeenCalledOnce();
  });
});

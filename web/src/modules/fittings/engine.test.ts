/// <reference types="node" />
import { beforeAll, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import * as glue from "@eveshipfit/dogma-engine/esf_dogma_engine_bg.js";
import { readCatalog, readNames, type Catalog } from "./sde";
import {
  emptyFit,
  engineInput,
  importEFT,
  exportEFT,
  savedToFit,
  type Fit,
} from "./model";
import { summarize } from "./statistics";
import { compatibleCharge, groupLabel, hullSlots } from "./catalog-ui";
let catalog: Catalog;
beforeAll(async () => {
  const { instance } = await WebAssembly.instantiate(
    readFileSync(
      "node_modules/@eveshipfit/dogma-engine/esf_dogma_engine_bg.wasm",
    ),
    { "./esf_dogma_engine_bg.js": glue },
  );
  glue.__wbg_set_wasm(instance.exports);
  (instance.exports.__wbindgen_start as () => void)();
  const data = readFileSync("node_modules/@eveshipfit/sde/dist/sde.dat");
  expect(glue.load_sde(data)).toBe(3503375);
  catalog = readCatalog(data);
});
const rifter = (): Fit => ({
  ...emptyFit(),
  ship_type_id: "587",
  skill_mode: "all0",
});
const calc = (f: Fit) =>
  summarize(glue.calculate(engineInput(f, catalog), null), f, catalog);
it("filters actual SDE charges by ammunition family and size, and reads localized hull groups", () => {
  const weapon = catalog.types.find((t) => t.id === 2873)!;
  const small = catalog.types.find((t) => t.name === "EMP S")!;
  const medium = catalog.types.find((t) => t.name === "EMP M")!;
  const missile = catalog.types.find(
    (t) => t.name === "Scourge Light Missile",
  )!;
  expect(compatibleCharge(small, weapon, catalog)).toBe(true);
  expect(compatibleCharge(medium, weapon, catalog)).toBe(false);
  expect(compatibleCharge(missile, weapon, catalog)).toBe(false);
  expect(compatibleCharge(small, undefined, catalog)).toBe(false);
  expect(hullSlots(rifter(), "high", catalog)).toBe(3);
  expect(
    groupLabel(catalog.types.find((t) => t.id === 587)!.group, catalog),
  ).toBe("护卫舰");
});
it("uses the actual pinned Dogma engine for base hull and skill bonuses", () => {
  const zero = calc(rifter()),
    five = calc({ ...rifter(), skill_mode: "all5" });
  expect(zero.cpu).toBe(130);
  expect(zero.power).toBe(41);
  expect(five.cpu).toBeCloseTo(162.5);
  expect(five.power).toBeCloseTo(51.25);
  expect(zero.resists[0].values).toEqual([
    0,
    expect.closeTo(20, 3),
    expect.closeTo(40, 3),
    50,
  ]);
  expect(zero.ehp).toBeCloseTo(1809.74437);
});
it("accounts for ammo, reload and offline state; rejects incompatible charges", () => {
  const fit = rifter();
  fit.items = [
    {
      type_id: "2873",
      slot: "high",
      index: 0,
      quantity: 1,
      state: "active",
      charge_id: "185",
    },
  ];
  const active = calc(fit);
  expect(active.cpuLoad).toBe(3);
  expect(active.powerLoad).toBe(1);
  expect(active.dps).toBeCloseTo(10.51327);
  fit.items[0].state = "offline";
  expect(calc(fit).dps).toBe(0);
  expect(calc(fit).cpuLoad).toBe(0);
  fit.items[0].charge_id = "2048";
  expect(() => calc(fit)).toThrow("弹药与装备不兼容");
});
it("does not double count drone DPS and reflects deployment quantity", () => {
  const fit = {
    ...rifter(),
    ship_type_id: "626",
    items: [
      {
        type_id: "2454",
        slot: "drone_bay" as const,
        index: 0,
        quantity: 2,
        state: "active" as const,
      },
    ],
  };
  const active = calc(fit);
  expect(active.droneDps).toBeGreaterThan(0);
  expect(active.dps).toBeCloseTo(active.droneDps);
  const stored = calc({
    ...fit,
    items: [{ ...fit.items[0], state: "online" }],
  });
  expect(stored.dps).toBe(0);
});
it("applies resistance equipment and flags overloaded fitting resources", () => {
  const fit = rifter();
  fit.items = [
    { type_id: "2048", slot: "low", index: 0, quantity: 1, state: "active" },
  ];
  expect(calc(fit).resists[0].values[0]).toBeGreaterThan(0);
  fit.items = [
    {
      type_id: "2873",
      slot: "high",
      index: 7,
      quantity: 1,
      state: "active",
      charge_id: "185",
    },
  ];
  expect(calc(fit).warnings).toContain("高能量槽超出舰船槽位");
});
it("round trips English EFT including empty slots, ammo, drones and offline equipment", () => {
  const text =
    "[Rifter, Test]\n[Empty low slot]\nDamage Control II /offline\n\n125mm Gatling AutoCannon II, EMP S\n\nHobgoblin I x2\nEMP S x1000\n";
  const fit = importEFT(text, catalog, new Map());
  expect(fit.items[0].index).toBe(1);
  expect(fit.items[0].state).toBe("offline");
  expect(importEFT(exportEFT(fit, catalog), catalog, new Map())).toEqual(fit);
  expect(() => importEFT("[Unknown, Test]", catalog, new Map())).toThrow(
    "未识别",
  );
  expect(() =>
    engineInput({ ...rifter(), skill_mode: "character" }, catalog),
  ).toThrow("技能尚未同步");
});
it("imports localized names and fails closed on unknown ESI slots and malformed data", () => {
  const names = readNames(
    readFileSync("node_modules/@eveshipfit/sde/dist/names.dat"),
  );
  const chinese = [...names.names].find(
    ([name, id]) => id === 587 && /[\u4e00-\u9fff]/.test(name),
  );
  expect(chinese).toBeDefined();
  expect(
    importEFT(`[${chinese![0]}, 测试]`, catalog, names.names).ship_type_id,
  ).toBe("587");
  expect(() => readCatalog(new Uint8Array(12))).toThrow("格式");
  expect(() =>
    savedToFit({
      id: "1",
      name: "x",
      ship_type_id: "587",
      description: "",
      items: [{ type_id: "1", flag: "Invalid", quantity: 1 }],
    }),
  ).toThrow("槽位");
});

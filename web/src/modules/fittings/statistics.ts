import { gameTerm } from "@/lib/eve-terminology";
import { msg } from "@/lib/i18n";
import type { Catalog } from "./sde";
import { slotNames, type Fit } from "./model";
type Calculated = {
  attributes: Map<number, { value: number }>;
  state: string;
  max_state: string;
  charge?: Calculated;
};
export type Statistics = {
  cpu: number | null;
  cpuLoad: number | null;
  power: number | null;
  powerLoad: number | null;
  calibration: number | null;
  calibrationLoad: number | null;
  dps: number;
  droneDps: number;
  ehp: number | null;
  speed: number | null;
  align: number | null;
  capacitor: number | null;
  capSeconds: number | null;
  resists: { name: string; hp: number | null; values: (number | null)[] }[];
  slots: Record<string, number>;
  states: { state: string; max: string }[];
  warnings: string[];
};
export function summarize(
  raw: unknown,
  fit: Fit,
  catalog: Catalog,
): Statistics {
  const r = raw as { ship: Calculated; items: Calculated[] };
  if (
    !(r.ship?.attributes instanceof Map) ||
    !Array.isArray(r.items) ||
    r.items.length !== fit.items.length
  )
    throw Error(msg("模拟结果格式异常"));
  const get = (item: Calculated, name: string) => {
    const v = item.attributes.get(catalog.attributes[name])?.value;
    return typeof v === "number" && Number.isFinite(v) ? v : null;
  };
  const a = (name: string) => get(r.ship, name);
  const slots = Object.fromEntries(
    Object.entries({
      high: "hiSlots",
      medium: "medSlots",
      low: "lowSlots",
      rig: "rigSlots",
      subsystem: "maxSubSystems",
      service: "serviceSlots",
    }).map(([k, v]) => [k, a(v) ?? 0]),
  );
  const warnings: string[] = [];
  const types = new Map(catalog.types.map((t) => [String(t.id), t]));
  for (const [slot, count] of Object.entries(slots))
    if (fit.items.some((i) => i.slot === slot && i.index >= count))
      warnings.push(
        msg("{0}超出舰船槽位", slotNames[slot as keyof typeof slotNames]),
      );
  if ((a("cpuLoad") ?? 0) > (a("cpuOutput") ?? 0))
    warnings.push(msg("CPU 超出上限"));
  if ((a("powerLoad") ?? 0) > (a("powerOutput") ?? 0))
    warnings.push(msg("能量栅格超出上限"));
  fit.items.forEach((i, n) => {
    const t = types.get(i.type_id);
    if (
      t?.slot &&
      i.slot !== t.slot &&
      i.slot !== "cargo" &&
      !(t.slot === "fighter_bay" && i.slot === "fighter_tube")
    )
      warnings.push(msg("槽位不匹配：{0}", t.name));
    if (i.state === "overload" && r.items[n].max_state !== "overload")
      warnings.push(msg("装备状态已调整：{0}", t?.name ?? i.type_id));
    if (i.charge_id && r.items[n].charge?.state === "offline")
      warnings.push(msg("弹药未生效：{0}", t?.name ?? i.type_id));
  });
  for (const [load, capacity, label] of [
    ["upgradeLoad", "upgradeCapacity", gameTerm("attributes", "calibration")],
    [
      "droneBandwidthLoad",
      "droneBandwidth",
      gameTerm("attributes", "droneBandwidth"),
    ],
    ["droneCapacityLoad", "droneCapacity", gameTerm("slots", "drone_bay")],
    ["droneActive", "maxActiveDrones", msg("出战无人机数量")],
    ["turretSlotsLeft", "", gameTerm("attributes", "turretHardpoints")],
    ["launcherSlotsLeft", "", gameTerm("attributes", "launcherHardpoints")],
  ]) {
    if (
      capacity
        ? a(load) !== null && a(capacity) !== null && a(load)! > a(capacity)!
        : a(load) !== null && a(load)! < 0
    )
      warnings.push(msg("{0}超出上限", label));
  }
  const resists = [
    [gameTerm("attributes", "shield"), "shieldCapacity", "shield"],
    [gameTerm("attributes", "armor"), "armorHP", "armor"],
    [gameTerm("attributes", "structure"), "hp", ""],
  ].map(([name, hp, prefix]) => ({
    name,
    hp: a(hp),
    values: ["Em", "Thermal", "Kinetic", "Explosive"].map((d) => {
      const v = a(
        prefix
          ? `${prefix}${d}DamageResonance`
          : `${d[0].toLowerCase()}${d.slice(1)}DamageResonance`,
      );
      return v === null ? null : 100 * (1 - v);
    }),
  }));
  return {
    cpu: a("cpuOutput"),
    cpuLoad: a("cpuLoad"),
    power: a("powerOutput"),
    powerLoad: a("powerLoad"),
    calibration: a("upgradeCapacity"),
    calibrationLoad: a("upgradeLoad"),
    dps: a("damagePerSecondWithReload") ?? 0,
    droneDps: a("droneDamagePerSecond") ?? 0,
    ehp: a("ehp"),
    speed: a("maxVelocity"),
    align: a("alignTime"),
    capacitor: a("capacitorCapacity"),
    capSeconds: a("capacitorDepletesIn"),
    resists,
    slots,
    states: r.items.map((i) => ({ state: i.state, max: i.max_state })),
    warnings: [...new Set(warnings)],
  };
}

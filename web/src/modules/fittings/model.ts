import { gameLabels } from "@/lib/eve-terminology";
import { msg } from "@/lib/i18n";
import type { Catalog, TypeInfo } from "./sde";
export type Slot =
  | "high"
  | "medium"
  | "low"
  | "rig"
  | "subsystem"
  | "service"
  | "fighter_tube"
  | "fighter_bay"
  | "drone_bay"
  | "cargo";
export type FitItem = {
  type_id: string;
  slot: Slot;
  index: number;
  quantity: number;
  state: "offline" | "online" | "active" | "overload";
  charge_id?: string;
};
export type Fit = {
  name: string;
  ship_type_id: string;
  mode_id?: string;
  skill_mode: "all5" | "all0" | "character";
  character_id?: string;
  items: FitItem[];
};
export type Draft = {
  id: string;
  name: string;
  version: string;
  updated_at: string;
  fit?: Fit;
  can_edit: boolean;
};
export type SavedFit = {
  id: string;
  name: string;
  description: string;
  ship_type_id: string;
  items: { flag: string; quantity: number; type_id: string }[];
};
export const slotNames: Record<Slot, string> = gameLabels("slots");
export const emptyFit = (): Fit => ({
  name: msg("新配装"),
  ship_type_id: "",
  skill_mode: "all5",
  items: [],
});
export function savedToFit(saved: SavedFit): Fit {
  const slots: Record<string, Slot> = {
    HiSlot: "high",
    MedSlot: "medium",
    LoSlot: "low",
    RigSlot: "rig",
    SubSystemSlot: "subsystem",
    ServiceSlot: "service",
    Cargo: "cargo",
    DroneBay: "drone_bay",
    FighterBay: "fighter_bay",
  };
  return {
    name: saved.name,
    ship_type_id: saved.ship_type_id,
    skill_mode: "all5",
    items: saved.items.map((i) => {
      const m = i.flag.match(/^(.*?)(\d+)?$/)!;
      const slot = slots[m[1]];
      if (!slot) throw Error(msg("无法模拟槽位 {0}", i.flag));
      return {
        type_id: i.type_id,
        slot,
        index: Number(m[2] ?? 0),
        quantity: i.quantity,
        state:
          slot === "drone_bay" || slot === "fighter_bay" ? "online" : "active",
      };
    }),
  };
}
export function importEFT(
  text: string,
  catalog: Catalog,
  localized: Map<string, number>,
): Fit {
  if (text.length > 200000) throw Error(msg("配装文本过长"));
  const lines = text
    .replaceAll("\r", "")
    .split("\n")
    .map((s) => s.trim());
  const header = lines.shift()?.match(/^\[([^,]+),\s*(.+)\]$/);
  if (!header) throw Error(msg("首行应为 [舰船名称, 方案名称]"));
  const byName = new Map(
    catalog.types.map((t) => [t.name.toLowerCase(), t.id]),
  );
  const byID = new Map(catalog.types.map((t) => [t.id, t]));
  const resolve = (name: string): TypeInfo => {
    const id =
      byName.get(name.toLowerCase()) ?? localized.get(name.toLowerCase());
    const t = id ? byID.get(id) : undefined;
    if (!t) throw Error(msg("未识别物品：{0}", name));
    return t;
  };
  const ship = resolve(header[1]);
  if (ship.category !== 6) throw Error(msg("请选择舰船类型"));
  const f: Fit = {
    name: header[2],
    ship_type_id: String(ship.id),
    skill_mode: "all5",
    items: [],
  };
  const indices: Record<string, number> = {};
  const empty: Record<string, Slot> = {
    low: "low",
    med: "medium",
    high: "high",
    rig: "rig",
    service: "service",
  };
  for (const line of lines) {
    if (!line) continue;
    const gap = line.match(/^\[Empty (low|med|high|rig|service) slot\]$/i);
    if (gap) {
      const slot = empty[gap[1].toLowerCase()];
      indices[slot] = (indices[slot] ?? 0) + 1;
      continue;
    }
    const offline = /\s*\/offline$/i.test(line);
    const raw = line.replace(/\s*\/offline$/i, "");
    const stack = raw.match(/^(.*?)\s+x(\d+)$/);
    const quantity = stack ? Number(stack[2]) : 1;
    const pair = (stack ? stack[1] : raw).split(/,\s*/);
    const item = resolve(pair[0]);
    if (!Number.isSafeInteger(quantity) || quantity < 1 || quantity > 1000000)
      throw Error(msg("物品数量无效"));
    const slot = (item.slot || "cargo") as Slot;
    const isModule = !["cargo", "drone_bay", "fighter_bay"].includes(slot);
    const charge = pair[1] ? resolve(pair[1]) : undefined;
    if (pair.length > 2 || (charge && charge.category !== 8))
      throw Error(msg("弹药无效：{0}", line));
    if (isModule && quantity !== 1) throw Error(msg("装备须逐行填写"));
    const index = isModule ? (indices[slot] ?? 0) : 0;
    indices[slot] = index + 1;
    f.items.push({
      type_id: String(item.id),
      slot,
      index,
      quantity,
      state: offline ? "offline" : isModule ? "active" : "online",
      ...(charge ? { charge_id: String(charge.id) } : {}),
    });
  }
  if (f.items.length > 512) throw Error(msg("配装物品超过512项"));
  return f;
}
export function exportEFT(f: Fit, catalog: Catalog): string {
  const types = new Map(catalog.types.map((t) => [String(t.id), t]));
  const name = (id: string) => {
    const t = types.get(id);
    if (!t) throw Error(msg("模拟数据缺少物品 #{0}", id));
    return t.name;
  };
  const out = [`[${name(f.ship_type_id)}, ${f.name}]`];
  const empty: Record<string, string> = {
    low: "low",
    medium: "med",
    high: "high",
    rig: "rig",
    service: "service",
  };
  for (const slot of [
    "low",
    "medium",
    "high",
    "rig",
    "subsystem",
    "service",
  ] as Slot[]) {
    const items = f.items
      .filter((i) => i.slot === slot)
      .sort((a, b) => a.index - b.index);
    let at = 0;
    for (const i of items) {
      while (at < i.index) {
        if (empty[slot]) out.push(`[Empty ${empty[slot]} slot]`);
        at++;
      }
      out.push(
        name(i.type_id) +
          (i.charge_id ? `, ${name(i.charge_id)}` : "") +
          (i.state === "offline" ? " /offline" : ""),
      );
      at++;
    }
    out.push("");
  }
  for (const slot of [
    "drone_bay",
    "fighter_bay",
    "fighter_tube",
    "cargo",
  ] as Slot[]) {
    for (const i of f.items.filter((i) => i.slot === slot))
      out.push(`${name(i.type_id)} x${i.quantity}`);
    out.push("");
  }
  return out.join("\n").trim() + "\n";
}

export function engineInput(
  f: Fit,
  catalog: Catalog,
  skills?: Record<string, number>,
) {
  const types = new Map(catalog.types.map((t) => [String(t.id), t]));
  if (types.get(f.ship_type_id)?.category !== 6) throw Error(msg("请选择舰船"));
  if (f.mode_id) throw Error(msg("当前版本暂不支持战术模式，请移除模式后模拟"));
  if (f.items.length > 512) throw Error(msg("配装物品超过512项"));
  if (f.skill_mode === "character" && !skills)
    throw Error(msg("角色技能尚未同步"));
  for (const i of f.items) {
    const type = types.get(i.type_id),
      charge = i.charge_id ? types.get(i.charge_id) : undefined;
    if (!type || (i.charge_id && !charge))
      throw Error(msg("模拟数据缺少配装中的物品"));
    if (!Number.isInteger(i.quantity) || i.quantity < 1 || i.quantity > 1000000)
      throw Error(msg("物品数量无效"));
    if (i.charge_id) {
      const groups = [1, 2, 3, 4, 5]
        .map((n) => type.attributes[catalog.attributes[`chargeGroup${n}`]])
        .filter(Boolean);
      if (charge?.category !== 8 || !groups.includes(charge.group))
        throw Error(msg("弹药与装备不兼容：{0}", type.name));
      const size = type.attributes[catalog.attributes.chargeSize],
        chargeSize = charge.attributes[catalog.attributes.chargeSize];
      if (size && size !== chargeSize)
        throw Error(msg("弹药尺寸不匹配：{0}", type.name));
    }
  }
  return {
    ship: { type_id: Number(f.ship_type_id) },
    items: f.items.map((i) => ({
      type_id: Number(i.type_id),
      slot: {
        type: i.slot,
        ...(!["cargo", "drone_bay", "fighter_bay"].includes(i.slot)
          ? { index: i.index }
          : {}),
      },
      state:
        i.slot === "drone_bay" && i.state === "online" ? "offline" : i.state,
      quantity: i.quantity,
      ...(i.charge_id ? { charge: { type_id: Number(i.charge_id) } } : {}),
    })),
    character: {
      skills:
        f.skill_mode === "all5"
          ? Object.fromEntries(catalog.skills.map((id) => [id, 5]))
          : f.skill_mode === "all0"
            ? {}
            : skills,
    },
  };
}

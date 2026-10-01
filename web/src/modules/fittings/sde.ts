import { msg } from "@/lib/i18n";
// Read the pinned upstream FlatBuffer schema (specs/eve.fbs), without extracting
// files or fetching executable code. Calculation itself belongs to the WASM engine.
export class FlatData {
  view: DataView;
  bytes: Uint8Array;
  root: number;
  constructor(bytes: Uint8Array, identifier: string) {
    this.bytes = bytes;
    this.view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
    if (new TextDecoder().decode(bytes.subarray(4, 8)) !== identifier)
      throw Error(msg("SDE格式不匹配"));
    this.root = this.view.getUint32(0, true);
  }
  field(table: number, index: number) {
    const v = table - this.view.getInt32(table, true);
    const p = 4 + index * 2;
    return p < this.view.getUint16(v, true)
      ? this.view.getUint16(v + p, true)
      : 0;
  }
  int(table: number, index: number) {
    const f = this.field(table, index);
    return f ? this.view.getInt32(table + f, true) : 0;
  }
  bool(table: number, index: number) {
    const f = this.field(table, index);
    return f ? this.view.getUint8(table + f) !== 0 : false;
  }
  string(table: number, index: number) {
    const f = this.field(table, index);
    if (!f) return "";
    return this.text(table + f);
  }
  text(p: number) {
    const start = p + this.view.getUint32(p, true);
    const len = this.view.getUint32(start, true);
    return new TextDecoder().decode(
      this.bytes.subarray(start + 4, start + 4 + len),
    );
  }
  vector(table: number, index: number) {
    const f = this.field(table, index);
    if (!f) return { start: 0, count: 0 };
    const p = table + f;
    const start = p + this.view.getUint32(p, true);
    return { start: start + 4, count: this.view.getUint32(start, true) };
  }
  tables(table: number, index: number) {
    const v = this.vector(table, index);
    return Array.from({ length: v.count }, (_, i) => {
      const p = v.start + i * 4;
      return p + this.view.getUint32(p, true);
    });
  }
}
export type TypeInfo = {
  id: number;
  name: string;
  category: number;
  group: number;
  slot: string;
  attributes: Record<number, number>;
};
export type Catalog = {
  build: number;
  types: TypeInfo[];
  attributes: Record<string, number>;
  skills: number[];
  groups?: Record<number, string>;
};
export function readCatalog(bytes: Uint8Array): Catalog {
  const d = new FlatData(bytes, "ESF1");
  const effects = new Map(
    d.tables(d.root, 5).map((t) => [d.int(t, 0), d.string(t, 1)]),
  );
  const attributes = Object.fromEntries(
    d.tables(d.root, 4).map((t) => [d.string(t, 1), d.int(t, 0)]),
  );
  const types: TypeInfo[] = [];
  const skills: number[] = [];
  const slotEffects: Record<string, string> = {
    hiPower: "high",
    medPower: "medium",
    loPower: "low",
    rigSlot: "rig",
    subSystem: "subsystem",
    serviceSlot: "service",
  };
  for (const t of d.tables(d.root, 1)) {
    const id = d.int(t, 0),
      category = d.int(t, 3);
    if (category === 16) skills.push(id);
    if (!d.bool(t, 4)) continue;
    const attrs: Record<number, number> = {};
    const av = d.vector(t, 13);
    for (let i = 0; i < av.count; i++) {
      const p = av.start + i * 8;
      attrs[d.view.getInt32(p, true)] = d.view.getFloat32(p + 4, true);
    }
    const ev = d.vector(t, 14);
    let slot = "";
    for (let i = 0; i < ev.count; i++) {
      const name = effects.get(d.view.getInt32(ev.start + i * 8, true));
      if (name && slotEffects[name]) slot = slotEffects[name];
    }
    if (category === 18) slot = "drone_bay";
    if (category === 87) slot = "fighter_bay";
    if ([6, 7, 8, 18, 32, 66, 87].includes(category) || slot)
      types.push({
        id,
        name: d.string(t, 1),
        category,
        group: d.int(t, 2),
        slot,
        attributes: attrs,
      });
  }
  return {
    build: d.int(d.root, 0),
    types,
    attributes,
    skills,
    groups: Object.fromEntries(
      d.tables(d.root, 2).map((t) => [d.int(t, 0), d.string(t, 1)]),
    ),
  };
}
export function readNames(bytes: Uint8Array) {
  const d = new FlatData(bytes, "ESFN");
  const names = d.vector(d.root, 1),
    ids = d.vector(d.root, 2);
  if (names.count !== ids.count) throw Error(msg("SDE名称格式不匹配"));
  const out = new Map<string, number>();
  for (let i = 0; i < names.count; i++)
    out.set(
      d.text(names.start + i * 4),
      d.view.getInt32(ids.start + i * 4, true),
    );
  return { build: d.int(d.root, 0), names: out };
}

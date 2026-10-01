import { msg } from "@/lib/i18n";
import groupNames from "./group-names.json";
import type { Catalog, TypeInfo } from "./sde";
import type { Fit, Slot } from "./model";

export function groupLabel(id: number, catalog: Catalog) {
  const localized: Record<string, string> = groupNames.groups;
  return (
    (catalog.build === groupNames.build ? localized[String(id)] : undefined) ||
    catalog.groups?.[id] ||
    msg("分类 #{0}", id)
  );
}

export function compatibleCharge(
  charge: TypeInfo,
  module: TypeInfo | undefined,
  catalog: Catalog,
) {
  if (!module || charge.category !== 8) return false;
  const groups = [1, 2, 3, 4, 5]
    .map((n) => module.attributes[catalog.attributes[`chargeGroup${n}`]])
    .filter(Boolean);
  const size = module.attributes[catalog.attributes.chargeSize];
  return (
    groups.includes(charge.group) &&
    (!size || charge.attributes[catalog.attributes.chargeSize] === size)
  );
}

export const slotAttributes: Partial<Record<Slot, string>> = {
  high: "hiSlots",
  medium: "medSlots",
  low: "loSlots",
  rig: "rigSlots",
  subsystem: "maxSubSystems",
  service: "serviceSlots",
};

export function hullSlots(
  fit: Fit,
  slot: Slot,
  catalog: Catalog,
  calculated?: Record<string, number>,
) {
  const attribute = slotAttributes[slot];
  if (!attribute) return undefined;
  const hull = catalog.types.find((t) => String(t.id) === fit.ship_type_id);
  return (
    calculated?.[slot] ?? hull?.attributes[catalog.attributes[attribute]] ?? 0
  );
}

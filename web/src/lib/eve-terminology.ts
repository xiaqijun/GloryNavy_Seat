import catalog from "./eve-terminology.json";
import { getLocale, type Locale } from "./i18n";

type Group = Exclude<keyof typeof catalog, "metadata">;
type Term = { zh: string; en: string };

// Game names bypass application copy translation: identical Chinese strings
// may have different meanings in different EVE domains.
export function gameTerm(
  group: Group,
  code: string,
  language: Locale = getLocale(),
): string {
  const entries: Record<string, Term> = catalog[group];
  if (!Object.hasOwn(entries, code)) return code;
  const term = entries[code];
  return (language === "en" ? term.en : term.zh) || term.en || code;
}

export function gameLabels<G extends Group>(
  group: G,
  language: Locale = getLocale(),
) {
  return Object.fromEntries(
    Object.keys(catalog[group]).map((code) => [
      code,
      gameTerm(group, code, language),
    ]),
  ) as {
    [K in keyof (typeof catalog)[G]]: string;
  };
}

const flagSlots: Record<string, keyof typeof catalog.slots> = {
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

export function assetSlotLabel(flag: string, numbered = false): string {
  const match =
    /^(HiSlot|MedSlot|LoSlot|RigSlot|SubSystemSlot|ServiceSlot)(\d+)$/.exec(
      flag,
    );
  const key = match?.[1] ?? flag;
  if (!Object.hasOwn(flagSlots, key)) return flag;
  const label = gameTerm("slots", flagSlots[key]);
  if (!numbered || !match) return label;
  const index = Number(match[2]);
  return Number.isSafeInteger(index + 1) ? `${label} ${index + 1}` : flag;
}

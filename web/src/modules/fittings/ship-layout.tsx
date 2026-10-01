import { msg } from "@/lib/i18n";
import { useState } from "react";
import { Orbit, Plus } from "lucide-react";
import { IconAction } from "@/components/ui/icon-action";
import { EveImage } from "@/components/eve-image";
import type { Catalog } from "./sde";
import { slotNames, type Fit, type Slot } from "./model";
import { hullSlots } from "./catalog-ui";
import type { Statistics } from "./statistics";

export type SelectedSlot = { slot: Slot; index: number };
export function ShipLayout({
  fit,
  catalog,
  stats,
  name,
  selected,
  onSelect,
}: {
  fit: Fit;
  catalog: Catalog;
  stats?: Statistics;
  name: (id: string) => string;
  selected: SelectedSlot | null;
  onSelect: (selected: SelectedSlot) => void;
}) {
  const [failed, setFailed] = useState("");
  const rings: {
    slot: Slot;
    start: number;
    step: number;
    radius: number;
    limit: number;
  }[] = [
    { slot: "high", start: -136, step: 16, radius: 41, limit: 8 },
    { slot: "medium", start: -16, step: 16, radius: 41, limit: 8 },
    { slot: "low", start: 104, step: 16, radius: 41, limit: 8 },
    { slot: "rig", start: 62, step: 28, radius: 27, limit: 3 },
  ];
  return (
    <div
      className="fit-ship-layout"
      role="group"
      aria-label={msg("舰船槽位布局")}
    >
      <svg viewBox="0 0 480 480" aria-hidden="true" className="fit-orbits">
        <circle cx="240" cy="240" r="198" />
        <circle cx="240" cy="240" r="158" />
        <path d="M240 18v18M462 240h-18M240 462v-18M18 240h18" />
      </svg>
      <div className="fit-ship-render">
        {failed === fit.ship_type_id ? (
          <Orbit size={96} aria-hidden="true" />
        ) : (
          <img
            src={`https://images.evetech.net/types/${fit.ship_type_id}/render?size=512&tenant=tranquility`}
            alt={name(fit.ship_type_id)}
            onError={() => setFailed(fit.ship_type_id)}
          />
        )}
      </div>
      {rings.flatMap(({ slot, start, step, radius, limit }) =>
        Array.from(
          {
            length: Math.min(
              limit,
              Math.max(0, hullSlots(fit, slot, catalog, stats?.slots) ?? 0),
            ),
          },
          (_, index) => {
            const itemIndex = fit.items.findIndex(
              (item) => item.slot === slot && item.index === index,
            );
            const item = fit.items[itemIndex];
            const state = stats?.states[itemIndex]?.state ?? item?.state;
            const angle = ((start + index * step) * Math.PI) / 180;
            return (
              <IconAction
                key={`${slot}-${index}`}
                label={`${slotNames[slot]} ${index + 1}：${item ? name(item.type_id) : msg("空槽")}`}
                className={`fit-wheel-slot ${item ? "is-filled" : ""} ${state === "offline" ? "is-offline" : ""}`}
                style={{
                  left: `${50 + radius * Math.cos(angle)}%`,
                  top: `${50 + radius * Math.sin(angle)}%`,
                }}
                aria-pressed={
                  selected?.slot === slot && selected.index === index
                }
                onClick={() => onSelect({ slot, index })}
              >
                {item ? (
                  <EveImage kind="type" id={item.type_id} />
                ) : (
                  <Plus size={16} />
                )}
                <span className="fit-slot-number" aria-hidden="true">
                  {slotNames[slot].slice(0, 1)}
                  {index + 1}
                </span>
              </IconAction>
            );
          },
        ),
      )}
    </div>
  );
}

import "./reward-summary.css";
import { Coins } from "lucide-react";
import { msg, getLocale } from "@/lib/i18n";
import { EveImage } from "@/components/eve-image";
import type { PhysicalContent } from "@/modules/exchange/catalog-api";
export function RewardSummary({
  rewards,
  compact = false,
}: {
  rewards?: PhysicalContent & { coins_minor?: number };
  compact?: boolean;
}) {
  if (!rewards) return null;
  return (
    <section
      className={`growth-reward-summary${compact ? " growth-reward-summary-compact" : ""}`}
      aria-label={msg("发放内容")}
    >
      {!compact && <strong>{msg("发放内容")}</strong>}
      {rewards.fittings?.map((f) => (
        <div key={`f-${f.fitting_id}`}>
          <EveImage kind="type" id={f.ship_type_id || "0"} />
          <span>{f.name || `#${f.fitting_id}`}</span>
          <b>× {f.quantity}</b>
        </div>
      ))}
      {rewards.items?.map((i) => (
        <div key={`i-${i.type_id}`}>
          <EveImage kind="type" id={i.type_id} />
          <span>{i.name || `#${i.type_id}`}</span>
          <b>× {i.quantity}</b>
        </div>
      ))}
      {(rewards.isk_minor || 0) > 0 && (
        <div>
          <Coins size={24} />
          <span>ISK</span>
          <b>
            {((rewards.isk_minor || 0) / 100).toLocaleString(getLocale(), {
              maximumFractionDigits: 2,
            })}
          </b>
        </div>
      )}
      {(rewards.coins_minor || 0) > 0 && (
        <div>
          <Coins size={24} />
          <span>{msg("果壳币")}</span>
          <b>
            {((rewards.coins_minor || 0) / 100).toLocaleString(getLocale())}
          </b>
        </div>
      )}
    </section>
  );
}

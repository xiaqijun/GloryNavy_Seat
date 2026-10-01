import { msg } from "@/lib/i18n";
import { MapPin } from "lucide-react";
import { formatDate } from "./api";

// Historical fleet/loss location only; never fall back to a pilot's current position.
export function StarLocation({
  id,
  name,
  at,
  label = msg("点名星系"),
}: {
  id?: string | null;
  name?: string;
  at?: string | null;
  label?: string;
}) {
  const recorded = !!id && /^[1-9]\d*$/.test(id);
  return (
    <small className="attendance-location">
      <MapPin size={14} aria-hidden="true" />
      <span title={recorded ? msg("星系 ID：{0}", id) : undefined}>
        {label} {recorded ? name || `#${id}` : msg("未记录")}
      </span>
      {recorded && at && <time dateTime={at}>{formatDate(at)}</time>}
    </small>
  );
}

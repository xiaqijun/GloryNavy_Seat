import { ShieldCheck } from "lucide-react";
import { Button } from "@/components/ui/button";
import { msg } from "@/lib/i18n";
import type { Permission } from "./manage-api";

const officerPresets = [
  { name: "补损专员", permission: "corporation.welfare.compensation" },
  { name: "福利发放专员", permission: "corporation.welfare" },
  { name: "贷款专员", permission: "corporation.loan" },
] as const;

export type OfficerPreset = (typeof officerPresets)[number];

export function OfficerPresets({ permissions, busy, onSelect }: {
  permissions: Permission[];
  busy: boolean;
  onSelect: (preset: OfficerPreset) => void;
}) {
  const available = officerPresets.filter((preset) =>
    permissions.some((permission) => permission.id === preset.permission),
  );
  if (!available.length) return null;
  return (
    <section className="access-officer-presets" aria-label={msg("专员配置")}>
      <h2>{msg("专员配置")}</h2>
      <div>
        {available.map((preset) => (
          <Button key={preset.permission} variant="outline" disabled={busy}
            onClick={() => onSelect(preset)}>
            <ShieldCheck aria-hidden="true" />
            {msg(preset.name)}
          </Button>
        ))}
      </div>
    </section>
  );
}

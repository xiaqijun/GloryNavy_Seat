import { useState } from "react";
import { Check, Copy } from "lucide-react";
import { Button } from "@/components/ui/button";
import { msg } from "@/lib/i18n";
import type { Case } from "./api";

function CopyField({
  label,
  value,
  action,
  wide = false,
}: {
  label: string;
  value: string;
  action: string;
  wide?: boolean;
}) {
  const [status, setStatus] = useState<"idle" | "copied" | "failed">("idle");
  return (
    <div className={`welfare-contract-copy${wide ? " wide" : ""}`}>
      <label>
        {label}
        <input
          readOnly
          value={value}
          onFocus={(e) => e.currentTarget.select()}
        />
      </label>
      <Button
        variant="outline"
        aria-label={action}
        title={action}
        disabled={!value}
        onClick={async () => {
          try {
            await navigator.clipboard.writeText(value);
            setStatus("copied");
          } catch {
            setStatus("failed");
          }
        }}
      >
        {status === "copied" ? (
          <Check size={16} aria-hidden="true" />
        ) : (
          <Copy size={16} aria-hidden="true" />
        )}
      </Button>
      {status === "copied" && <small role="status">{msg("已复制")}</small>}
      {status === "failed" && (
        <small role="alert">{msg("复制失败，请手动复制")}</small>
      )}
    </div>
  );
}

export function ContractHandoff({ item }: { item: Case }) {
  const packaged = item.kind.startsWith("growth_") || item.kind.startsWith("activity_");
  const cents =
    Number.isSafeInteger(item.award_minor) && item.award_minor > 0
      ? BigInt(item.award_minor)
      : null;
  const amount = cents === null ? "" : String(cents / 100n);
  const growthISK = item.detail.rewards?.isk_minor || 0;
  // A stable site reference for the game contract description, not an ESI ID
  // or proof of payment. Existing delivery verification remains authoritative.
  return (
    <section className="welfare-contract-handoff" aria-label={msg("合同信息")}>
      <CopyField
        label={msg("接收角色")}
        value={item.detail.character_name}
        action={msg("复制接收角色")}
      />
      {(!packaged ||
        !!item.detail.rewards?.isk_minor) && (
        <CopyField
          key={`${item.id}:${amount}`}
          label={msg("支付金额 / ISK")}
          value={
            packaged
              ? String(Math.floor(growthISK / 100))
              : amount
          }
          action={msg("复制支付金额")}
        />
      )}
      <CopyField
        wide={!packaged}
        label={msg("合同结算 ID")}
        value={item.reference ?? `GNV-WF-${item.id}`}
        action={msg("复制合同结算 ID")}
      />
    </section>
  );
}

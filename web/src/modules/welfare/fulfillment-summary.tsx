import { msg } from "@/lib/i18n";
import { ContractHandoff } from "./contract-handoff";
import { DeliverySummary } from "./delivery-contracts";
import type { Case } from "./api";

const labels: Record<string, string> = {
  waiting_contract: "等待合同同步",
  multiple_contracts: "存在多个同编号合同，请核对",
  mismatch: "合同信息不符，请核对",
  waiting_items: "等待合同明细同步",
  issuer_unverified: "发放方权限待核对",
  contract_claimed: "合同已用于其他发放",
  contract_unavailable: "已关联合同暂不可读取",
  awaiting_acceptance: "等待接取合同",
  finished: "合同已完成",
  snapshot_required: "旧记录缺少完整奖励快照，请核对后重新申请",
  coins_review_required: "历史果壳币奖励待确认发放",
  coins_credited: "果壳币已到账",
};

export function FulfillmentSummary({
  item: v,
  user,
  manage,
}: {
  item: Case;
  user: string;
  manage: boolean;
}) {
  const d = v.detail;
  const paying = ["approved", "executing", "cancel_requested"].includes(
    v.state,
  );
  const growth = v.kind.startsWith("growth_") || v.kind.startsWith("activity_");
  const coinsOnly =
    growth &&
    d.rewards &&
    !d.rewards.fittings?.length &&
    !d.rewards.items?.length &&
    !d.rewards.isk_minor;
  const canCreate =
    paying &&
    v.state !== "cancel_requested" &&
    !d.delivery &&
    !coinsOnly &&
    d.payment_status !== "snapshot_required";
  return (
    <>
      {v.state === "cancel_requested" && d.cancellation && (
        <section className="welfare-loss-contract">
          <h3>{msg("取消原因")}</h3>
          <p className="welfare-proof">{d.cancellation.reason}</p>
          <p>{msg("取消审核期间继续核对发放合同。")}</p>
        </section>
      )}
      {(paying || d.delivery || d.payment_status === "coins_credited") && (
        <section className="welfare-loss-contract">
          <h3>{msg(coinsOnly ? "果壳币" : "合同信息")}</h3>
          {d.delivery ? (
            <DeliverySummary
              contract={d.delivery.contract}
              recipient={d.character_name}
              isk={!growth || !!d.rewards?.isk_minor}
            />
          ) : canCreate && manage && user !== v.account_id ? (
            <ContractHandoff item={v} />
          ) : null}
          <p role="status" aria-atomic="true">
            {msg(
              labels[
                d.payment_status ||
                  (coinsOnly ? "coins_review_required" : "waiting_contract")
              ] || "等待核对",
            )}
          </p>
          {canCreate && manage && user !== v.account_id && (
            <p>{msg("将结算 ID 原样填入游戏合同描述。")}</p>
          )}
        </section>
      )}
    </>
  );
}

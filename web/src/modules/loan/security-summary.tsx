import { securityMoney } from "./security-format";
import { msg } from "@/lib/i18n";
import type { SecuritySummary } from "./api";

export function SecuritySummaryView({ value }: { value: SecuritySummary }) {
  if (!value.credit_ready) return <p className="loan-dialog-note" role="status">{msg("信用评估暂不可用，请等待更新后申请保障。")}</p>;
  return <section className="loan-detail-section loan-security-summary">
    <h3>{msg("超额保障")}</h3>
    <dl className="loan-detail-facts">
      <div><dt>{msg("可用个人额度")}</dt><dd>{securityMoney(value.available_credit_minor)}</dd></div>
      <div><dt>{msg("本单所需保障")}</dt><dd>{securityMoney(value.required_minor)}</dd></div>
      <div><dt>{msg("已生效保障")}</dt><dd>{securityMoney(value.effective_minor)}</dd></div>
      <div><dt>{msg("办理中保障")}</dt><dd>{securityMoney(value.pending_minor)}</dd></div>
    </dl>
    <p className="loan-dialog-note">{value.legacy_pending ? msg("请先完成或取消已有待交付物品，再新增保障。") : value.required_minor === 0 ? msg("借款在可用个人额度内，无需担保或抵押。") : value.unallocated_minor > 0 ? msg("还需申请保障：{0}", securityMoney(value.unallocated_minor)) : value.effective_minor >= value.required_minor ? msg("所需保障已补足。") : msg("保障正在办理，确认或审核通过后才生效。")}</p>
  </section>;
}

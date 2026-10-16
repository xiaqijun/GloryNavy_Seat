import type { ReactNode } from "react";
import { getLocale, msg } from "@/lib/i18n";
import type { CaseDetail, Collateral } from "./api";
import { RepaymentSummary } from "./repayment";

const exactMoney = (minor: number) => `${new Intl.NumberFormat(getLocale(), { maximumFractionDigits: 2 }).format(minor / 100)} ISK`;
const securityState = (state: string) => ({ proposed: msg("待审核"), approved: msg("已批准"), held: msg("抵押中"), released: msg("已释放"), disputed: msg("有异议"), invited: msg("待确认"), accepted: msg("已接受"), active: msg("生效中"), rejected: msg("已拒绝"), paid: msg("已还清"), pending: msg("待还款"), partial: msg("部分已还"), overdue: msg("已逾期") }[state] ?? state);

export function CollateralItems({ items }: { items: unknown }) {
  const rows = Array.isArray(items) ? items.filter((v): v is Record<string, unknown> => !!v && typeof v === "object" && v.included !== false) : [];
  return rows.length ? <ul className="loan-collateral-items">{rows.map((item, i) => <li key={String(item.record_id ?? item.item_id ?? i)}><span>{typeof item.name === "string" && item.name || msg("物品 {0}", String(item.type_id ?? "—"))}</span><strong>× {String(item.quantity ?? "—")}</strong></li>)}</ul> : <p className="loan-detail-empty">{msg("暂无物品明细")}</p>;
}

export function LoanCaseDetail({ detail, status, collateralAction, requests }: { detail: CaseDetail; status: string; collateralAction: (item: Collateral) => ReactNode; requests: ReactNode }) {
  const item = detail.case;
  return <div className="loan-case-detail">
    <div className="loan-detail-identity"><strong>{item.public_id}</strong><span className="loan-state">{status}</span></div>
    <dl className="loan-detail-metrics"><div><dt>{msg("本金")}</dt><dd>{exactMoney(item.principal_minor)}</dd></div><div><dt>{msg("利息")}</dt><dd>{exactMoney(item.interest_minor)}</dd></div><div><dt>{msg("总应还")}{item.requested_due_at && !item.first_due_at && msg("（预计）")}</dt><dd>{exactMoney(item.total_due_minor)}</dd></div></dl>
    <section className="loan-detail-section"><h3>{msg("还款安排")}</h3><div className="loan-detail-terms"><RepaymentSummary item={item} /></div><dl className="loan-detail-facts"><div><dt>{msg("年利率")}</dt><dd>{item.interest_rate_bps == null ? "—" : `${item.interest_rate_bps / 100}%`}</dd></div><div><dt>{msg("申请时间")}</dt><dd>{new Date(item.created_at).toLocaleString(getLocale())}</dd></div></dl>
      {!!detail.installments?.length && <div className="loan-installments">{detail.installments.map(row => <div className="loan-installment" key={row.id}><div><strong>{msg("第 {0} 期", row.sequence)}</strong><span className="loan-state">{securityState(row.state)}</span></div><time dateTime={row.due_at}>{new Date(row.due_at).toLocaleString(getLocale())}</time><dl><div><dt>{msg("本期应还")}</dt><dd>{exactMoney(row.principal_minor + row.interest_minor)}</dd></div><div><dt>{msg("已还")}</dt><dd>{exactMoney(row.paid_principal_minor + row.paid_interest_minor)}</dd></div></dl></div>)}</div>}
    </section>
    <section className="loan-detail-section"><h3>{msg("物品抵押")}</h3>{requests}{detail.collateral.map(c => <article className="loan-collateral-record" key={c.id}><div className="loan-detail-identity"><strong>{msg("合同 #{0}", c.contract_id)}</strong><span className="loan-state">{securityState(c.state)}</span></div><CollateralItems items={c.items} /><dl className="loan-detail-facts"><div><dt>{msg("物品估值")}</dt><dd>{exactMoney(c.valuation_minor)}</dd></div><div><dt>{msg("抵押额度")}</dt><dd>{exactMoney(c.covered_minor)}</dd></div></dl>{collateralAction(c)}</article>)}{!detail.collateral.length && !(detail.collateral_requests?.length) && <p className="loan-detail-empty">{msg("尚未选择抵押物品")}</p>}</section>
    {!!detail.guarantees.length && <section className="loan-detail-section"><h3>{msg("他人担保")}</h3>{detail.guarantees.map(g => <div className="loan-detail-identity" key={g.id}><strong>{exactMoney(g.amount_minor)}</strong><span className="loan-state">{securityState(g.state)}</span></div>)}</section>}
  </div>;
}

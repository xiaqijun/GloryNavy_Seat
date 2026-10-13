import { Info, ShieldCheck } from "lucide-react";
import { Navigate, useSearchParams } from "react-router-dom";
import { msg } from "@/lib/i18n";
import * as api from "./api";
import "./loan.css";

const money = (v: number) => `${new Intl.NumberFormat(undefined, { maximumFractionDigits: 2 }).format(v / 100)} ISK`;

export default function LoanDashboardPage() {
  const [searchParams] = useSearchParams();
  const view = searchParams.get("view");
  return <Navigate to={`/loans/operations?view=${view === "risk" || view === "evaluations" ? view : "dashboard"}`} replace />;
}

export function LoanDashboardPanel({ context }: { context: api.Context }) {
  const { pool_summary: summary } = context;
  const canViewPool = context.administrator || context.can_manage_pool === true;
  const poolOpen = context.pools.some((item) => item.state === "open");
  return <div className="loan-dashboard">
    <section className="loan-dashboard-panel loan-pool-overview" aria-label={msg("整体借贷信息")}>
      <div className="loan-section-head"><div><h2>{msg("整体借贷信息")}</h2><p>{msg("统一贷款池的入金、放款和可用余额")}</p></div><span className="loan-state">{poolOpen ? msg("贷款池开放") : msg("统一贷款池暂不可用")}</span></div>
      {canViewPool ? <div className="loan-dashboard-metrics">
        <div><span>{msg("待核验入金")}</span><strong>{money(summary.pending_minor)}</strong></div>
        <div><span>{msg("已入池资金")}</span><strong>{money(summary.cash_minor)}</strong></div>
        <div><span>{msg("已放款本金")}</span><strong>{money(summary.funded_minor)}</strong></div>
        <div><span>{msg("已预留本金")}</span><strong>{money(summary.reserved_minor)}</strong></div>
        <div><span>{msg("可放贷余额")}</span><strong>{money(summary.available_minor)}</strong></div>
      </div> : <div className="loan-pool-private"><ShieldCheck size={18} aria-hidden="true" /><span>{msg("整体资金金额仅对贷款管理者展示，当前账号可查看贷款池状态和个人办理信息。")}</span></div>}
      {canViewPool && <div className="loan-dashboard-funding">
        <p>{msg("资金池资金来自已核验的出借入金或管理员直接登记；登记资金不需要游戏合同。")}</p>
      </div>}
    </section>
  </div>;
}

export function LoanCreditSummary({ credit }: { credit: api.Credit }) {
  const noAvailableLimit = credit.total_limit_minor <= 0;
  return <section className="loan-credit-summary" aria-label={msg("我的额度与信用")}>
    <div className="loan-section-head"><div><h2>{msg("我的额度与信用")}</h2><p>{msg("额度按统一贷款池资金、收入证据和信用评分综合计算")}</p></div>{noAvailableLimit && <span className="loan-state loan-state-warning">{msg("暂不可用")}</span>}</div>
    <div className="loan-dashboard-overview loan-credit-overview">
      <div className="loan-credit-metric loan-credit-score">
        <span>{msg("系统信用评分")}</span>
        <strong>{credit.score == null ? "—" : credit.score}<small>/100</small></strong>
      </div>
      <div className="loan-credit-metric loan-credit-limit"><span>{msg("个人额度")}</span><strong>{money(credit.total_limit_minor)}</strong><small>{msg("可申请本金上限")}</small></div>
    </div>
    {noAvailableLimit && <div className="loan-credit-explanation" role="status"><Info size={18} aria-hidden="true" /><div><strong>{msg("额度为什么是 0？")}</strong><p>{msg("个人额度会先受统一贷款池可用资金和收入测算结果限制，再按信用评分折算。当前没有形成可用额度，通常是贷款池尚无已入池资金、收入数据未达到规则且策略设为暂停、信用状态被暂停，或评分系数被设为 0；完成资金登记或钱包同步后请刷新页面。")}</p></div></div>}
  </section>;
}

import { ShieldCheck } from "lucide-react";
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
  const pool = context.pools.find((item) => item.state === "open") ?? context.pools[0];
  const poolOpen = context.pools.some((item) => item.state === "open");
  const fundingWallet = pool?.funding_wallet_owner_id
    ? context.funding_wallets?.find((wallet) => wallet.kind === pool.funding_wallet_kind && wallet.owner_id === pool.funding_wallet_owner_id && wallet.division === (pool.funding_wallet_division ?? 0))
    : undefined;
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
        <div><span>{msg("当前资金池钱包")}</span><strong>{fundingWallet?.name ?? msg("未配置")}</strong></div>
        <p>{msg("选择钱包只设置合同归属，不会自动把钱包余额计入已入池资金；完成并核验入金合同后，资金才会进入贷款流水。")}</p>
      </div>}
    </section>
  </div>;
}

export function LoanCreditSummary({ credit }: { credit: api.Credit }) {
  return <section className="loan-credit-summary" aria-label={msg("我的额度与信用")}>
    <div className="loan-section-head"><h2>{msg("我的额度与信用")}</h2></div>
    <div className="loan-dashboard-overview">
      <div className="loan-credit-metric">
        <span>{msg("系统信用评分")}</span>
        <strong>{credit.score == null ? "—" : credit.score}<small>/100</small></strong>
      </div>
      <div className="loan-credit-metric"><span>{msg("个人额度")}</span><strong>{money(credit.total_limit_minor)}</strong></div>
    </div>
  </section>;
}

import { useQuery } from "@tanstack/react-query";
import { ArrowRight, Gauge, Landmark, RefreshCw, ShieldCheck } from "lucide-react";
import { Link, Navigate, useSearchParams } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { msg } from "@/lib/i18n";
import { useSession } from "@/modules/identity";
import * as api from "./api";
import "./loan.css";

const money = (v: number) => `${new Intl.NumberFormat(undefined, { maximumFractionDigits: 2 }).format(v / 100)} ISK`;
const percent = (score: number, max: number) => Math.max(0, Math.min(100, (score / max) * 100));

export default function LoanDashboardPage() {
  const session = useSession();
  const [searchParams] = useSearchParams();
  if (session.isError) return <p role="alert">{session.error.message}</p>;
  if (!session.data) return <p role="status">{msg("正在读取")}</p>;
  if (!session.data.session) return <Navigate to="/" replace />;
  const view = searchParams.get("view");
  if (view === "risk" || view === "evaluations") return <Navigate to={`/loans/operations?view=${view}`} replace />;
  return <Dashboard userId={session.data.session.user_id} />;
}

function Dashboard({ userId }: { userId: string }) {
  const context = useQuery({
    queryKey: ["loan", "dashboard", userId],
    queryFn: ({ signal }) => api.context(signal),
    staleTime: 30_000,
    refetchOnWindowFocus: false,
  });
  const refresh = () => void context.refetch();
  if (context.isError) return <div className="loan-feedback" role="alert"><p>{context.error.message}</p><Button variant="outline" onClick={refresh}>{msg("重试")}</Button></div>;
  if (context.isPending || !context.data) return <p role="status">{msg("正在读取")}</p>;

  const { credit, pool_summary: summary } = context.data;
  const factors = [
    [msg("履约"), credit.repayment_points ?? 0, 55],
    [msg("负债压力"), credit.leverage_points ?? 0, 20],
    [msg("担保/抵押履历"), credit.security_points ?? 0, 10],
    [msg("军团活动集结率"), credit.corporation_attendance_points ?? 0, 3],
    [msg("近6个月联盟 PAP 满足"), credit.alliance_fulfillment_points ?? 0, 2],
    [msg("资产规模评分"), credit.asset_points ?? 0, 10],
  ] as const;

  return <div className="loan-page loan-dashboard">
    <header className="loan-heading">
      <div className="loan-title"><span className="loan-mark"><Landmark aria-hidden="true" /></span><div><h1>{msg("贷款业务")}</h1></div></div>
      <div className="loan-heading-actions"><Button variant="outline" disabled={context.isFetching} onClick={refresh}><RefreshCw size={16} aria-hidden="true" />{msg("刷新")}</Button><Button asChild><Link to="/loans/operations">{msg("进入贷款办理")}<ArrowRight size={16} aria-hidden="true" /></Link></Button></div>
    </header>

    <section className="loan-dashboard-metrics" aria-label={msg("统一贷款池")}>
      <div><span>{msg("待核验入金")}</span><strong>{money(summary.pending_minor)}</strong></div>
      <div><span>{msg("已入池资金")}</span><strong>{money(summary.cash_minor)}</strong></div>
      <div><span>{msg("可放贷余额")}</span><strong>{money(summary.available_minor)}</strong></div>
    </section>

    <section className="loan-dashboard-overview" aria-label={msg("我的额度与信用")}>
      <div className="loan-dashboard-score">
        <div className="loan-dashboard-score-icon"><Gauge size={20} aria-hidden="true" /></div>
        <span>{msg("系统信用评分")}</span>
        <strong>{credit.score == null ? "—" : credit.score}<small>/100</small></strong>
        <span className="loan-state">{credit.state}</span>
      </div>
      <div className="loan-dashboard-limits">
        <div><span>{msg("系统总额度")}</span><strong>{money(credit.total_limit_minor)}</strong></div>
        <div><span>{msg("系统无担保额度")}</span><strong>{money(credit.unsecured_limit_minor)}</strong></div>
        <div><span>{msg("担保单笔上限")}</span><strong>{credit.guarantee_limit_minor == null ? "—" : money(credit.guarantee_limit_minor)}</strong></div>
      </div>
    </section>

    <section className="loan-dashboard-panel loan-dashboard-factors" aria-label={msg("评分因子")}>
      <div className="loan-section-head"><h2>{msg("评分因子")}</h2></div>
      <div className="loan-factor-grid">{factors.map(([label, score, max]) => <div className="loan-factor" key={label} aria-label={`${label}: ${score}/${max}`}><div><span>{label}</span><strong>{score}/{max}</strong></div><div className="loan-factor-track" aria-hidden="true"><span style={{ width: `${percent(score, max)}%` }} /></div></div>)}</div>
    </section>

    {credit.state !== "active" && <div className="loan-notice" role="status"><ShieldCheck size={18} aria-hidden="true" /><span>{msg("系统信用评估尚未通过，审批前需要完成履约评估。")}</span></div>}
  </div>;
}

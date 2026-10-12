import { useQuery } from "@tanstack/react-query";
import { ArrowRight, Landmark, RefreshCw, ShieldCheck } from "lucide-react";
import { Link, Navigate, useSearchParams } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { msg } from "@/lib/i18n";
import { useSession } from "@/modules/identity";
import * as api from "./api";
import "./loan.css";

const money = (v: number) => `${new Intl.NumberFormat(undefined, { maximumFractionDigits: 2 }).format(v / 100)} ISK`;

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
  return <div className="loan-page">
    <header className="loan-heading">
      <div className="loan-title"><span className="loan-mark"><Landmark aria-hidden="true" /></span><div><h1>{msg("贷款业务")}</h1><p>{msg("查看个人信用、额度和统一贷款池概况")}</p></div></div>
      <div className="loan-heading-actions"><Button variant="outline" disabled={context.isFetching} onClick={refresh}><RefreshCw size={16} aria-hidden="true" />{msg("刷新")}</Button><Button asChild><Link to="/loans/operations">{msg("进入贷款办理")}<ArrowRight size={16} aria-hidden="true" /></Link></Button></div>
    </header>
    <section className="loan-dashboard-metrics" aria-label={msg("统一贷款池")}>
      <div><span>{msg("待核验入金")}</span><strong>{money(summary.pending_minor)}</strong></div>
      <div><span>{msg("已入池资金")}</span><strong>{money(summary.cash_minor)}</strong></div>
      <div><span>{msg("可放贷余额")}</span><strong>{money(summary.available_minor)}</strong></div>
    </section>
    <section className="loan-dashboard-panel">
      <div className="loan-section-head"><div><h2>{msg("我的额度与信用")}</h2><p>{msg("额度由系统根据履约、负债和可用信用资料自动评估。")}</p></div><span className="loan-state">{credit.state}</span></div>
      <section className="loan-credit loan-dashboard-credit" aria-label={msg("我的额度与信用")}>
        <div><span>{msg("系统信用评分")}</span><strong>{credit.score ?? "—"}/100</strong><small>{msg("系统自动评估")}</small></div>
        <div><span>{msg("系统总额度")}</span><strong>{money(credit.total_limit_minor)}</strong><small>{msg("按评估结果动态计算")}</small></div>
        <div><span>{msg("系统无担保额度")}</span><strong>{money(credit.unsecured_limit_minor)}</strong><small>{msg("按评估结果动态计算")}</small></div>
        <div><span>{msg("担保单笔上限")}</span><strong>{credit.guarantee_limit_minor == null ? "—" : money(credit.guarantee_limit_minor)}</strong><small>{msg("按风控规则计算")}</small></div>
      </section>
      <section className="loan-credit-factors" aria-label={msg("评分因子")}><div><span>{msg("履约")}</span><strong>{credit.repayment_points ?? 0}/55</strong></div><div><span>{msg("负债压力")}</span><strong>{credit.leverage_points ?? 0}/20</strong></div><div><span>{msg("担保/抵押履历")}</span><strong>{credit.security_points ?? 0}/10</strong></div><div><span>{msg("军团活动集结率")}</span><strong>{credit.corporation_attendance_points ?? 0}/3</strong><small>{(credit.corporation_eligible_events ?? 0) > 0 ? `${Math.round((credit.corporation_attendance_bps ?? 0) / 100)}% · ${credit.corporation_attended_events ?? 0}/${credit.corporation_eligible_events}` : msg("暂无有效活动数据")}</small></div><div><span>{msg("近6个月联盟 PAP 满足")}</span><strong>{credit.alliance_fulfillment_points ?? 0}/2</strong><small>{(credit.alliance_eligible_months ?? 0) > 0 ? `${Math.round((credit.alliance_fulfillment_bps ?? 0) / 100)}% · ${credit.alliance_fulfilled_months ?? 0}/${credit.alliance_eligible_months}` : msg("暂无完整联盟快照")}</small></div><div><span>{msg("资产规模评分")}</span><strong>0/10</strong><small>{msg("资产数据源尚未接入")}</small></div></section>
      <p className="loan-credit-factor-note">{msg("PAP 总分 5 分由两项组成：军团活动集结率 3 分，近 6 个月联盟 PAP 满足情况 2 分；缺失或未完成的数据保持中性。资产规模评分当前固定为 0 分。")}</p>
    </section>
    {credit.state !== "active" && <div className="loan-notice" role="status"><ShieldCheck size={18} aria-hidden="true" /><span>{msg("系统信用评估尚未通过，审批前需要完成履约评估。")}</span></div>}
    <section className="loan-dashboard-panel loan-dashboard-note"><div className="loan-section-head"><div><h2>{msg("贷款办理")}</h2><p>{msg("申请贷款、提交出借额度、处理担保邀请和登记抵押请进入贷款办理页面。")}</p></div><Button variant="outline" asChild><Link to="/loans/operations">{msg("进入贷款办理")}<ArrowRight size={16} aria-hidden="true" /></Link></Button></div><p>{msg("评估记录和风控设置也在贷款办理页面中按权限提供。")}</p></section>
  </div>;
}

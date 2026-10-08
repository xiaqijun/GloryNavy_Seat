import { useQuery } from "@tanstack/react-query";
import { Link, Navigate } from "react-router-dom";
import {
  Activity,
  ArrowUpRight,
  Award,
  ChartNoAxesCombined,
  ClipboardCheck,
  Coins,
  Landmark,
  ShieldCheck,
  Timer,
  RefreshCw,
  UsersRound,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { useState } from "react";
import TrendChart from "@/components/charts/trend-chart";
import { Card } from "@/components/ui/card";
import { IconAction } from "@/components/ui/icon-action";
import { Select } from "@/components/ui/select";
import { msg, getLocale } from "@/lib/i18n";
import { useManagementAccess } from "@/modules/access";
import { getContext, getEvents, getOnline, type Event } from "@/modules/attendance/api";
import { getPAP } from "@/modules/attendance/pap-api";
import { getAlliancePAPFulfillment } from "@/modules/attendance/alliance-pap-api";
import { getQueue, type Queue } from "@/modules/approval/api";
import { getModuleCatalog } from "@/app/catalog";
import { getPublicActivity } from "./public-activity-api";
import { getPublicCorporation } from "./public-corporation-api";
import * as walletApi from "@/modules/wallet/api";
import * as welfareApi from "@/modules/welfare/api";
import "./operations.css";

const date = (value: string) =>
  new Date(value).toLocaleString(getLocale(), {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });

const number = (value: number) =>
  value.toLocaleString(getLocale(), { maximumFractionDigits: 1 });

function compactISK(value: string | number | null | undefined, minor = false) {
  const amount = typeof value === "number" ? value : Number(value);
  if (!Number.isFinite(amount)) return "—";
  const isk = minor ? amount / 100 : amount;
  const abs = Math.abs(isk);
  const unit = abs >= 1_000_000_000 ? "B" : abs >= 1_000_000 ? "M" : abs >= 1_000 ? "K" : "ISK";
  const divisor = unit === "B" ? 1_000_000_000 : unit === "M" ? 1_000_000 : unit === "K" ? 1_000 : 1;
  const rounded = Math.round(isk / divisor);
  return `${rounded.toLocaleString(getLocale())} ${unit === "ISK" ? "ISK" : `${unit} ISK`}`;
}

function monthStart() {
  const value = new Date();
  value.setHours(0, 0, 0, 0);
  value.setDate(1);
  return value;
}

function sixMonthStart() {
  const value = monthStart();
  value.setMonth(value.getMonth() - 5);
  return value;
}

type TrendPoint = { label: string; value: number };

function monthTrend<T>(items: T[], at: (item: T) => string, value: (item: T) => number) {
  const start = monthStart().getTime();
  const points = Array.from({ length: 5 }, (_, index) => ({ label: `${index + 1}`, value: 0 }));
  for (const item of items) {
    const stamp = Date.parse(at(item));
    if (!Number.isFinite(stamp) || stamp < start) continue;
    const index = Math.min(points.length - 1, Math.floor((stamp - start) / (7 * 24 * 60 * 60 * 1000)));
    points[index].value += Math.max(0, value(item));
  }
  return points;
}

async function currentMonthEvents(signal: AbortSignal): Promise<Event[]> {
  const start = monthStart().getTime();
  const items: Event[] = [];
  let before = "";
  for (let page = 0; page < 200; page += 1) {
    const result = await getEvents(before, signal);
    items.push(...result.events);
    const oldest = result.events.at(-1);
    if (!result.next_cursor || (oldest && Date.parse(oldest.starts_at) < start)) break;
    before = result.next_cursor;
  }
  return items.filter((event) => Date.parse(event.starts_at) >= start);
}

async function currentMonthWelfareCases(corp: string, signal: AbortSignal) {
  const start = monthStart().getTime();
  const items: welfareApi.Case[] = [];
  let before = "";
  for (let page = 0; page < 200; page += 1) {
    const result = await welfareApi.list(corp, true, "", before, signal);
    items.push(...result.items.filter((item) => Date.parse(item.created_at) >= start));
    const oldest = result.items.at(-1);
    if (!result.next_cursor || (oldest && Date.parse(oldest.created_at) < start)) break;
    before = result.next_cursor;
  }
  return items;
}

function Feedback({ error, retry }: { error?: Error; retry?: () => void }) {
  return (
    <div className="operations-feedback" role={error ? "alert" : "status"}>
      <Activity size={18} aria-hidden="true" />
      <span>{error?.message ?? msg("正在读取")}</span>
      {retry && (
        <IconAction label={msg("重试")} onClick={retry}>
          <RefreshCw />
        </IconAction>
      )}
    </div>
  );
}

function Metric({
  icon: Icon,
  label,
  value,
  hint,
  to,
}: {
  icon: LucideIcon;
  label: string;
  value: string;
  hint?: string;
  to?: string;
}) {
  const content = (
    <>
      <div className="operations-metric-label">
        <span className="operations-icon-box">
          <Icon size={18} aria-hidden="true" />
        </span>
        <span>{label}</span>
      </div>
      <strong>{value}</strong>
      {hint && <small>{hint}</small>}
      {to && <ArrowUpRight className="operations-metric-arrow" size={16} aria-hidden="true" />}
    </>
  );
  return to ? (
    <Link className="operations-metric" to={to}>
      {content}
    </Link>
  ) : (
    <div className="operations-metric">{content}</div>
  );
}

function StateBadge({ state }: { state: string }) {
  const label =
    state === "open"
      ? msg("进行中")
      : state === "closed"
        ? msg("已结束")
        : state;
  return <span className={`operations-badge ${state === "open" ? "is-live" : ""}`}>{label}</span>;
}

function TrendLine({ label, points, suffix = "" }: { label: string; points: TrendPoint[]; suffix?: string }) {
  return <div className="operations-trend">
    <span className="operations-trend-label">{label}</span>
    <TrendChart label={label} points={points} unit={suffix.trim()} />
  </div>;
}
function incomeTrendPoints(items: walletApi.IncomeTrendPoint[]) {
  const start = sixMonthStart();
  const byMonth = new Map(items.map((item) => [item.period, item]));
  return Array.from({ length: 6 }, (_, index) => {
    const value = new Date(start);
    value.setMonth(start.getMonth() + index);
    const period = `${value.getFullYear()}-${String(value.getMonth() + 1).padStart(2, "0")}`;
    const item = byMonth.get(period);
    const rawIncome = item ? Number(item.income) : 0;
    return {
      label: period.slice(5),
      value: Number.isFinite(rawIncome) ? Math.max(0, rawIncome / 1_000_000) : 0,
      income: item?.income ?? "0",
      activeMembers: item?.active_members ?? 0,
    };
  });
}

function financeTrendPoints(items: walletApi.FinanceTrendPoint[], field: "net" | "tax") {
  const start = sixMonthStart();
  const byMonth = new Map(items.map((item) => [item.period, item]));
  return Array.from({ length: 6 }, (_, index) => {
    const value = new Date(start);
    value.setMonth(start.getMonth() + index);
    const period = `${value.getFullYear()}-${String(value.getMonth() + 1).padStart(2, "0")}`;
    const item = byMonth.get(period);
    const rawValue = item ? Number(item[field]) : 0;
    return {
      label: period.slice(5),
      value: Number.isFinite(rawValue) ? rawValue / 1_000_000 : 0,
    };
  });
}

export default function OperationsPage() {
  const { session, access } = useManagementAccess();
  if (session.isSuccess && !session.data.session) return <Navigate to="/" replace />;
  if (session.isError || access.isError)
    return <Feedback error={session.error ?? access.error ?? undefined} />;
  if (!access.isSuccess || !session.data?.session) return <Feedback />;
  if (!access.data.administrator && !access.data.can_manage)
    return <Feedback error={new Error(msg("没有访问权限"))} />;
  return <OperationsWorkspace user={session.data.session.user_id} />;
}

function OperationsWorkspace({ user }: { user: string }) {
  const [selectedCorp, setSelectedCorp] = useState("");
  const catalog = useQuery({
    queryKey: ["host", "modules"],
    queryFn: ({ signal }) => getModuleCatalog(signal),
    staleTime: 60_000,
  });
  const modules = new Set(catalog.data?.map((module) => module.id));
  const context = useQuery({
    queryKey: ["attendance", "context", user],
    queryFn: ({ signal }) => getContext(signal),
    enabled: modules.has("attendance"),
    staleTime: 30_000,
  });
  const corp = context.data?.corporations.find((item) => item.id === selectedCorp) ?? context.data?.corporations[0];
  const corpId = corp?.id ?? "";
  const pap = useQuery({
    queryKey: ["operations", "pap", user, corpId],
    queryFn: ({ signal }) => getPAP(corpId, "month", 0, signal),
    enabled: !!corpId && modules.has("attendance"),
    staleTime: 30_000,
    refetchInterval: 60_000,
  });
  const online = useQuery({
    queryKey: ["operations", "online", user, corpId],
    queryFn: ({ signal }) => getOnline(corpId, "7", "", signal),
    enabled: !!corpId && modules.has("attendance"),
    staleTime: 30_000,
    refetchInterval: 60_000,
  });
  const allianceFulfillment = useQuery({
    queryKey: ["operations", "alliance-pap-fulfillment", user],
    queryFn: ({ signal }) => getAlliancePAPFulfillment(signal),
    enabled: modules.has("attendance"),
    staleTime: 60_000,
    refetchInterval: 60_000,
  });
  const events = useQuery({
    queryKey: ["operations", "events", user],
    queryFn: ({ signal }) => currentMonthEvents(signal),
    enabled: !!corpId && modules.has("attendance"),
    staleTime: 30_000,
    refetchInterval: 60_000,
  });
  const approval = useQuery({
    queryKey: ["operations", "approval", user],
    queryFn: ({ signal }) => getQueue(new URLSearchParams({ view: "pending" }), signal),
    enabled: modules.has("approval"),
    staleTime: 30_000,
    refetchInterval: 60_000,
  });
  const publicCorp = useQuery({
    queryKey: ["eve", "public", "corporation"],
    queryFn: ({ signal }) => getPublicCorporation(signal),
    staleTime: 300_000,
  });
  const publicActivity = useQuery({
    queryKey: ["eve", "public", "activity"],
    queryFn: ({ signal }) => getPublicActivity(signal),
    staleTime: 300_000,
  });
  const financeFrom = new Date();
  financeFrom.setHours(0, 0, 0, 0);
  financeFrom.setDate(1);
  const finance = useQuery({
    queryKey: ["operations", "finance", user, corpId, financeFrom.toISOString().slice(0, 10)],
    queryFn: ({ signal }) => walletApi.corporationSummary(corpId, financeFrom.toISOString(), signal),
    enabled: !!corpId && modules.has("wallet"),
    staleTime: 60_000,
    refetchInterval: 60_000,
  });
  const financeTrendFrom = sixMonthStart();
  const financeTrend = useQuery({
    queryKey: ["operations", "finance-trend", user, corpId, financeTrendFrom.toISOString().slice(0, 10)],
    queryFn: ({ signal }) => walletApi.corporationFinanceTrend(corpId, financeTrendFrom.toISOString(), new Date().toISOString(), signal),
    enabled: !!corpId && modules.has("wallet"),
    staleTime: 60_000,
    refetchInterval: 60_000,
  });
  const incomeTrendFrom = sixMonthStart();
  const incomeTrend = useQuery({
    queryKey: ["operations", "personal-income-trend", user, corpId, incomeTrendFrom.toISOString().slice(0, 10)],
    queryFn: ({ signal }) => walletApi.corporationPersonalIncomeTrend(corpId, incomeTrendFrom.toISOString(), new Date().toISOString(), signal),
    enabled: !!corpId && modules.has("wallet"),
    staleTime: 60_000,
    refetchInterval: 60_000,
  });
  const welfareContext = useQuery({
    queryKey: ["operations", "welfare-context", user, corpId],
    queryFn: ({ signal }) => welfareApi.context(corpId, signal),
    enabled: !!corpId && modules.has("welfare"),
    staleTime: 60_000,
  });
  const welfareCases = useQuery({
    queryKey: ["operations", "welfare-cases", user, corpId, financeFrom.toISOString().slice(0, 7)],
    queryFn: ({ signal }) => currentMonthWelfareCases(corpId, signal),
    enabled: !!corpId && modules.has("welfare"),
    staleTime: 60_000,
    refetchInterval: 60_000,
  });
  const corpOptions = (context.data?.corporations ?? []).map((item) => ({
    value: item.id,
    label: item.name,
  }));
  const selectedCorpId = corpId;
  const papValue = pap.data?.points;
  const onlineValue = online.data?.members.length;
  const queue = approval.data;
  const isPublicCorp = corpId === String(publicCorp.data?.corporation_id ?? "");
  const papEvents = (events.data ?? []).filter((event) => !corpId || event.corporation_id === corpId);
  const recentEvents = papEvents.slice(0, 6);
  const papIssued = papEvents.filter((event) => event.pap_issued).length;
  const papPending = papEvents.filter((event) => event.state === "closed" && !event.pap_issued).length;
  const lossCases = welfareCases.data ?? [];
  const lossReserved = lossCases
    .filter((item) => ["submitted", "information", "approved", "executing", "cancel_requested"].includes(item.state))
    .reduce((sum, item) => sum + item.award_minor, 0);
  const lossCompleted = lossCases
    .filter((item) => item.state === "completed")
    .reduce((sum, item) => sum + item.award_minor, 0);
  const papTrend = monthTrend(papEvents, (event) => event.starts_at, (event) => event.pap_points ?? 1);
  const lossTrend = monthTrend(lossCases, (item) => item.created_at, (item) => item.award_minor / 100);
  const refresh = () => {
    void Promise.all([
      context.refetch(),
      pap.refetch(),
      allianceFulfillment.refetch(),
      online.refetch(),
      events.refetch(),
      approval.refetch(),
      finance.refetch(),
      financeTrend.refetch(),
      incomeTrend.refetch(),
      welfareContext.refetch(),
      welfareCases.refetch(),
      publicCorp.refetch(),
      publicActivity.refetch(),
    ]);
  };
  return (
    <div className="operations-page">
      <header className="operations-heading">
        <div className="operations-title">
          <span className="operations-mark"><ChartNoAxesCombined aria-hidden="true" /></span>
          <div>
            <h1>{msg("军团运营")}</h1>
            <p>{corp?.name ?? msg("军团数据")}</p>
          </div>
        </div>
        <div className="operations-actions">
          {corpOptions.length > 1 && (
            <Select label={msg("选择军团")} value={selectedCorpId} onValueChange={setSelectedCorp} options={corpOptions} />
          )}
          <IconAction label={msg("刷新运营数据")} disabled={context.isFetching} onClick={refresh}>
            <RefreshCw className={context.isFetching ? "is-spinning" : ""} />
          </IconAction>
        </div>
      </header>
      {context.isError ? <Feedback error={context.error} retry={() => void context.refetch()} /> : !context.data ? <Feedback /> : null}
      <section className="operations-metrics" aria-label={msg("运营指标")}>
        <Metric icon={UsersRound} label={msg("军团成员")} value={isPublicCorp && publicCorp.data ? number(publicCorp.data.member_count) : "—"} hint={isPublicCorp ? msg("公开快照") : msg("暂无公开快照")} />
        <Metric icon={Award} label={msg("本月军团 PAP")} value={papValue == null ? "—" : number(papValue)} hint={pap.data ? msg("{0} 场活动 · {1} 条记录", pap.data.events, pap.data.participations) : msg("等待同步")} to="/attendance?view=pap" />
        <Metric icon={Activity} label={msg("近 7 日在线成员")} value={onlineValue == null ? "—" : number(onlineValue)} hint={online.data ? msg("在线时长 {0}", hours(online.data.seconds)) : msg("等待采样")} to="/attendance?view=online" />
        <Metric icon={ClipboardCheck} label={msg("待审批")} value={queueCount(queue, "pending")} hint={queue?.unavailable.length ? msg("部分来源不可用") : msg("管理员待办")} to="/approvals?view=pending" />
      </section>
      <div className="operations-grid">
        <Card className="operations-panel operations-queue">
          <header><h2>{msg("待办分布")}</h2><Link to="/approvals" className="operations-link">{msg("查看审批中心")} <ArrowUpRight size={15} aria-hidden="true" /></Link></header>
          <QueueSummary queue={queue} />
        </Card>
        <Card className="operations-panel operations-finance">
          <header><h2>{msg("财务情况")}</h2><Link to="/wallet" className="operations-link">{msg("查看钱包")} <ArrowUpRight size={15} aria-hidden="true" /></Link></header>
          {!modules.has("wallet") || finance.isError || financeTrend.isError ? <Feedback error={finance.error ?? financeTrend.error ?? undefined} retry={() => void Promise.all([finance.refetch(), financeTrend.refetch()])} /> : !finance.data || !financeTrend.data ? <Feedback /> : <FinanceSummary corporation={finance.data.items[0]} trend={financeTrend.data.items} />}
        </Card>
        <Card className="operations-panel operations-member-income">
          <header><h2>{msg("成员收入变化")}</h2><Link to="/wallet" className="operations-link">{msg("查看钱包")} <ArrowUpRight size={15} aria-hidden="true" /></Link></header>
          {!modules.has("wallet") || incomeTrend.isError ? <Feedback error={incomeTrend.error ?? undefined} retry={() => void incomeTrend.refetch()} /> : !incomeTrend.data ? <Feedback /> : <MemberIncomeTrend points={incomeTrendPoints(incomeTrend.data.items)} />}
        </Card>
        <Card className="operations-panel operations-pap">
          <header><h2>{msg("集结情况 · PAP结算")}</h2><Link to="/attendance?view=pap" className="operations-link">{msg("查看考勤")} <ArrowUpRight size={15} aria-hidden="true" /></Link></header>
          <div className="operations-summary-grid">
            <SummaryValue icon={Award} label={msg("本月 PAP")} value={papValue == null ? "—" : number(papValue)} />
            <SummaryValue icon={Activity} label={msg("活动场次")} value={number(papEvents.length)} />
            <SummaryValue icon={ShieldCheck} label={msg("已结算")} value={number(papIssued)} />
            <SummaryValue icon={Timer} label={msg("待结算")} value={number(papPending)} />
          </div>
          <AllianceFulfillmentSummary data={allianceFulfillment.data} />
          <TrendLine label={msg("本月 PAP 趋势")} points={papTrend} suffix=" PAP" />
          <p className="operations-public-note">{pap.data ? msg("本月集结分已按活动结算状态统计") : msg("等待 PAP 同步")}</p>
        </Card>
        <Card className="operations-panel operations-loss-funds">
          <header><h2>{msg("补损资金")}</h2><Link to="/welfare" className="operations-link">{msg("查看补损")} <ArrowUpRight size={15} aria-hidden="true" /></Link></header>
          {!modules.has("welfare") || welfareCases.isError ? <Feedback error={welfareCases.error ?? undefined} retry={() => void welfareCases.refetch()} /> : !welfareCases.data || welfareContext.isFetching ? <Feedback /> : <div className="operations-summary-grid operations-summary-grid-compact">
            <SummaryValue icon={Coins} label={msg("待结算占用")} value={compactISK(lossReserved, true)} />
            <SummaryValue icon={ShieldCheck} label={msg("已完成发放")} value={compactISK(lossCompleted, true)} />
            <SummaryValue icon={Landmark} label={msg("本月案件")} value={number(lossCases.length)} />
          </div>}
          <TrendLine label={msg("本月补损趋势")} points={lossTrend} suffix=" ISK" />
          <p className="operations-public-note">{welfareContext.data?.loss_quotas?.length ? msg("按本月补损案件与当前规则统计") : msg("补损额度未配置")}</p>
        </Card>
        <Card className="operations-panel">
          <header><h2>{msg("近期活动")}</h2><Link to="/attendance" className="operations-link">{msg("查看考勤")} <ArrowUpRight size={15} aria-hidden="true" /></Link></header>
          {!events.data ? <Feedback error={events.error ?? undefined} retry={() => void events.refetch()} /> : recentEvents.length === 0 ? <p className="operations-empty">{msg("暂无活动记录")}</p> : <div className="operations-event-list">{recentEvents.map((event) => <Link to={`/attendance?event=${event.id}`} key={event.id}><span className="operations-event-copy"><strong>{event.title}</strong><small>{date(event.starts_at)}</small></span><span className="operations-event-meta"><StateBadge state={event.state} /><span>{event.participants} {msg("人")}</span></span><ArrowUpRight size={16} aria-hidden="true" /></Link>)}</div>}
        </Card>
        <Card className="operations-panel operations-public">
          <header><h2>{msg("公开运营")}</h2><Link to="/#strength" className="operations-link">{msg("查看公开资料")} <ArrowUpRight size={15} aria-hidden="true" /></Link></header>
          <div className="operations-public-grid">
            <div><span>{msg("本月击毁")}</span><strong>{isPublicCorp && publicActivity.data?.combat?.months.at(-1)?.kills != null ? number(publicActivity.data.combat.months.at(-1)!.kills!) : "—"}</strong></div>
            <div><span>{msg("击毁价值")}</span><strong>{isPublicCorp && publicActivity.data?.combat?.months.at(-1)?.value != null ? compactISK(publicActivity.data.combat.months.at(-1)!.value!) : "—"}</strong></div>
            <div><span>{msg("在线采样覆盖")}</span><strong>{isPublicCorp && publicActivity.data?.online ? `${number(publicActivity.data.online.covered_characters)}/${number(publicActivity.data.online.bound_characters)}` : "—"}</strong></div>
          </div>
          <p className="operations-public-note">{isPublicCorp && publicCorp.data ? msg("公开数据只展示数字，不替代管理明细") : msg("当前军团没有可用公开快照")}</p>
        </Card>
      </div>
      {isPublicCorp && publicCorp.data && <p className="operations-updated">{msg("公开军团数据更新于 {0}", date(publicCorp.data.updated_at))}</p>}
    </div>
  );
}

function AllianceFulfillmentSummary({ data }: { data?: import("@/modules/attendance/alliance-pap-api").AlliancePAPFulfillment }) {
  const hasMembers = data?.available && data.eligible_accounts > 0;
  const rate = hasMembers ? `${Math.round(data.rate_bps / 100)}%` : "—";
  return <div className="operations-alliance-fulfillment">
    <div>
      <span>{msg("联盟集结满足率")}</span>
      <strong>{rate}</strong>
    </div>
    <small>{hasMembers ? msg("{0}/{1} 人达标 · 目标 {2} PAP/人", data.achieved_accounts, data.eligible_accounts, data.target) : msg("联盟 PAP 数据暂不可用")}</small>
  </div>;
}

function FinanceSummary({ corporation, trend }: { corporation?: walletApi.Summary["items"][number]; trend: walletApi.FinanceTrendPoint[] }) {
  if (!corporation) return <p className="operations-empty">{msg("暂无财务快照")}</p>;
  const observed = corporation.observed_at;
  const total = corporation.balance;
  const currentPeriod = `${new Date().getUTCFullYear()}-${String(new Date().getUTCMonth() + 1).padStart(2, "0")}`;
  const current = trend.find((item) => item.period === currentPeriod);
  return <>
    <div className="operations-finance-highlights">
      <div className="operations-finance-total">
        <span className="operations-report-kicker"><Coins size={15} aria-hidden="true" />{msg("总余额")}</span>
        <strong>{compactISK(total)}</strong>
        <small>{msg("军团钱包")}</small>
      </div>
      <div className="operations-finance-tax">
        <span className="operations-report-kicker">{msg("税收")}</span>
        <strong>{compactISK(current?.tax)}</strong>
        <small>{msg("本月")}</small>
      </div>
      <div className="operations-finance-observed">
        <span>{observed ? msg("数据截至 {0}", date(observed)) : msg("等待钱包同步")}</span>
      </div>
    </div>
    <div className="operations-finance-charts">
      <div className="operations-finance-chart">
        <div className="operations-trend-heading"><span className="operations-trend-label">{msg("钱包趋势")}</span><span>{msg("近六个月")}</span></div>
        <TrendChart label={msg("钱包趋势")} points={financeTrendPoints(trend, "net")} unit="M ISK" />
      </div>
      <div className="operations-finance-chart">
        <div className="operations-trend-heading"><span className="operations-trend-label">{msg("税收趋势")}</span><span>{msg("近六个月")}</span></div>
        <TrendChart label={msg("税收趋势")} points={financeTrendPoints(trend, "tax")} unit="M ISK" />
      </div>
    </div>
  </>;
}

function MemberIncomeTrend({ points }: { points: { label: string; value: number; income: string; activeMembers: number }[] }) {
  const total = points.reduce((sum, point) => sum + point.value, 0);
  const current = points.at(-1);
  return <>
    <div className="operations-summary-grid operations-summary-grid-compact">
      <SummaryValue icon={Coins} label={msg("近六个月收入")} value={compactISK(total * 1_000_000)} />
      <SummaryValue icon={Activity} label={msg("本月成员收入")} value={current ? compactISK(current.income) : "—"} />
      <SummaryValue icon={UsersRound} label={msg("本月有收入成员")} value={current ? number(current.activeMembers) : "—"} />
    </div>
    <div className="operations-income-chart">
      <TrendChart label={msg("成员收入六个月趋势")} points={points} unit="M ISK" />
    </div>
  </>;
}

function SummaryValue({ icon: Icon, label, value }: { icon: LucideIcon; label: string; value: string }) {
  return <div className="operations-summary-value"><span><Icon size={16} aria-hidden="true" />{label}</span><strong>{value}</strong></div>;
}

function QueueSummary({ queue }: { queue?: Queue }) {
  const rows = [
    ["pending", msg("待审批")],
    ["fulfillment", msg("待发放")],
    ["exceptions", msg("异常")],
    ["information", msg("待补充")],
  ] as const;
  return <div className="operations-queue-summary">{rows.map(([id, label]) => { const value = queueCount(queue, id); return <div key={id}><span><i className={`queue-dot queue-${id}`} />{label}</span><strong>{value}</strong><div className="queue-track"><span style={{ width: queue ? `${Math.min(100, (Number(value === "—" ? 0 : value) / Math.max(1, ...Object.values(queue.counts))) * 100)}%` : "0%" }} /></div></div>; })}</div>;
}

function queueCount(queue: Queue | undefined, id: string) {
  if (!queue || queue.unavailable.length > 0) return "—";
  const value = queue.counts[id];
  return value == null ? "—" : number(value);
}

function hours(seconds: number) {
  return `${(seconds / 3600).toLocaleString(getLocale(), { maximumFractionDigits: 1 })} h`;
}

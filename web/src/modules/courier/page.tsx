import { useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Navigate } from "react-router-dom";
import { Check, Copy, ExternalLink, RefreshCw, Truck } from "lucide-react";
import { useSession } from "@/modules/identity";
import { msg, getLocale } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { useToast } from "@/components/ui/toast-context";
import { IconAction } from "@/components/ui/icon-action";
import { Select } from "@/components/ui/select";
import * as api from "./api";
import "./courier.css";

const format = (value: number) => value.toLocaleString(getLocale(), { maximumFractionDigits: 2 });
const money = (value: number) => `${value < 1e6 ? format(value) : new Intl.NumberFormat("en-US", { notation: "compact", maximumFractionDigits: 2 }).format(value)} ISK`;

export default function CourierPage() {
  const s = useSession();
  if (s.isError) return <p role="alert">{s.error.message}</p>;
  if (!s.data) return <p role="status">{msg("正在读取")}</p>;
  if (!s.data.session) return <Navigate to="/" replace />;
  return <Workspace key={s.data.session.user_id} user={s.data.session.user_id} csrf={s.data.session.csrf_token} />;
}

function Workspace({ user, csrf }: { user: string; csrf: string }) {
  const toast = useToast();
  const [mode, setMode] = useState<"paste" | "manual">("paste");
  const [text, setText] = useState("");
  const [origin, setOrigin] = useState("");
  const [routeID, setRouteID] = useState("");
  const [volume, setVolume] = useState("0");
  const [collateral, setCollateral] = useState("0");
  const [copied, setCopied] = useState("");
  const [copyError, setCopyError] = useState("");
  const q = useQuery({ queryKey: ["courier", "routes", user], queryFn: ({ signal }) => api.routes(signal), refetchInterval: 300000, retry: 1 });
  const estimate = useMutation({ mutationFn: () => api.appraise(csrf, text) });
  const available = q.data?.routes.filter(r => !r.paused) || [];
  const categories = q.data?.location_categories || {};
  const locationCompare = (a: string, b: string) => (categories[a]?.category_priority ?? 999999) - (categories[b]?.category_priority ?? 999999) || a.localeCompare(b);
  const origins = [...new Set(available.map(r => r.start))].sort(locationCompare);
  const destinations = available.filter(r => r.start === origin).sort((a, b) => locationCompare(a.end, b.end) || a.transport_type.localeCompare(b.transport_type));
  const route = destinations.find(r => String(r.id) === routeID);
  const appraisal = mode === "paste" ? estimate.data?.appraisal : undefined;
  const failed = Object.entries(appraisal?.failed_items || {});
  const totalVolume = api.inputNumber(volume) + (appraisal?.volume || 0);
  const totalCollateral = api.inputNumber(collateral) + (appraisal?.sell_price || 0);
  const result = route ? api.quote(route, totalVolume, totalCollateral) : null;
  const ready = !!result && totalVolume > 0 && !result.oversized && !failed.length && (mode === "manual" || (!!appraisal && !estimate.isError)) && !estimate.isPending && !q.isError && !q.isFetching;
  const resetCopy = () => { setCopied(""); setCopyError(""); };
  async function copy(label: string, value: string) {
    try { await navigator.clipboard.writeText(value); setCopied(`${label}:${value}`); setCopyError(""); toast.success(msg("已复制")); }
    catch { setCopyError(msg("复制失败，请手动复制")); }
  }
  const field = (label: string, value: string) => <div className="courier-copy" key={label}><label><span>{label}</span><input aria-label={label} value={value} readOnly onFocus={e => e.target.select()} /></label><IconAction label={copied === `${label}:${value}` ? msg("已复制") : msg("复制 {0}", label)} disabled={!ready} onClick={() => void copy(label, value)}>{copied === `${label}:${value}` ? <Check aria-hidden="true" /> : <Copy aria-hidden="true" />}</IconAction></div>;
  return <div className="courier-page">
    <header className="courier-heading"><Truck aria-hidden="true" /><h1>{msg("快递服务")}</h1><a href="https://hallogistics.space/" target="_blank" rel="noreferrer">HA Logistics <ExternalLink size={16} aria-hidden="true" /></a><IconAction label={msg("刷新路线")} disabled={q.isFetching} onClick={() => void q.refetch()}><RefreshCw aria-hidden="true" /></IconAction></header>
    {q.isPending && <p role="status">{msg("正在读取")}</p>}
    {q.isError && <p role="alert">{msg("路线读取失败，请刷新后重试")}</p>}
    {q.data && !available.length && <p role="status">{msg("暂无可用路线")}</p>}
    <div className="courier-grid">
      <div className="courier-card courier-form">
        <h2>{msg("货物与路线")}</h2>
        <div className="courier-modes" role="group" aria-label={msg("货物输入方式")}>
          {(["paste", "manual"] as const).map(m => <Button key={m} variant={mode === m ? "default" : "outline"} aria-pressed={mode === m} disabled={estimate.isPending} onClick={() => { setMode(m); setVolume("0"); setCollateral("0"); resetCopy(); }}>{m === "paste" ? msg("物品清单") : msg("手动填写")}</Button>)}
        </div>
        {mode === "paste" && <form onSubmit={e => { e.preventDefault(); resetCopy(); estimate.mutate(); }}>
          <label htmlFor="courier-items">{msg("物品清单")}</label>
          <textarea id="courier-items" rows={5} maxLength={100000} value={text} disabled={estimate.isPending} onChange={e => { setText(e.target.value); estimate.reset(); resetCopy(); }} placeholder={msg("粘贴物品名称和数量")} />
          <div className="courier-actions"><Button type="submit" disabled={!text.trim() || estimate.isPending}>{estimate.isPending ? msg("正在估价…") : msg("估价")}</Button>{appraisal?.janice_url && <a href={appraisal.janice_url} target="_blank" rel="noreferrer">{msg("估价详情")} <ExternalLink size={16} /></a>}</div>
          {estimate.isError && <p role="alert">{estimate.error.message}</p>}
          {appraisal && <dl className="courier-cargo"><div><dt>{msg("物品体积")}</dt><dd>{format(appraisal.volume)} m³</dd></div><div><dt>{msg("出售价")}</dt><dd title={`${format(appraisal.sell_price)} ISK`}>{money(appraisal.sell_price)}</dd></div></dl>}
          {failed.length > 0 && <div role="alert"><strong>{msg("存在未识别物品，请修正后重新估价")}</strong><ul>{failed.map(([name, count]) => <li key={name}>{name} × {format(count)}</li>)}</ul></div>}
        </form>}
        <div className="courier-inputs">
          <label>{mode === "paste" ? msg("额外体积 / m³") : msg("货物体积 / m³")}<input type="number" min="0" step="any" inputMode="decimal" value={volume} onChange={e => { setVolume(e.target.value); resetCopy(); }} /></label>
          <label>{mode === "paste" ? msg("额外押金 / ISK") : msg("押金 / ISK")}<input type="number" min="0" step="any" inputMode="decimal" value={collateral} onChange={e => { setCollateral(e.target.value); resetCopy(); }} /></label>
        </div>
        {(!Number.isFinite(totalVolume) || !Number.isFinite(totalCollateral) || totalVolume < 0 || totalCollateral < 0) && <p role="alert">{msg("请输入有效的非负数字")}</p>}
        <div className="courier-location"><span>{msg("起点")}</span><Select label={msg("起点")} value={origin} disabled={!available.length} options={origins.map(x => ({ value: x, label: x, group: categories[x]?.category_name || "" }))} onValueChange={v => { setOrigin(v); const matches = available.filter(r => r.start === v); setRouteID(matches.length === 1 ? String(matches[0].id) : ""); resetCopy(); }} /></div>
        <div className="courier-location"><span>{msg("终点")}</span><Select label={msg("终点")} value={routeID} disabled={!destinations.length} options={destinations.map(r => ({ value: String(r.id), label: r.end, group: categories[r.end]?.category_name || "" }))} onValueChange={v => { setRouteID(v); resetCopy(); }} /></div>
      </div>
      <section className="courier-card courier-result" aria-label={msg("快递合同")}>
        <h2>{msg("快递合同")}</h2>
        {!route && <p>{msg("请选择路线")}</p>}
        {route && <>
          <dl className="courier-rates"><div><dt>{msg("体积单价")}</dt><dd>{format(route.cost_per_m3)} ISK/m³</dd></div><div><dt>{msg("押金费率")}</dt><dd>{format(route.collateral_percentage)}%</dd></div><div><dt>{msg("最低运费")}</dt><dd title={`${format(route.minimum_cost)} ISK`}>{money(route.minimum_cost)}</dd></div><div><dt>{msg("运量上限")}</dt><dd>{route.transport_volume ? `${format(route.transport_volume)} m³` : msg("不限")}</dd></div><div><dt>{msg("到期天数")}</dt><dd>{msg("{0} 天", route.expiration)}</dd></div><div><dt>{msg("完成天数")}</dt><dd>{msg("{0} 天", route.completion)}</dd></div></dl>
          {result && <>
            <dl className="courier-totals"><div><dt>{msg("运费")}</dt><dd title={`${format(result.reward)} ISK`}>{money(result.reward)}</dd></div><div><dt>{msg("总计体积")}</dt><dd>{format(result.volume)} m³</dd></div></dl>
            <div className="courier-breakdown"><span>{msg("体积运费")} {money(result.volumeCost)}</span><span>{msg("押金费用")} {money(result.collateralCost)}</span>{result.minimumApplied && <span>{msg("已按最低运费计算")}</span>}</div>
            {result.oversized && <p role="alert">{msg("体积超出路线运量，请拆分合同")}</p>}
            {mode === "paste" && !appraisal && <p role="status">{msg("请先完成物品估价")}</p>}
            <div className="courier-fields">
              {field(msg("合同发给"), "Heavy Assets Logistics")}
              {field(msg("运送至"), route.end)}
              {field(msg("报酬 / ISK"), String(Math.round(result.reward)))}
              {field(msg("合同押金 / ISK"), String(Math.round(result.collateral)))}
            </div>
            <Button variant="outline" disabled={!ready} onClick={() => void copy(msg("合同信息"), [`${msg("合同发给")}: Heavy Assets Logistics`, `${msg("起点")}: ${route.start}`, `${msg("运送至")}: ${route.end}`, `${msg("报酬 / ISK")}: ${Math.round(result.reward)}`, `${msg("合同押金 / ISK")}: ${Math.round(result.collateral)}`, `${msg("到期天数")}: ${route.expiration}`, `${msg("完成天数")}: ${route.completion}`, `${msg("总计体积")}: ${result.volume} m³`].join("\n"))}><Copy size={18} aria-hidden="true" />{copied.startsWith(`${msg("合同信息")}:`) ? msg("已复制") : msg("复制合同信息")}</Button>
          </>}
        </>}
        {copyError && <p role="alert">{copyError}</p>}
      </section>
    </div>
  </div>;
}

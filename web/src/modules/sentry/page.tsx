import { msg, getLocale } from "@/lib/i18n";
import { type ReactNode, useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Navigate } from "react-router-dom";
import { RadioTower, RefreshCw, Copy, Coins, Clock3, Settings2, Save, X } from "lucide-react";
import { useSession } from "@/modules/identity";
import { Button } from "@/components/ui/button";
import { IconAction } from "@/components/ui/icon-action";
import { useToast } from "@/components/ui/toast-context";
import * as api from "./api";
import "./sentry.css";

const date = (value: string) =>
  value ? new Date(value).toLocaleString(getLocale(), { hour12: false }) : "—";

const coins = (minor: number) =>
  (minor / 100).toLocaleString(getLocale(), {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  });

const duration = (seconds: number) => {
  const minutes = Math.floor(seconds / 60);
  const rest = seconds % 60;
  return minutes > 0 ? `${minutes}${msg("分钟")} ${rest}${msg("秒")}` : `${rest}${msg("秒")}`;
};

export default function SentryPage() {
  const session = useSession();
  if (session.isError) return <p role="alert">{session.error.message}</p>;
  if (!session.data) return <p role="status">{msg("正在读取")}</p>;
  if (!session.data.session) return <Navigate to="/login" replace />;
  return <Workspace csrf={session.data.session.csrf_token} />;
}

function Workspace({ csrf }: { csrf: string }) {
  const queryClient = useQueryClient();
  const toast = useToast();
  // The API intentionally returns the plaintext only from create/rotate. Keep
  // each returned value in page memory so a query refresh cannot make the
  // copy action depend on the last-rendered card.
  const [secrets, setSecrets] = useState<Record<string, string>>({});
  const [copied, setCopied] = useState(false);
  const autoCreateStarted = useRef(false);
  const requestKey = useRef(crypto.randomUUID());
  const q = useQuery({ queryKey: ["sentry", "keys"], queryFn: ({ signal }) => api.list(signal) });
  const create = useMutation({
    mutationFn: () => api.create(csrf, "默认预警密钥", ["monitor", "alert"], requestKey.current),
    onSuccess: (key) => {
      autoCreateStarted.current = true;
      requestKey.current = crypto.randomUUID();
      if (key.secret) setSecrets((values) => ({ ...values, [key.id]: key.secret! }));
      setCopied(false);
      void queryClient.invalidateQueries({ queryKey: ["sentry", "keys"] });
    },
    onError: () => {
      autoCreateStarted.current = false;
      void queryClient.invalidateQueries({ queryKey: ["sentry", "keys"] });
    },
  });
  const rotate = useMutation({
    mutationFn: (id: string) => api.rotate(csrf, id),
    onSuccess: (key) => {
      if (key.secret) setSecrets((values) => ({ ...values, [key.id]: key.secret! }));
      setCopied(false);
      void queryClient.invalidateQueries({ queryKey: ["sentry", "keys"] });
    },
  });
  const items = q.data?.items || [];
  useEffect(() => {
    if (!q.isLoading && !q.isError && items.length === 0 && !create.isPending && !autoCreateStarted.current) {
      autoCreateStarted.current = true;
      create.mutate();
    }
  }, [create, items.length, q.isError, q.isLoading]);

  const retryAutoCreate = () => {
    autoCreateStarted.current = false;
    create.reset();
    requestKey.current = crypto.randomUUID();
    create.mutate();
  };

  const refreshKey = (id: string) => {
    setSecrets((values) => {
      const next = { ...values };
      delete next[id];
      return next;
    });
    setCopied(false);
    rotate.mutate(id);
  };

  const copySecret = async (value: string) => {
    if (!navigator.clipboard?.writeText) {
      toast.error(msg("当前浏览器不支持复制"));
      return;
    }
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      toast.success(msg("密钥已复制"));
    } catch {
      toast.error(msg("复制失败，请手动复制"));
    }
  };

  const copyCardSecret = (id: string) => {
    const value = secrets[id];
    if (value) {
      void copySecret(value);
      return;
    }
    toast.info(msg("完整密钥仅在生成或刷新后可用"));
  };

  return (
    <div className="sentry-page">
      <header className="sentry-heading">
        <span className="sentry-brand"><RadioTower aria-hidden="true" /></span>
        <div><h1>{msg("预警平台")}</h1><p>{msg("查看预警消耗并管理客户端密钥")}</p></div>
      </header>
      {q.isError && <div className="sentry-state" role="alert">{q.error.message}<Button variant="outline" onClick={() => void q.refetch()}>{msg("重试")}</Button></div>}
      {!q.isError && items.length === 0 && create.isError && <div className="sentry-state" role="alert"><span>{msg("密钥准备失败")}</span><Button variant="outline" onClick={retryAutoCreate}>{msg("重试")}</Button></div>}
      <AlertUsagePanel csrf={csrf} keyActions={items.map((key) => <span className="sentry-key-actions" key={key.id}>
        <Button variant="outline" onClick={() => copyCardSecret(key.id)} aria-label={msg("复制密钥")}><Copy aria-hidden="true" />{msg(copied ? "已复制" : "复制密钥")}</Button>
        <Button onClick={() => refreshKey(key.id)} disabled={key.status !== "active" || rotate.isPending} aria-busy={rotate.isPending}><RefreshCw aria-hidden="true" />{msg("更新密钥")}</Button>
      </span>)} />
    </div>
  );
}

function AlertUsagePanel({ csrf, keyActions }: { csrf: string; keyActions: ReactNode }) {
  const [before, setBefore] = useState("");
  const [state, setState] = useState("");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const usage = useQuery({
    queryKey: ["sentry", "alert-usage"],
    queryFn: ({ signal }) => api.usage(signal),
    refetchInterval: 60_000,
  });
  const page = useQuery({
    queryKey: ["sentry", "alert-consumptions", before, state, from, to],
    queryFn: ({ signal }) =>
      api.consumptions(
        {
          before,
          state,
          from: from ? new Date(`${from}T00:00:00`).toISOString() : "",
          to: to ? new Date(`${to}T23:59:59.999`).toISOString() : "",
        },
        signal,
      ),
    refetchInterval: 60_000,
  });
  const pricing = useQuery({
    queryKey: ["sentry", "alert-pricing"],
    queryFn: ({ signal }) => api.pricing(signal),
    refetchInterval: 60_000,
  });
  const refresh = () => {
    void usage.refetch();
    void page.refetch();
  };
  const setFilter = (setter: (value: string) => void, value: string) => {
    setter(value);
    setBefore("");
  };
  return (
    <section className="sentry-usage" aria-labelledby="sentry-usage-title">
      <div className="sentry-usage-heading">
        <div>
          <h2 id="sentry-usage-title">{msg("预警果壳币消费")}</h2>
        </div>
        <div className="sentry-usage-actions">
          {keyActions}
          <IconAction label={msg("刷新消费记录")} disabled={usage.isFetching || page.isFetching} onClick={refresh}>
            <RefreshCw size={18} aria-hidden="true" />
          </IconAction>
        </div>
      </div>
      {usage.isError ? (
        <div className="sentry-state" role="alert">
          <span>{usage.error.message}</span>
          <Button variant="outline" onClick={refresh}>{msg("重试")}</Button>
        </div>
      ) : usage.data ? (
        <>
          <div className="sentry-usage-summary">
            <UsageMetric icon={<Coins aria-hidden="true" />} label={msg("可用果壳币")} value={coins(usage.data.available_minor)} />
            <UsageMetric icon={<Clock3 aria-hidden="true" />} label={msg("预警暂占")} value={coins(usage.data.alert_reserved_minor)} />
            <UsageMetric icon={<Coins aria-hidden="true" />} label={msg("预警累计净消费")} value={coins(usage.data.alert_settled_minor)} />
            <UsageMetric icon={<Coins aria-hidden="true" />} label={msg("预警已释放")} value={coins(usage.data.alert_released_minor)} />
            <UsageMetric icon={<Coins aria-hidden="true" />} label={msg("预警累计退款")} value={coins(usage.data.alert_refunded_minor)} />
          </div>
          <p className="sentry-usage-asof">{msg("统计时间")}：{date(usage.data.as_of)}</p>
        </>
      ) : (
        <p className="sentry-usage-loading" role="status">{msg("正在读取")}</p>
      )}
      {pricing.isError ? (
        <p className="sentry-usage-error" role="alert">{pricing.error.message}</p>
      ) : pricing.data ? (
        <AlertPricingPanel key={`${pricing.data.version}:${pricing.data.updated_at ?? ""}`} pricing={pricing.data} csrf={csrf} />
      ) : (
        <p className="sentry-usage-loading" role="status">{msg("正在读取收费配置")}</p>
      )}
      <div className="sentry-usage-toolbar" aria-label={msg("筛选消费记录")}>
        <label>
          <span>{msg("状态")}</span>
          <select value={state} onChange={(event) => setFilter(setState, event.target.value)}>
            <option value="">{msg("全部")}</option>
            <option value="reserved">{msg("暂占")}</option>
            <option value="settled">{msg("已结算")}</option>
            <option value="released">{msg("已释放")}</option>
            <option value="refunded">{msg("已退款")}</option>
          </select>
        </label>
        <label><span>{msg("开始日期")}</span><input type="date" value={from} onChange={(event) => setFilter(setFrom, event.target.value)} /></label>
        <label><span>{msg("结束日期")}</span><input type="date" value={to} onChange={(event) => setFilter(setTo, event.target.value)} /></label>
      </div>
      {page.isError ? (
        <p className="sentry-usage-error" role="alert">{page.error.message}</p>
      ) : page.isPending ? (
        <p className="sentry-usage-loading" role="status">{msg("正在读取")}</p>
      ) : !page.data.items.length ? (
        <p className="sentry-usage-empty">{msg("暂无预警消费记录")}</p>
      ) : (
        <div className="sentry-consumption-list">
          {page.data.items.map((item) => <ConsumptionRow key={item.id} item={item} />)}
        </div>
      )}
      {(before || page.data?.next_cursor) && (
        <div className="sentry-usage-pagination">
          <Button variant="outline" disabled={!before} onClick={() => setBefore("")}>{msg("返回最新")}</Button>
          <Button variant="outline" disabled={!page.data?.next_cursor} onClick={() => setBefore(page.data?.next_cursor || "")}>{msg("更早记录")}</Button>
        </div>
      )}
    </section>
  );
}

function AlertPricingPanel({ pricing, csrf }: { pricing: api.AlertPricing; csrf: string }) {
  const [editing, setEditing] = useState(false);
  const client = useQueryClient();
  const [priceVersion, setPriceVersion] = useState(pricing.price_version);
  const [unitSeconds, setUnitSeconds] = useState(String(pricing.unit_seconds));
  const [unitPrice, setUnitPrice] = useState((pricing.unit_price_minor / 100).toFixed(2));
  const [maxGrantSeconds, setMaxGrantSeconds] = useState(String(pricing.max_grant_seconds));
  const [ttlSeconds, setTtlSeconds] = useState(String(pricing.grant_ttl_seconds));
  const save = useMutation({
    mutationFn: () => api.updatePricing(csrf, {
      price_version: priceVersion.trim(),
      unit_seconds: Number(unitSeconds),
      unit_price_minor: Math.round(Number(unitPrice) * 100),
      max_grant_seconds: Number(maxGrantSeconds),
      grant_ttl_seconds: Number(ttlSeconds),
      version: pricing.version,
    }),
    onSuccess: () => {
      setEditing(false);
      void client.invalidateQueries({ queryKey: ["sentry", "alert-pricing"] });
    },
  });
  const valid = priceVersion.trim() !== "" &&
    Number.isSafeInteger(Number(unitSeconds)) && Number(unitSeconds) > 0 && Number(unitSeconds) <= 86400 &&
    /^\d+(\.\d{1,2})?$/.test(unitPrice) && Number(unitPrice) > 0 && Number(unitPrice) <= 10000000000 &&
    Number.isSafeInteger(Number(maxGrantSeconds)) && Number(maxGrantSeconds) > 0 && Number(maxGrantSeconds) <= 2678400 &&
    Number.isSafeInteger(Number(ttlSeconds)) && Number(ttlSeconds) >= 60 && Number(ttlSeconds) <= 2678400;
  const hasPolicy = pricing.unit_seconds > 0 && pricing.unit_price_minor > 0 && pricing.max_grant_seconds > 0 && pricing.grant_ttl_seconds >= 60;
  return (
    <section className="sentry-pricing" aria-labelledby="sentry-pricing-title">
      <div className="sentry-pricing-heading">
        <div>
          <h3 id="sentry-pricing-title">{msg("预警收费配置")}</h3>
        </div>
        <div className="sentry-pricing-actions">
          <span className={`sentry-pricing-status ${pricing.charging_enabled ? "is-on" : "is-off"}`}>
            {pricing.charging_enabled ? msg("收费已启用") : msg("收费开关未启用")}
          </span>
          {pricing.can_edit && !editing && <Button variant="outline" onClick={() => setEditing(true)}><Settings2 aria-hidden="true" />{msg("配置收费")}</Button>}
        </div>
      </div>
      {editing && pricing.can_edit ? (
        <form className="sentry-pricing-form" onSubmit={(event) => { event.preventDefault(); if (valid && !save.isPending) save.mutate(); }}>
          <label><span>{msg("价格版本")}</span><input value={priceVersion} onChange={(event) => setPriceVersion(event.target.value)} maxLength={80} required /></label>
          <label><span>{msg("计价单位（秒）")}</span><input type="number" min={1} max={86400} value={unitSeconds} onChange={(event) => setUnitSeconds(event.target.value)} required /></label>
          <label><span>{msg("每单位价格（果壳币）")}</span><input type="number" min={0.01} max={10000000000} step={0.01} value={unitPrice} onChange={(event) => setUnitPrice(event.target.value)} required /></label>
          <label><span>{msg("单次授权上限（秒）")}</span><input type="number" min={1} max={2678400} value={maxGrantSeconds} onChange={(event) => setMaxGrantSeconds(event.target.value)} required /></label>
          <label><span>{msg("授权有效期（秒）")}</span><input type="number" min={60} max={2678400} value={ttlSeconds} onChange={(event) => setTtlSeconds(event.target.value)} required /></label>
          <div className="sentry-pricing-form-actions"><Button type="button" variant="outline" onClick={() => setEditing(false)} disabled={save.isPending}><X aria-hidden="true" />{msg("取消")}</Button><Button type="submit" disabled={!valid || save.isPending}><Save aria-hidden="true" />{save.isPending ? msg("正在保存") : msg("保存收费配置")}</Button></div>
          {save.isError && <p role="alert">{save.error.message}</p>}
        </form>
      ) : (
        <div className="sentry-pricing-summary">
          <div><span>{msg("计价单位")}</span><strong>{hasPolicy ? `${pricing.unit_seconds}${msg("秒")}` : "—"}</strong></div>
          <div><span>{msg("每单位价格")}</span><strong>{hasPolicy ? `${coins(pricing.unit_price_minor)} ${msg("币")}` : "—"}</strong></div>
          <div><span>{msg("授权上限")}</span><strong>{hasPolicy ? `${pricing.max_grant_seconds}${msg("秒")}` : "—"}</strong></div>
          <div><span>{msg("授权有效期")}</span><strong>{hasPolicy ? `${pricing.grant_ttl_seconds}${msg("秒")}` : "—"}</strong></div>
          <small>{pricing.configured ? `${msg("版本")}：${pricing.price_version}` : hasPolicy ? msg("尚未保存独立收费配置，当前使用部署默认值") : msg("尚未配置预警收费规则")}</small>
        </div>
      )}
    </section>
  );
}

function UsageMetric({ icon, label, value }: { icon: ReactNode; label: string; value: string }) {
  return <div className="sentry-usage-metric"><span className="sentry-usage-metric-icon">{icon}</span><span>{label}</span><strong>{value} <small>{msg("币")}</small></strong></div>;
}

function ConsumptionRow({ item }: { item: api.AlertConsumption }) {
  const label = item.state === "reserved" ? msg("暂占") : item.state === "settled" ? msg("已结算") : item.state === "released" ? msg("已释放") : msg("已退款");
  const returned = item.state === "released" || item.state === "refunded";
  return (
    <article className="sentry-consumption-row">
      <div className="sentry-consumption-main">
        <div><strong>{date(item.started_at)}</strong><span>{duration(item.duration_seconds)}</span></div>
        <div className="sentry-consumption-amount"><strong className={returned ? "is-returned" : ""}>{returned ? "+" : "−"}{coins(item.coins_minor)} {msg("币")}</strong><span className={`sentry-consumption-state is-${item.state}`}>{label}</span></div>
      </div>
      <div className="sentry-consumption-meta">{msg("计价")}：{item.unit_seconds}{msg("秒")} / {coins(item.unit_price_minor)} {msg("币")} · {msg("到期")}：{date(item.expires_at)}</div>
      <details className="sentry-consumption-details"><summary>{msg("查看记录详情")}</summary><dl><div><dt>{msg("区间编号")}</dt><dd>{item.interval_id}</dd></div><div><dt>{msg("授权编号")}</dt><dd>{item.grant_id}</dd></div>{item.price_version && <div><dt>{msg("价格版本")}</dt><dd>{item.price_version}</dd></div>}</dl></details>
    </article>
  );
}

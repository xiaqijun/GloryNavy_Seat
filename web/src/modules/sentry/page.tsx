import { msg, getLocale } from "@/lib/i18n";
import { type ReactNode, useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Navigate } from "react-router-dom";
import { RadioTower, RefreshCw, Copy, Coins, Clock3, Settings2, CircleCheck, CircleOff } from "lucide-react";
import { useSession } from "@/modules/identity";
import { Button } from "@/components/ui/button";
import { IconAction } from "@/components/ui/icon-action";
import { FormDialog } from "@/components/ui/form-dialog";
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
    queryKey: ["sentry", "time-pricing"],
    queryFn: ({ signal }) => api.timePricing(signal),
    refetchInterval: 60_000,
  });
  const [pricingEditing, setPricingEditing] = useState(false);
  const refresh = () => {
    void usage.refetch();
    void page.refetch();
  };
  const setFilter = (setter: (value: string) => void, value: string) => {
    setter(value);
    setBefore("");
  };
  const priceValue = pricing.data && pricing.data.alert_hourly_price_minor > 0 ? `${coins(pricing.data.alert_hourly_price_minor)} ${msg("币")}/${msg("小时")}` : "—";
  return (
    <section className="sentry-usage" aria-labelledby="sentry-usage-title">
      <div className="sentry-usage-heading">
        <div>
          <h2 id="sentry-usage-title">{msg("预警果壳币消费")}</h2>
        </div>
        <div className="sentry-usage-actions">
          {keyActions}
          {pricing.data && (
            <span
              className={`sentry-charging-indicator ${pricing.data.charging_enabled ? "is-on" : "is-off"}`}
              aria-label={pricing.data.charging_enabled ? msg("收费已启用") : msg("收费开关未启用")}
              title={pricing.data.charging_enabled ? msg("收费已启用") : msg("收费开关未启用")}
            >
              {pricing.data.charging_enabled ? <CircleCheck size={18} aria-hidden="true" /> : <CircleOff size={18} aria-hidden="true" />}
            </span>
          )}
          {pricing.data?.can_edit && !pricingEditing && <Button variant="outline" onClick={() => setPricingEditing(true)}><Settings2 aria-hidden="true" />{msg("配置收费")}</Button>}
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
            <UsageMetric icon={<Clock3 aria-hidden="true" />} label={msg("预警消费价格")} value={priceValue} unit="" />
            <UsageMetric icon={<Coins aria-hidden="true" />} label={msg("预警累计净消费")} value={coins(usage.data.alert_settled_minor)} />
            <UsageMetric icon={<Coins aria-hidden="true" />} label={msg("监控奖励")} value={coins(usage.data.monitor_reward_minor)} />
          </div>
        </>
      ) : (
        <p className="sentry-usage-loading" role="status">{msg("正在读取")}</p>
      )}
      {pricing.isError ? (
        <p className="sentry-usage-error" role="alert">{pricing.error.message}</p>
      ) : pricing.data ? (
        <AlertPricingPanel key={`${pricing.data.version}:${pricing.data.updated_at ?? ""}`} pricing={pricing.data} csrf={csrf} editing={pricingEditing} setEditing={setPricingEditing} />
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

function AlertPricingPanel({ pricing, csrf, editing, setEditing }: { pricing: api.TimePricing; csrf: string; editing: boolean; setEditing: (value: boolean) => void }) {
  const client = useQueryClient();
  const [alertPrice, setAlertPrice] = useState((pricing.alert_hourly_price_minor / 100).toFixed(2));
  const [monitorPrice, setMonitorPrice] = useState((pricing.monitor_hourly_reward_minor / 100).toFixed(2));
  const [chargingEnabled, setChargingEnabled] = useState(pricing.charging_enabled);
  const save = useMutation({
    mutationFn: () => api.updateTimePricing(csrf, {
      alert_hourly_price_minor: Math.round(Number(alertPrice) * 100),
      monitor_hourly_reward_minor: Math.round(Number(monitorPrice) * 100),
      version: pricing.version,
      charging_enabled: chargingEnabled,
    }),
    onSuccess: () => {
      setEditing(false);
      void client.invalidateQueries({ queryKey: ["sentry", "time-pricing"] });
      void client.invalidateQueries({ queryKey: ["sentry", "alert-usage"] });
    },
  });
  const valid = /^\d+(\.\d{1,2})?$/.test(alertPrice) && Number(alertPrice) > 0 && Number(alertPrice) <= 10000000000 &&
    /^\d+(\.\d{1,2})?$/.test(monitorPrice) && Number(monitorPrice) >= 0 && Number(monitorPrice) <= 10000000000;
  return (
    <>
      {editing && pricing.can_edit && (
        <FormDialog
          title={msg("预警收费配置")}
          close={() => setEditing(false)}
          busy={save.isPending}
          disabled={!valid}
          submitLabel={msg("保存收费配置")}
          className="sentry-pricing-form"
          onSubmit={() => save.mutate()}
        >
          <label className="sentry-pricing-switch">
            <input type="checkbox" checked={chargingEnabled} onChange={(event) => setChargingEnabled(event.target.checked)} />
            <span>{msg("启用预警收费")}</span>
          </label>
          <label><span>{msg("预警消费价格（果壳币/小时）")}</span><input autoFocus type="number" min={0.01} max={10000000000} step={0.01} value={alertPrice} onChange={(event) => setAlertPrice(event.target.value)} required /></label>
          <label><span>{msg("监控奖励价格（果壳币/小时）")}</span><input type="number" min={0} max={10000000000} step={0.01} value={monitorPrice} onChange={(event) => setMonitorPrice(event.target.value)} required /></label>
          {save.isError && <p role="alert">{save.error.message}</p>}
        </FormDialog>
      )}
    </>
  );
}

function UsageMetric({ icon, label, value, unit = msg("币") }: { icon: ReactNode; label: string; value: string; unit?: string }) {
  return <div className="sentry-usage-metric"><span className="sentry-usage-metric-icon">{icon}</span><span>{label}</span><strong>{value}{unit && <small> {unit}</small>}</strong></div>;
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

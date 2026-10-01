import { msg, getLocale } from "@/lib/i18n";
import { useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Navigate } from "react-router-dom";
import {
  Calculator,
  FileText,
  Settings2,
  ArrowDownLeft,
  ArrowUpRight,
  Scale,
  ClipboardList,
} from "lucide-react";
import { useSession } from "@/modules/identity";
import { Button } from "@/components/ui/button";
import { IconAction } from "@/components/ui/icon-action";
import { Modal } from "@/components/ui/dialog";
import { EveImage } from "@/components/eve-image";
import * as api from "./api";
import "./market.css";
import { ContractPicker } from "./contract-picker";
const money = (v: string | null) =>
  v === null ? "—" : v.replace(/\B(?=(\d{3})+(?!\d))/g, ",");
const labels = {
  buy: msg("收购价"),
  mid: msg("中间价"),
  sell: msg("出售价"),
} as const;
const status: Record<string, string> = {
  invalid_quantity: msg("数量格式无效"),
  unknown_type: msg("未识别物品"),
  unavailable: msg("报价暂不可用，可重试"),
  missing_orders: msg("缺少买单或卖单"),
};
export default function MarketPage() {
  const s = useSession();
  if (s.isError) return <p role="alert">{s.error.message}</p>;
  if (!s.data) return <p role="status">{msg("正在读取")}</p>;
  if (!s.data.session) return <Navigate to="/login" replace />;
  return (
    <Workspace
      key={s.data.session.user_id}
      user={s.data.session.user_id}
      csrf={s.data.session.csrf_token}
    />
  );
}
function Workspace({ user, csrf }: { user: string; csrf: string }) {
  const [text, setText] = useState("");
  const [picker, setPicker] = useState(false);
  const [source, setSource] = useState("");
  const [editor, setEditor] = useState(false);
  const [result, setResult] = useState<api.Appraisal | null>(null);
  const q = useQuery({
    queryKey: ["market", "settings", user],
    queryFn: ({ signal }) => api.context(signal),
  });
  const estimate = useMutation({
    mutationFn: () => api.estimate(csrf, text),
    onSuccess: (value) => {
      setResult(value);
      setSource("");
    },
  });
  return (
    <div className="market-page">
      <header className="market-heading">
        <span className="market-symbol">
          <Calculator aria-hidden="true" />
        </span>
        <h1>{msg("物品估价")}</h1>
        <span className="market-location">{msg("吉他 4-4")}</span>
        {q.data?.administrator && (
          <IconAction label={msg("估价比例")} onClick={() => setEditor(true)}>
            <Settings2 aria-hidden="true" />
          </IconAction>
        )}
      </header>
      {q.isError && (
        <p role="alert">
          {q.error.message}{" "}
          <Button variant="outline" onClick={() => void q.refetch()}>
            {msg("重试")}{" "}
          </Button>
        </p>
      )}
      <form
        className="market-input"
        onSubmit={(e) => {
          e.preventDefault();
          estimate.mutate();
        }}
      >
        <div className="market-input-heading">
          <label htmlFor="appraisal-items">
            <ClipboardList size={18} aria-hidden="true" />
            {msg("物品清单")}{" "}
          </label>
          <Button
            type="button"
            variant="outline"
            disabled={estimate.isPending || !q.data}
            onClick={() => setPicker(true)}
          >
            <FileText size={18} aria-hidden="true" />
            {msg("从合同估价")}
          </Button>
        </div>
        <textarea
          id="appraisal-items"
          value={text}
          disabled={estimate.isPending}
          onChange={(e) => setText(e.target.value)}
          maxLength={32768}
          placeholder={msg(
            "粘贴游戏中的物品清单，或逐行输入：\n三钛合金 × 1000\nDamage Control II × 1",
          )}
          rows={5}
        />
        <div className="market-input-actions">
          <span>{msg("最多 100 行 · 数量默认为 1")}</span>
          <Button
            type="submit"
            disabled={!text.trim() || estimate.isPending || !q.data}
          >
            {estimate.isPending ? msg("正在估价…") : msg("估价")}
          </Button>
        </div>
        {estimate.isError && <p role="alert">{estimate.error.message}</p>}
      </form>
      {result && (
        <section aria-label={msg("估价结果")} aria-busy={estimate.isPending}>
          <div className="market-result-heading">
            <h2>{msg("估价结果")}</h2>
            <span>
              {msg("折算比例")} {result.ratio_bps / 100}%
            </span>
          </div>
          {source && <p className="market-source">{source}</p>}
          {!result.complete && (
            <p className="market-incomplete" role="status">
              {msg("部分物品报价不完整，以下为已知小计。")}{" "}
            </p>
          )}
          <div className="market-totals">
            {(
              [
                ["buy", ArrowDownLeft],
                ["mid", Scale],
                ["sell", ArrowUpRight],
              ] as const
            ).map(([key, Icon]) => (
              <article
                key={key}
                className={
                  key === "mid" ? "market-total is-mid" : "market-total"
                }
              >
                <div>
                  <Icon size={18} aria-hidden="true" />
                  <span>{labels[key]}</span>
                </div>
                <strong>
                  {money(result.totals[key])}
                  <small>ISK</small>
                </strong>
                <p>
                  {msg("折算")} <b>{money(result.adjusted[key])}</b> ISK
                </p>
              </article>
            ))}
          </div>
          <div className="market-table-wrap">
            <table>
              <caption className="sr-only">
                {msg("吉他 4-4 物品估价明细，金额均为数量合计")}{" "}
              </caption>
              <thead>
                <tr>
                  <th>{msg("物品")}</th>
                  <th>{msg("数量")}</th>
                  <th>{msg("收购价 / ISK")}</th>
                  <th>{msg("中间价 / ISK")}</th>
                  <th>{msg("出售价 / ISK")}</th>
                </tr>
              </thead>
              <tbody>
                {result.lines.map((l, i) => (
                  <tr key={i}>
                    <td>
                      <div className="market-item">
                        {l.type_id !== "0" && (
                          <EveImage kind="type" id={l.type_id} />
                        )}
                        <div>
                          <span>{l.name}</span>
                          {l.status !== "ready" && (
                            <small className="market-row-warning">
                              {status[l.status]}
                            </small>
                          )}
                          {l.observed_at && (
                            <small>
                              {msg("行情")}{" "}
                              {new Date(l.observed_at).toLocaleString(
                                getLocale(),
                                {
                                  hour12: false,
                                },
                              )}
                            </small>
                          )}
                        </div>
                      </div>
                    </td>
                    <td>
                      {l.quantity === "0"
                        ? "—"
                        : Number(l.quantity).toLocaleString()}
                    </td>
                    <td>{money(l.buy)}</td>
                    <td>{money(l.mid)}</td>
                    <td>{money(l.sell)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <p className="market-footnote">
            {msg(
              "吉他 4-4 买一与卖一的均值为中间价；不含税费，不按订单深度估算成交。",
            )}{" "}
          </p>
        </section>
      )}
      {picker && (
        <ContractPicker
          user={user}
          csrf={csrf}
          close={() => setPicker(false)}
          done={(value, selectedSource) => {
            setResult(value);
            setSource(selectedSource);
            setPicker(false);
          }}
        />
      )}
      {editor && q.data && (
        <RatioEditor
          value={q.data.settings}
          csrf={csrf}
          close={() => setEditor(false)}
          done={() => {
            setEditor(false);
            setResult(null);
            void q.refetch();
          }}
        />
      )}
    </div>
  );
}
function RatioEditor({
  value,
  csrf,
  close,
  done,
}: {
  value: api.Settings;
  csrf: string;
  close: () => void;
  done: () => void;
}) {
  const [ratio, setRatio] = useState(String(value.ratio_bps / 100));
  const save = useMutation({
    mutationFn: () =>
      api.configure(csrf, {
        version: value.version,
        ratio_bps: Math.round(Number(ratio) * 100),
      }),
    onSuccess: done,
  });
  const valid = /^\d+(\.\d{1,2})?$/.test(ratio) && Number(ratio) <= 1000;
  return (
    <Modal
      title={msg("估价比例")}
      close={close}
      busy={save.isPending}
      size="compact"
      footer={
        <Button
          type="submit"
          form="market-ratio"
          disabled={!valid || save.isPending}
        >
          {save.isPending ? msg("保存中…") : msg("保存")}
        </Button>
      }
    >
      <form
        id="market-ratio"
        className="market-ratio-form"
        onSubmit={(e) => {
          e.preventDefault();
          save.mutate();
        }}
      >
        <label htmlFor="market-ratio-input">{msg("统一比例 / %")}</label>
        <input
          id="market-ratio-input"
          inputMode="decimal"
          value={ratio}
          onChange={(e) => setRatio(e.target.value)}
        />
        <small>{msg("收购价、中间价和出售价共用；100% 为原价。")}</small>
        {!valid && <p role="alert">{msg("请输入 0–1000，最多两位小数。")}</p>}
        {save.isError && <p role="alert">{save.error.message}</p>}
      </form>
    </Modal>
  );
}

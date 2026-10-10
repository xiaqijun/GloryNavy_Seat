import { msg, getLocale } from "@/lib/i18n";
import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { EveImage } from "@/components/eve-image";
import * as api from "./api";

const isk = (v: string | null) =>
  v === null
    ? "—"
    : Number(v).toLocaleString(getLocale(), { maximumFractionDigits: 2 });

export function ValuationView({
  item,
  csrf,
  refresh,
  detailOnly = false,
}: {
  item: api.Case;
  csrf: string;
  refresh: boolean;
  detailOnly?: boolean;
}) {
  const cache = useQueryClient();
  const [key, setKey] = useState(() => crypto.randomUUID());
  const v = item.detail.valuation;
  const mutation = useMutation({
    mutationFn: () =>
      api.post("commands", csrf, {
        action: "appraise",
        id: item.id,
        version: item.version,
        request_key: key,
        note: msg("重新核价"),
      }),
    onSuccess: async () => {
      setKey(crypto.randomUUID());
      await cache.invalidateQueries({ queryKey: ["welfare"] });
    },
  });
  return (
    <section
      className="welfare-valuation"
      aria-label={msg("补损核价")}
      aria-busy={mutation.isPending}
    >
      {(!detailOnly || refresh) && <div className="welfare-valuation-head">
        <div>
          {!detailOnly && <><span>
            {v?.source === "purchase_contract"
              ? msg("购舰合同核价")
              : msg("市场中间价核价")}
          </span>
          <strong>
            {v?.state === "ready"
              ? `${api.money(v.amount_minor)} ISK`
              : msg(api.valuationStateMessage(v?.state))}
          </strong></>}
        </div>
        {refresh && (
          <Button
            type="button"
            variant="outline"
            disabled={mutation.isPending}
            onClick={() => mutation.mutate()}
          >
            <RefreshCw size={16} aria-hidden="true" />
            {mutation.isPending ? msg("核价中") : msg("重新核价")}
          </Button>
        )}
      </div>}
      {v?.reason && <p role="status">{msg(v.reason)}</p>}
      {v && (
        <p className="welfare-valuation-meta">
          {api.date(v.at)}
          {v.market && msg(" · 吉他 4-4 · {0}%", v.market.ratio_bps / 100)}
          {item.detail.pricing_mode === "manual" && msg(" · 已人工核价")}
        </p>
      )}
      {v?.market && (
        <details>
          <summary>
            {msg("船体及全部物品 ·")} {v.market.lines.length} {msg("项")}
          </summary>
          <div className="welfare-valuation-table">
            <table>
              <thead>
                <tr>
                  <th>{msg("物品")}</th>
                  <th>{msg("数量")}</th>
                  <th>{msg("中间价 / ISK")}</th>
                  <th>{msg("行情时间")}</th>
                </tr>
              </thead>
              <tbody>
                {v.market.lines.map((l, i) => (
                  <tr key={i}>
                    <td>
                      <EveImage kind="type" id={l.type_id} />
                      <span>{l.name}</span>
                    </td>
                    <td>{Number(l.quantity).toLocaleString(getLocale())}</td>
                    <td>{l.mid === null ? msg("缺少报价") : isk(l.mid)}</td>
                    <td>{l.observed_at ? api.date(l.observed_at) : "—"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <p>
            {v.market.complete ? msg("原价合计") : msg("已知小计")}：
            {isk(v.market.totals.mid)} ISK
          </p>
        </details>
      )}
      {v?.contract && (
        <details>
          <summary>
            {msg("购舰合同 #")}
            {v.contract.id}
          </summary>
          <p>
            {v.contract.title || msg("物品交换")} ·{" "}
            {isk(v.contract.price || null)} ISK
          </p>
          <ul>
            {v.contract.items.map((i, n) => (
              <li key={n}>
                {i.name} × {i.quantity}
                {!i.included && msg("（索取）")}
              </li>
            ))}
          </ul>
        </details>
      )}
      {mutation.isError && <p role="alert">{mutation.error.message}</p>}
    </section>
  );
}

import { msg, getLocale } from "@/lib/i18n";
import { useEffect, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, Navigate } from "react-router-dom";
import { Coins } from "lucide-react";
import { useSession } from "@/modules/identity";
import { getData } from "@/lib/http";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { getContext, formatDate } from "./api";
import { RewardsPanel } from "./rewards-panel";
import "./exchange.css";
export default function ExchangePage() {
  const s = useSession();
  if (s.isError) return <p role="alert">{s.error.message}</p>;
  if (!s.data) return <p role="status">{msg("正在读取")}</p>;
  if (!s.data.session) return <Navigate to="/login" replace />;
  return (
    <Workspace user={s.data.session.user_id} csrf={s.data.session.csrf_token} />
  );
}
function Workspace({ user, csrf }: { user: string; csrf: string }) {
  const q = useQuery({
    queryKey: ["exchange", "context", user],
    queryFn: ({ signal }) => getContext(signal),
  });
  return (
    <div className="exchange-page">
      <header className="exchange-heading">
        <span className="exchange-mark">
          <Coins aria-hidden="true" />
        </span>
        <h1>{msg("兑换中心")}</h1>
      </header>
      {q.isError ? (
        <div role="alert">
          {q.error.message}
          <Button onClick={() => void q.refetch()}>{msg("重试")}</Button>
        </div>
      ) : !q.data ? (
        <p role="status">{msg("正在读取")}</p>
      ) : (
        <RewardsPanel user={user} csrf={csrf} context={q.data} />
      )}
      {!q.isError && <WalletHistory user={user} />}
    </div>
  );
}
type Entry = {
  id: string;
  kind: "source" | "reserve" | "refund" | "alert_reserve" | "alert_release" | "alert_refund";
  reference: string;
  delta_minor: number;
  reason: string;
  created_at: string;
};
function WalletHistory({ user }: { user: string }) {
  const [before, setBefore] = useState("");
  const client = useQueryClient();
  const q = useQuery({
    queryKey: ["exchange", "wallet", user, before],
    queryFn: ({ signal }) => getWalletEntries(before, signal),
  });
  useEffect(() => {
    if (!q.data?.next_cursor) return;
    const next = q.data.next_cursor;
    void client.prefetchQuery({
      queryKey: ["exchange", "wallet", user, next],
      queryFn: ({ signal }) => getWalletEntries(next, signal),
      staleTime: 30_000,
    });
  }, [client, q.data?.next_cursor, user]);
  return (
    <Card className="exchange-card">
      <h2>{msg("果壳币流水")}</h2>
      {q.isError ? (
        <div role="alert">
          {q.error.message}
          <Button onClick={() => void q.refetch()}>{msg("重试")}</Button>
        </div>
      ) : !q.data ? (
        <p role="status">{msg("正在读取")}</p>
      ) : (
        <>
          {!q.data.items.length && (
            <p className="exchange-note">{msg("暂无流水")}</p>
          )}
          {q.data.items.map((e) => (
            <div className="exchange-order-row" key={e.id}>
              <div className="exchange-event-copy">
                <strong>
                  {e.kind.startsWith("alert_") ? (
                    <Link to="/sentry" className="exchange-alert-link">
                      {entryLabel(e.kind)}
                    </Link>
                  ) : entryLabel(e.kind)}
                </strong>
                <small>
                  {e.reason} · {formatDate(e.created_at)}
                </small>
                <small>{e.reference}</small>
              </div>
              <strong className="exchange-coin-delta">
                {e.delta_minor > 0 ? "+" : ""}
                {(e.delta_minor / 100).toLocaleString(getLocale())}{" "}
                {msg("币")}{" "}
              </strong>
            </div>
          ))}
          {(before || q.data.next_cursor) && (
            <div className="exchange-toolbar">
              <Button
                variant="outline"
                disabled={!before}
                onClick={() => setBefore("")}
              >
                {msg("返回最新")}{" "}
              </Button>
              <Button
                variant="outline"
                disabled={!q.data.next_cursor}
                onClick={() => setBefore(q.data!.next_cursor)}
              >
                {msg("更早流水")}{" "}
              </Button>
            </div>
          )}
        </>
      )}
    </Card>
  );
}
function entryLabel(kind: Entry["kind"]) {
  switch (kind) {
    case "source": return msg("PAP 发币 / 调整");
    case "reserve": return msg("奖励兑换");
    case "refund": return msg("取消退回");
    case "alert_reserve": return msg("预警暂占");
    case "alert_release": return msg("预警释放");
    case "alert_refund": return msg("预警退款");
  }
}
function getWalletEntries(before: string, signal?: AbortSignal) {
  return getData(
    `/api/v1/exchange/wallet?${new URLSearchParams({ before })}`,
    (v: unknown): v is { items: Entry[]; next_cursor: string } => {
      if (!v || typeof v !== "object") return false;
      const r = v as { items?: unknown; next_cursor?: unknown };
      return (
        typeof r.next_cursor === "string" &&
        Array.isArray(r.items) &&
        r.items.every(
          (x) =>
            x &&
            typeof x.id === "string" &&
            /^[1-9]\d*$/.test(x.id) &&
            ["source", "reserve", "refund", "alert_reserve", "alert_release", "alert_refund"].includes(x.kind) &&
            typeof x.reference === "string" &&
            Number.isSafeInteger(x.delta_minor) &&
            typeof x.reason === "string" &&
            typeof x.created_at === "string" &&
            Number.isFinite(Date.parse(x.created_at)),
        )
      );
    },
    signal,
  );
}

import { msg } from "@/lib/i18n";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, Clock3, RefreshCw, TriangleAlert } from "lucide-react";
import { useState } from "react";
import { IconAction } from "@/components/ui/icon-action";
import {
  getCharacterSync,
  refreshSync,
  resourceLabel,
  syncLabel,
  syncDate,
  reasonLabel,
  contractDetailLabel,
} from "./sync-api";
import "./sync.css";

export function SyncRows({
  characterID,
  userID,
  csrf,
}: {
  characterID: string;
  userID: string;
  csrf: string;
}) {
  const client = useQueryClient();
  const [notice, setNotice] = useState("");
  const query = useQuery({
    queryKey: ["eve", "sync", "character", userID, characterID],
    queryFn: ({ signal }) => getCharacterSync(characterID, signal),
    refetchInterval: (q) =>
      q.state.data?.targets.some((t) => ["queued", "running"].includes(t.state))
        ? 5_000
        : 30_000,
    refetchIntervalInBackground: false,
  });
  const update = useMutation({
    mutationFn: (resource: string) => refreshSync(characterID, csrf, resource),
    onSuccess: async (results) => {
      setNotice(
        results.some((r) => r.outcome === "deferred")
          ? msg("已安排，将在可更新时同步")
          : results.some((r) => r.outcome === "already_pending")
            ? msg("已在等待同步")
            : msg("已加入同步队列"),
      );
      await client.invalidateQueries({ queryKey: ["eve", "sync"] });
      await client.invalidateQueries({ queryKey: ["access", "me", userID] });
    },
  });
  if (query.isPending)
    return (
      <p className="muted" role="status">
        {msg("正在读取同步状态")}{" "}
      </p>
    );
  if (query.isError)
    return (
      <div className="sync-inline-error" role="alert">
        <span>{msg("同步状态读取失败")}</span>
        <IconAction
          label={msg("重试同步状态")}
          onClick={() => void query.refetch()}
        >
          <RefreshCw />
        </IconAction>
      </div>
    );
  return (
    <div className="sync-resource-list" aria-label={msg("角色同步状态")}>
      {query.data.targets.length === 0 && (
        <p className="muted">{msg("等待 EVE 授权")}</p>
      )}
      {query.data.targets.map((t) => {
        const waiting = ["queued", "running"].includes(t.state);
        const ok = t.state === "idle" && t.freshness === "fresh";
        const Icon = ok
          ? Check
          : t.state === "blocked" || t.state === "failed"
            ? TriangleAlert
            : Clock3;
        return (
          <div className="sync-resource-row" key={t.id}>
            <div className="sync-resource-copy">
              <strong>{resourceLabel(t.resource)}</strong>
              <span
                className={`sync-state ${ok ? "is-fresh" : t.state === "failed" || t.state === "blocked" ? "is-error" : ""}`}
              >
                <Icon size={14} aria-hidden="true" />
                {syncLabel(t)}
              </span>
              <time
                className="sync-time"
                dateTime={t.last_success_at ?? undefined}
              >
                {syncDate(t.last_success_at)}
              </time>
              {t.reason && (
                <span className="sync-reason">{reasonLabel(t.reason)}</span>
              )}
              {contractDetailLabel(t) && (
                <span className="sync-reason">{contractDetailLabel(t)}</span>
              )}
            </div>
            <IconAction
              label={msg("同步{0}", resourceLabel(t.resource))}
              disabled={
                !query.data.available ||
                update.isPending ||
                waiting ||
                t.state === "blocked"
              }
              aria-busy={update.isPending && update.variables === t.resource}
              onClick={() => {
                setNotice("");
                update.mutate(t.resource);
              }}
            >
              <RefreshCw
                aria-hidden="true"
                className={t.state === "running" ? "animate-spin" : undefined}
              />
            </IconAction>
          </div>
        );
      })}
      {!query.data.available && (
        <p className="sync-reason">{msg("同步服务未就绪")}</p>
      )}
      {update.error && (
        <p role="alert" className="login-error">
          {update.error.message}
        </p>
      )}
      {notice && (
        <p role="status" className="sync-reason">
          {notice}
        </p>
      )}
    </div>
  );
}

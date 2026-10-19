import { useQuery } from "@tanstack/react-query";
import { ExternalLink, RefreshCw } from "lucide-react";
import { getData } from "@/lib/http";
import { getLocale, msg } from "@/lib/i18n";
import { date } from "./api";
import "./battle-report.css";

type Team = { id: string; name: string; pilots: number; kills: number; losses: number; kills_isk: number; losses_isk: number; own: boolean };
export type BattleReport = { status: "pending" | "unavailable" | "ready"; summary?: { start: string; end: string; observed_at: string; systems: string[]; kill_count: number; total_isk: number; teams: Team[] } };
const object = (v: unknown): v is Record<string, unknown> => !!v && typeof v === "object" && !Array.isArray(v);
const amount = (v: unknown): v is number => typeof v === "number" && Number.isFinite(v) && v >= 0 && v <= 1e18;
const count = (v: unknown): v is number => amount(v) && Number.isSafeInteger(v);
const timestamp = (v: unknown): v is string => typeof v === "string" && Number.isFinite(Date.parse(v));
export function isBattleReport(v: unknown): v is BattleReport {
  if (!object(v) || !["ready", "pending", "unavailable"].includes(String(v.status))) return false;
  if (v.status !== "ready") return v.summary === undefined;
  const s = v.summary;
  return object(s) && timestamp(s.start) && timestamp(s.end) && timestamp(s.observed_at) && count(s.kill_count) && amount(s.total_isk) &&
    Array.isArray(s.systems) && s.systems.length > 0 && s.systems.every(x => typeof x === "string") &&
    Array.isArray(s.teams) && s.teams.length > 0 && s.teams.length <= 32 && s.teams.every(t => object(t) && typeof t.id === "string" && typeof t.name === "string" &&
      count(t.pilots) && count(t.kills) && count(t.losses) && amount(t.kills_isk) && amount(t.losses_isk) && typeof t.own === "boolean");
}
export function battleReportURL(km: string) {
  return /^[1-9]\d{0,18}$/.test(km) && BigInt(km) <= 9223372036854775807n ? `https://br.scers.cn/q?mode=expand&km=${km}` : "";
}
function isk(value: number) {
  const [divisor, suffix] = value >= 1e12 ? [1e12, "T"] : value >= 1e9 ? [1e9, "B"] : value >= 1e6 ? [1e6, "M"] : [1, ""];
  return `${(value / divisor).toLocaleString(getLocale(), { maximumFractionDigits: 2 })}${suffix}`;
}

export function BattleReportPanel({ killmailID, caseID, corporationID, characterID }: { killmailID: string; caseID?: string; corporationID?: string; characterID?: string }) {
  const link = battleReportURL(killmailID);
  const path = caseID ? `/api/v1/welfare/cases/${encodeURIComponent(caseID)}/battle-report` : `/api/v1/welfare/losses/battle-report?${new URLSearchParams({ corporation_id: corporationID || "", character_id: characterID || "", killmail_id: killmailID })}`;
  const q = useQuery({
    queryKey: ["welfare", "battle-report", path, getLocale()],
    queryFn: ({ signal }) => getData(path, isBattleReport, signal),
    enabled: !!link && !!(caseID || corporationID && characterID),
    staleTime: 60_000,
    retry: false,
    refetchOnWindowFocus: false,
    refetchInterval: query => query.state.data?.status === "pending" ? 5_000 : false,
  });
  if (!link) return null;
  const summary = q.data?.status === "ready" ? q.data.summary : undefined;
  const loading = q.isPending || q.data?.status === "pending";
  return <section className="welfare-battle-report" aria-label={msg("战报摘要")}>
    <header>
      <strong>{msg("战报摘要")}</strong>
      <a href={link} target="_blank" rel="noopener noreferrer">{msg("查看战报")}<ExternalLink size={14} aria-hidden="true" /></a>
    </header>
    {summary ? <>
      <div className="welfare-battle-facts">
        <span>{summary.systems.join(" · ")}</span>
        <time dateTime={summary.start}>{date(summary.start)} — {date(summary.end)}</time>
        <span>{summary.kill_count} KM · {isk(summary.total_isk)} ISK</span>
      </div>
      <div className="welfare-battle-teams">
        {summary.teams.map(team => <div className="welfare-battle-team" data-own={team.own} key={team.id}>
          <div className="welfare-battle-team-name"><strong>{team.name}</strong>{team.own && <span>{msg("本方")}</span>}</div>
          <dl>
            <div><dt>{msg("参战人数")}</dt><dd>{team.pilots.toLocaleString(getLocale())}</dd></div>
            <div><dt>{msg("击毁")}</dt><dd>{team.kills.toLocaleString(getLocale())}</dd></div>
            <div><dt>{msg("损失")}</dt><dd>{team.losses.toLocaleString(getLocale())}</dd></div>
            <div><dt>{msg("损失金额")}</dt><dd title={`${team.losses_isk.toLocaleString(getLocale())} ISK`}>{isk(team.losses_isk)} ISK</dd></div>
          </dl>
        </div>)}
      </div>
    </> : <div className="welfare-battle-state" role="status">
      <span>{msg(loading ? "正在生成战报摘要" : "战报摘要暂不可用")}</span>
      {!loading && <button type="button" onClick={() => void q.refetch()} disabled={q.isFetching}><RefreshCw size={14} aria-hidden="true" />{msg("重试")}</button>}
    </div>}
  </section>;
}

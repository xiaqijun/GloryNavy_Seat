import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { Building2, ChevronDown, Fuel, Layers3, MapPin, RefreshCw, ShieldCheck } from "lucide-react";
import { msg, getLocale } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { useSession } from "@/modules/identity";
import { Navigate } from "react-router-dom";
import * as api from "./api";
import "./structures.css";

const date = (v?: string) => (v ? new Date(v).toLocaleString(getLocale(), { hour12: false }) : "—");
type Filter = "all" | "upwell" | "pos";

export default function StructuresPage() {
  const session = useSession();
  if (session.isError) return <p role="alert">{session.error.message}</p>;
  if (!session.data) return <p role="status">{msg("正在读取")}</p>;
  if (!session.data.session) return <Navigate to="/login" replace />;
  return <Workspace />;
}

function Workspace() {
  const [filter, setFilter] = useState<Filter>("all");
  const q = useQuery({
    queryKey: ["structures", "overview"],
    queryFn: ({ signal }) => api.structures("", signal),
    staleTime: 60_000,
  });
  const items = q.data?.items ?? [];
  const filtered = filter === "all" ? items : items.filter((item) => item.kind === filter);
  const corporations = Array.from(filtered.reduce((groups, item) => {
    const group = groups.get(item.corporation_id) ?? { name: item.corporation_name, items: [] as api.Structure[] };
    group.items.push(item);
    groups.set(item.corporation_id, group);
    return groups;
  }, new Map<string, { name: string; items: api.Structure[] }>()));
  const counts = { all: items.length, upwell: items.filter((item) => item.kind === "upwell").length, pos: items.filter((item) => item.kind === "pos").length };
  return (
    <div className="structures-page">
      <header className="structures-heading">
        <div className="structures-title">
          <span className="structures-brand"><Building2 aria-hidden="true" /></span>
          <div><h1>{msg("建筑管理")}</h1><p>{msg("按类型查看军团建筑")}</p></div>
        </div>
        <Button variant="outline" disabled={q.isFetching} onClick={() => void q.refetch()}>
          <RefreshCw size={16} aria-hidden="true" /> {msg("刷新")}
        </Button>
      </header>
      {!q.isError && !q.isPending && items.length > 0 && <div className="structures-toolbar" aria-label={msg("建筑分类")}>
        <div className="structure-filters" role="tablist" aria-label={msg("建筑分类")}>
          {(["all", "upwell", "pos"] as Filter[]).map((key) => <button key={key} type="button" role="tab" aria-selected={filter === key} className={filter === key ? "is-active" : ""} onClick={() => setFilter(key)}>
            {msg(key === "all" ? "全部" : key === "upwell" ? "Upwell 建筑" : "POS")}<span>{counts[key]}</span>
          </button>)}
        </div>
        <span className="structures-total"><Layers3 size={15} aria-hidden="true" />{msg("共 {0} 个建筑", filtered.length)}</span>
      </div>}
      {q.isError && <div className="structures-feedback" role="alert"><p>{q.error.message}</p><Button variant="outline" onClick={() => void q.refetch()}>{msg("重试")}</Button></div>}
      {!q.isError && q.isPending && <p role="status">{msg("正在读取")}</p>}
      {!q.isError && !q.isPending && items.length === 0 && <div className="structures-empty"><Building2 size={32} aria-hidden="true" /><p>{msg("暂无可查看的建筑")}</p><span>{msg("需要角色拥有军团建筑读取权限，并完成 ESI 授权。")}</span></div>}
      {corporations.map(([corporationID, group]) => <section className="structure-corporation" key={corporationID}>
        <header className="structure-corporation-head"><div><h2>{group.name}</h2><span>{msg("军团 ID")} {corporationID}</span></div><span>{msg("最近同步")} {date(group.items.reduce((latest, item) => item.observed_at > latest ? item.observed_at : latest, ""))}</span></header>
        <div className="structures-grid">
          {group.items.map((item) => <article className="structure-card" key={`${item.kind}:${item.id}`}>
            <div className="structure-card-head"><div><span className="structure-kind">{item.kind === "pos" ? msg("POS") : msg("Upwell 建筑")}</span><h3>{item.name || msg("未命名建筑")}</h3></div><span className={`structure-state state-${item.state}`}>{item.state || msg("未知")}</span></div>
            <div className="structure-location"><MapPin size={16} aria-hidden="true" /><strong>{item.solar_system_name || (item.solar_system_id === "0" ? msg("未知星系") : item.solar_system_id)}</strong><span>{item.type_name || item.type_id}</span></div>
            <div className="structure-meta"><span>{msg("建筑 ID")} <b>{item.id}</b></span>{item.kind === "upwell" && <span>{msg("燃料到期")} <b>{date(item.fuel_expires)}</b></span>}</div>
            {item.kind === "pos" && <details className="structure-details"><summary><Fuel size={16} aria-hidden="true" /><span>{msg("燃料仓")}</span><b>{item.fuel?.length ?? 0}</b><ChevronDown size={16} aria-hidden="true" /></summary><div className="structure-detail-body">{item.fuel?.length ? item.fuel.map((f) => <span key={`${f.type_id}:${f.quantity}`}>{f.type_id} × {f.quantity}</span>) : <span>{msg("暂无燃料明细")}</span>}</div></details>}
            {item.services && item.services.length > 0 && <details className="structure-details"><summary><ShieldCheck size={16} aria-hidden="true" /><span>{msg("服务")}</span><b>{item.services.length}</b><ChevronDown size={16} aria-hidden="true" /></summary><div className="structure-detail-body">{item.services.map((s) => <span key={s.name}>{s.name} · {s.state}</span>)}</div></details>}
          </article>)}
        </div>
      </section>)}
      {!q.isError && !q.isPending && items.length > 0 && filtered.length === 0 && <div className="structures-empty"><Layers3 size={28} aria-hidden="true" /><p>{msg("此分类暂无建筑")}</p></div>}
    </div>
  );
}

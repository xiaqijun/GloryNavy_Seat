import { useQuery } from "@tanstack/react-query";
import { Building2, Fuel, RefreshCw, ShieldCheck } from "lucide-react";
import { msg, getLocale } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { useSession } from "@/modules/identity";
import { Navigate } from "react-router-dom";
import * as api from "./api";
import "./structures.css";

const date = (v?: string) => (v ? new Date(v).toLocaleString(getLocale(), { hour12: false }) : "—");

export default function StructuresPage() {
  const session = useSession();
  if (session.isError) return <p role="alert">{session.error.message}</p>;
  if (!session.data) return <p role="status">{msg("正在读取")}</p>;
  if (!session.data.session) return <Navigate to="/login" replace />;
  return <Workspace />;
}

function Workspace() {
  const q = useQuery({
    queryKey: ["structures", "overview"],
    queryFn: ({ signal }) => api.structures("", signal),
    staleTime: 60_000,
  });
  const items = q.data?.items ?? [];
  return (
    <div className="structures-page">
      <header className="structures-heading">
        <div className="structures-title">
          <span className="structures-brand"><Building2 aria-hidden="true" /></span>
          <div><h1>{msg("建筑管理")}</h1><p>{msg("查看军团 Upwell 建筑与 POS 的当前状态")}</p></div>
        </div>
        <Button variant="outline" disabled={q.isFetching} onClick={() => void q.refetch()}>
          <RefreshCw size={16} aria-hidden="true" /> {msg("刷新")}
        </Button>
      </header>
      {q.isError && <div className="structures-feedback" role="alert"><p>{q.error.message}</p><Button variant="outline" onClick={() => void q.refetch()}>{msg("重试")}</Button></div>}
      {!q.isError && q.isPending && <p role="status">{msg("正在读取")}</p>}
      {!q.isError && !q.isPending && items.length === 0 && <div className="structures-empty"><Building2 size={32} aria-hidden="true" /><p>{msg("暂无可查看的建筑")}</p><span>{msg("需要角色拥有军团建筑读取权限，并完成 ESI 授权。")}</span></div>}
      <div className="structures-grid">
        {items.map((item) => <article className="structure-card" key={`${item.kind}:${item.id}`}>
          <div className="structure-card-head"><div><span className="structure-kind">{item.corporation_name} · {item.kind === "pos" ? msg("POS") : msg("Upwell 建筑")}</span><h2>{item.name || msg("未命名建筑")}</h2></div><span className={`structure-state state-${item.state}`}>{item.state || msg("未知")}</span></div>
          <dl className="structure-facts"><div><dt>{msg("星系")}</dt><dd>{item.solar_system_name || item.solar_system_id}</dd></div><div><dt>{msg("建筑类型")}</dt><dd>{item.type_name || item.type_id}</dd></div><div><dt>{msg("建筑 ID")}</dt><dd>{item.id}</dd></div><div><dt>{msg("军团 ID")}</dt><dd>{item.corporation_id}</dd></div></dl>
          {item.kind === "pos" && <div className="structure-fuel"><Fuel size={17} aria-hidden="true" /><strong>{msg("燃料仓")}</strong>{item.fuel?.length ? <span>{item.fuel.map((f) => `${f.type_id} × ${f.quantity}`).join(" · ")}</span> : <span>{msg("暂无燃料明细")}</span>}</div>}
          {item.kind === "upwell" && <div className="structure-fuel"><Fuel size={17} aria-hidden="true" /><strong>{msg("燃料到期")}</strong><span>{date(item.fuel_expires)}</span></div>}
          {item.services && item.services.length > 0 && <div className="structure-services"><ShieldCheck size={17} aria-hidden="true" /><span>{item.services.map((s) => `${s.name} (${s.state})`).join(" · ")}</span></div>}
          <footer>{msg("观察时间")}：{date(item.observed_at)}</footer>
        </article>)}
      </div>
    </div>
  );
}

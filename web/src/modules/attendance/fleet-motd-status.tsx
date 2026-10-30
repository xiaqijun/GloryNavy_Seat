import { useQuery } from "@tanstack/react-query";
import { getData, apiFetch, APIError } from "@/lib/http";
import { msg } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { useToast } from "@/components/ui/toast-context";
import { useEffect, useRef, useState } from "react";

type Status = {state: "none" | "pending" | "preparing" | "sending" | "sent" | "failed" | "unknown" | "obsolete"; reason?: string; updated_at?: string};
const reasons:Record<string,string>={reauthorize:"需要舰队写入授权",rate_limited:"舰队接口请求受限",fleet_unavailable:"无法读取当前舰队",fleet_changed:"舰队或置顶内容已变化",permission_changed:"发起人的权限已变化",identity_changed:"来源角色已变化",motd_format_changed:"置顶内容格式已变化",interrupted:"发送结果待确认",confirmation_required:"发送结果待确认"};
const isStatus = (v: unknown): v is Status => !!v && typeof v === "object" && ["none","pending","preparing","sending","sent","failed","unknown","obsolete"].includes(String((v as {state?: unknown}).state));

export function FleetMOTDStatus({eventID,user,source,csrf,notify=false}:{eventID:string;user:string;source:string;csrf:string;notify?:boolean}) {
  const [error,setError]=useState("");
  const [busy,setBusy]=useState(false);
  const toast=useToast();
  const previous=useRef<{eventID:string;state:Status["state"];updatedAt?:string} | null>(null);
  const q=useQuery({queryKey:["attendance","fleet-motd",user,eventID],queryFn:({signal})=>getData(`/api/v1/attendance/events/${encodeURIComponent(eventID)}/fleet-motd`,isStatus,signal),refetchInterval: query => ["pending","preparing","sending"].includes(query.state.data?.state ?? "") ? 10000 : false});
  useEffect(()=>{
    if(!notify || !q.data) return;
    const current={eventID,state:q.data.state,updatedAt:q.data.updated_at};
    const old=previous.current;
    previous.current=current;
    if(old && old.eventID===eventID && current.state==="sent" && (old.state!=="sent" || (current.updatedAt && current.updatedAt!==old.updatedAt))) {
      toast.success(msg("舰队置顶已更新"));
    }
  },[eventID,notify,q.data,toast]);
  const authorize=async()=>{
    setError(""); setBusy(true);
    try {
      const r=await apiFetch(`/api/v1/eve/characters/${encodeURIComponent(source)}/reauthorize?purpose=fleet-write`,{method:"POST",credentials:"same-origin",headers:{"X-CSRF-Token":csrf}});
      const p=await r.json().catch(()=>null);
      if(!r.ok || typeof p?.data?.url!=="string") throw new APIError(p?.error?.message ?? msg("授权失败"),r.status);
      window.location.assign(p.data.url);
    } catch(e) { setError(e instanceof Error ? e.message : msg("授权失败")); setBusy(false); }
  };
  if(q.isError) return <p role="alert">{msg("置顶状态读取失败")}</p>;
  if(!q.data || q.data.state==="none" || q.data.state==="sent") return null;
  const state=q.data.state;
  const label=state==="failed" ? msg("舰队置顶更新失败") : state==="unknown" ? msg("舰队置顶更新结果待确认") : state==="obsolete" ? msg("舰队置顶更新已失效") : msg("正在更新舰队置顶");
  return <div className="attendance-toolbar"><span role="status">{label}{q.data.reason && reasons[q.data.reason] ? `（${msg(reasons[q.data.reason])}）` : ""}</span>{state==="unknown" && <span>{msg("请先核对游戏内置顶，再决定是否重新同步。")}</span>}{state==="failed" && q.data.reason==="reauthorize" && source && <><Button variant="outline" disabled={busy} onClick={()=>void authorize()}>{msg("授权舰队置顶")}</Button><span>{msg("授权后再次同步舰队置顶或点名并发放 PAP。")}</span></>}{error && <span role="alert">{error}</span>}</div>;
}

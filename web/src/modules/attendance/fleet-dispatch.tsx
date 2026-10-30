import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch, APIError, getData } from "@/lib/http";
import { msg } from "@/lib/i18n";
import { Button } from "@/components/ui/button";

type Channel = "qq" | "eve_mail" | "email";
type Count = { channel: Channel; state: string; reason?: string; count: number };
type Dispatch = {id:string;counts:Count[]};
const isDispatch=(v:unknown):v is Dispatch=>{
  if(!v || typeof v!=="object")return false;
  const d=v as Record<string,unknown>;
  return typeof d.id==="string" && Array.isArray(d.counts) && d.counts.every(x=>x && typeof x==="object" && ["qq","eve_mail","email"].includes(x.channel) && typeof x.state==="string" && Number.isSafeInteger(x.count) && x.count>=0);
};
const isEnvelope=(v:unknown):v is {dispatch:Dispatch|null}=>!!v && typeof v==="object" && ((v as {dispatch?:unknown}).dispatch===null || isDispatch((v as {dispatch?:unknown}).dispatch));
const label:Record<Channel,string>={qq:"QQ群",eve_mail:"游戏邮件",email:"Email"};
const states:Record<string,string>={pending:"待发送",preparing:"准备发送",sending:"发送中",sent:"已发送",failed:"发送失败",unknown:"结果待确认",skipped:"未发送"};
const reasons:Record<string,string>={email_unavailable:"邮箱未验证或未启用",no_bound_members:"没有本站已绑定成员",account_merged:"账号已合并",membership_changed:"成员军团归属已变化",permission_changed:"发起人的权限已变化",activity_closed:"活动已结束",group_changed:"群配置已变化",group_unavailable:"军团提醒群不可用",bot_unavailable:"QQ 机器人不可用",sender_unavailable:"发件角色不可用",context_changed:"发件配置已变化",service_unavailable:"发送服务不可用",rate_limited:"发送受限",interrupted:"发送结果待确认",confirmation_required:"发送结果待确认"};

export function FleetDispatch({eventID,user,csrf,revision,ready,onBusyChange}:{eventID:string;user:string;csrf:string;revision:number;ready:boolean;onBusyChange:(busy:boolean)=>void}) {
  const client=useQueryClient();
  const [channels,setChannels]=useState<Channel[]>([]);
  const [key,setKey]=useState(()=>crypto.randomUUID());
  const path=`/api/v1/attendance/events/${encodeURIComponent(eventID)}/fleet-dispatch`;
  const query=useQuery({queryKey:["attendance","fleet-dispatch",user,eventID],queryFn:({signal})=>getData(path,isEnvelope,signal),refetchInterval:q=>q.state.data?.dispatch?.counts.some(c=>["pending","preparing","sending"].includes(c.state)) ? 10000 : false});
  const send=useMutation({mutationFn:async()=>{
    const r=await apiFetch(path,{method:"POST",credentials:"same-origin",headers:{"Content-Type":"application/json","X-CSRF-Token":csrf},body:JSON.stringify({revision,request_key:key,channels})});
    const p=await r.json().catch(()=>null);if(!r.ok)throw new APIError(p?.error?.message ?? msg("发送失败"),r.status,p?.request_id);
    if(!isDispatch(p?.data))throw new APIError(msg("响应格式异常，请刷新核对结果"),r.status);
    return p.data;
  },onMutate:()=>onBusyChange(true),onSuccess:dispatch=>{client.setQueryData(["attendance","fleet-dispatch",user,eventID],{dispatch});setKey(crypto.randomUUID());},onSettled:()=>onBusyChange(false)});
  const toggle=(channel:Channel)=>{setChannels(current=>current.includes(channel)?current.filter(c=>c!==channel):[...current,channel]);setKey(crypto.randomUUID());};
  return <section className="fleet-dispatch">
    <strong>{msg("推送公告")}</strong>
    <div className="fleet-dispatch-options">{(["qq","eve_mail","email"] as const).map(channel=><label key={channel}><input type="checkbox" checked={channels.includes(channel)} onChange={()=>toggle(channel)} />{msg(label[channel])}</label>)}</div>
    {channels.includes("qq") && <p>{msg("QQ群使用当前军团配置的提醒群。")}</p>}
    {channels.includes("email") && <p>{msg("普通邮箱仅发送给已绑定本站并验证邮箱的军团成员。")}</p>}
    <Button variant="outline" disabled={!ready || channels.length===0 || send.isPending} onClick={()=>send.mutate()}>{send.isPending?msg("正在提交"):msg("推送")}</Button>
    {send.isError && <p role="alert">{send.error.message}</p>}
    {query.isError && <p role="alert">{msg("推送记录读取失败")}</p>}
    {query.data?.dispatch && <div role="status" className="fleet-dispatch-results">{query.data.dispatch.counts.map(c=><span key={`${c.channel}-${c.state}-${c.reason ?? ""}`}>{msg(label[c.channel])} · {msg(states[c.state] ?? "未知")}{c.reason !== "no_bound_members" && ` ${c.count}`}{c.reason && (reasons[c.reason] || c.reason.startsWith("qq_rejected_")) ? `（${msg(reasons[c.reason] ?? "QQ 平台拒绝发送")}）` : ""}</span>)}</div>}
  </section>;
}

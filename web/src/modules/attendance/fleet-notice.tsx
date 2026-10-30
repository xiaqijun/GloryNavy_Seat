import { useRef, useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch, APIError, getData } from "@/lib/http";
import { getLocale, msg } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { FleetDispatch } from "./fleet-dispatch";

type Notice = { solar_system: string; doctrine: string; meeting_point: string; commander: string; details: string; revision: number; updated_at?: string };
const empty: Notice = { solar_system: "", doctrine: "", meeting_point: "", commander: "", details: "", revision: 0 };
const isNotice = (v: unknown): v is Notice => {
  if (!v || typeof v !== "object") return false;
  const n = v as Record<string, unknown>;
  return ["solar_system", "doctrine", "meeting_point", "commander", "details"].every(k => typeof n[k] === "string") && Number.isSafeInteger(n.revision) && Number(n.revision) >= 0;
};
const isEnvelope = (v: unknown): v is {notice: Notice | null} => !!v && typeof v === "object" && ((v as {notice?: unknown}).notice === null || isNotice((v as {notice?: unknown}).notice));

export function FleetNoticeEditor({eventID, eventTitle, eventStartsAt, eventVersion, open, source, csrf, user}: {eventID: string; eventTitle:string; eventStartsAt:string; eventVersion:string; open:boolean; source:string; csrf: string; user: string}) {
  const client = useQueryClient();
  const [edited, setEdited] = useState<Notice | null>(null);
  const [key, setKey] = useState(() => crypto.randomUUID());
  const syncKey = useRef<{source:string;version:string;revision:number;key:string} | null>(null);
  const path = `/api/v1/attendance/events/${encodeURIComponent(eventID)}/fleet-notice`;
  const query = useQuery({queryKey:["attendance", "fleet-notice", user, eventID], queryFn:({signal}) => getData(path, isEnvelope, signal)});
  const draft = edited ?? query.data?.notice ?? empty;
  const save = useMutation({mutationFn: async () => {
    const response = await apiFetch(path, {method:"PUT", credentials:"same-origin", headers:{"Content-Type":"application/json", "X-CSRF-Token":csrf}, body:JSON.stringify({...draft, updated_at: undefined, request_key:key})});
    const payload = await response.json().catch(() => null);
    if (!response.ok) throw new APIError(payload?.error?.message ?? msg("保存失败，请重试"), response.status, payload?.request_id);
    if (!isNotice(payload?.data)) throw new APIError(msg("响应格式异常，请刷新核对结果"), response.status);
    return payload.data;
  }, onSuccess: notice => { client.setQueryData(["attendance", "fleet-notice", user, eventID], {notice}); setEdited(null); setKey(crypto.randomUUID()); }});
  const sync = useMutation({mutationFn: async () => {
    if(!syncKey.current || syncKey.current.source!==source || syncKey.current.version!==eventVersion || syncKey.current.revision!==draft.revision) syncKey.current={source,version:eventVersion,revision:draft.revision,key:crypto.randomUUID()};
    const response = await apiFetch(`/api/v1/attendance/events/${encodeURIComponent(eventID)}/fleet-motd`, {method:"POST",credentials:"same-origin",headers:{"Content-Type":"application/json","X-CSRF-Token":csrf},body:JSON.stringify({source_character_id:source,version:eventVersion,revision:draft.revision,request_key:syncKey.current.key})});
    const payload=await response.json().catch(()=>null);
    if(!response.ok) throw new APIError(payload?.error?.message ?? msg("同步失败"),response.status,payload?.request_id);
    if(payload?.data?.queued!==true) throw new APIError(msg("响应格式异常，请刷新核对结果"),response.status);
  },onSuccess:()=>{syncKey.current=null;void client.invalidateQueries({queryKey:["attendance","fleet-motd",user,eventID]});}});
  const update = (field: keyof Notice, value: string) => { setEdited({...draft, [field]:value}); setKey(crypto.randomUUID()); };
  const submit = (event: FormEvent) => { event.preventDefault(); save.mutate(); };
  if (query.isPending) return <p role="status">{msg("正在读取")}</p>;
  if (query.isError) return <p role="alert">{query.error.message}</p>;
  return <Card className="attendance-card"><form className="attendance-form" onSubmit={submit}>
    <h2>{msg("舰队公告")}</h2>
    <label>{msg("星系")}<input value={draft.solar_system} maxLength={120} required onChange={e=>update("solar_system", e.target.value)} /></label>
    <label>{msg("建制")}<input value={draft.doctrine} maxLength={500} required onChange={e=>update("doctrine", e.target.value)} /></label>
    <label>{msg("集结地点")}<input value={draft.meeting_point} maxLength={300} onChange={e=>update("meeting_point", e.target.value)} /></label>
    <label>{msg("指挥")}<input value={draft.commander} maxLength={120} onChange={e=>update("commander", e.target.value)} /></label>
    <label>{msg("补充信息")}<textarea value={draft.details} maxLength={3000} onChange={e=>update("details", e.target.value)} /></label>
    <div className="fleet-notice-preview"><strong>{msg("公告预览")}</strong><div>{eventTitle}</div><div>{msg("时间")}：{new Intl.DateTimeFormat(getLocale(),{timeZone:"Asia/Shanghai",year:"numeric",month:"2-digit",day:"2-digit",hour:"2-digit",minute:"2-digit",hour12:false}).format(new Date(eventStartsAt))}</div><div>{msg("星系")}：{draft.solar_system || "—"}</div><div>{msg("建制")}：{draft.doctrine || "—"}</div>{draft.meeting_point && <div>{msg("集结地点")}：{draft.meeting_point}</div>}{draft.commander && <div>{msg("指挥")}：{draft.commander}</div>}{draft.details && <div className="fleet-notice-details">{draft.details}</div>}</div>
    <Button type="submit" disabled={save.isPending}>{save.isPending ? msg("正在保存") : msg("保存公告")}</Button>
    {save.isSuccess && <p role="status">{msg("已保存")}</p>}
    {save.isError && <p role="alert">{save.error.message}</p>}
  </form><div className="attendance-toolbar"><Button variant="outline" disabled={!open || !source || edited!==null || !query.data?.notice || save.isPending || sync.isPending} onClick={()=>sync.mutate()}>{sync.isPending ? msg("正在同步") : msg("同步舰队置顶")}</Button>{sync.isError && <span role="alert">{sync.error.message}</span>}</div><FleetDispatch eventID={eventID} user={user} csrf={csrf} revision={draft.revision} ready={edited===null && !!query.data?.notice && !save.isPending} /></Card>;
}

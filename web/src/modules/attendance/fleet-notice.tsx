import { useId, useRef, useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch, APIError, getData } from "@/lib/http";
import { getLocale, msg } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Modal } from "@/components/ui/dialog";
import { ClipboardList } from "lucide-react";
import { FleetDispatch } from "./fleet-dispatch";
import { FleetMOTDStatus } from "./fleet-motd-status";

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
  const formId = useId();
  const [visible, setVisible] = useState(false);
  const [dispatchBusy, setDispatchBusy] = useState(false);
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
  const update = (field: keyof Notice, value: string) => { setEdited({...draft, [field]:value}); setKey(crypto.randomUUID()); save.reset(); };
  const submit = (event: FormEvent) => { event.preventDefault(); save.mutate(); };
  const busy = save.isPending || sync.isPending || dispatchBusy;
  return <>
    <div className="attendance-toolbar"><Button variant="outline" onClick={() => setVisible(true)}><ClipboardList />{msg("舰队公告")}</Button></div>
    {visible && <Modal title={msg("舰队公告")} close={() => setVisible(false)} busy={busy} footer={<>
      <Button variant="outline" onClick={() => setVisible(false)} disabled={busy}>{msg("关闭")}</Button>
      {!query.isPending && !query.isError && <Button type="submit" form={formId} disabled={busy}>{save.isPending ? msg("正在保存") : msg("保存公告")}</Button>}
    </>}>
    {query.isPending ? <p role="status">{msg("正在读取")}</p> : query.isError ? <p role="alert">{query.error.message}</p> : <>
    <form id={formId} className="attendance-form" onSubmit={submit}>
    <fieldset className="fleet-notice-fields" disabled={busy}>
    <label>{msg("星系")}<input value={draft.solar_system} maxLength={120} required onChange={e=>update("solar_system", e.target.value)} /></label>
    <label>{msg("建制")}<input value={draft.doctrine} maxLength={500} required onChange={e=>update("doctrine", e.target.value)} /></label>
    <label>{msg("集结地点")}<input value={draft.meeting_point} maxLength={300} onChange={e=>update("meeting_point", e.target.value)} /></label>
    <label>{msg("指挥")}<input value={draft.commander} maxLength={120} onChange={e=>update("commander", e.target.value)} /></label>
    <label>{msg("补充信息")}<textarea value={draft.details} maxLength={3000} onChange={e=>update("details", e.target.value)} /></label>
    </fieldset>
    <div className="fleet-notice-preview"><strong>{msg("公告预览")}</strong><div>{eventTitle}</div><div>{msg("时间")}：{new Intl.DateTimeFormat(getLocale(),{timeZone:"Asia/Shanghai",year:"numeric",month:"2-digit",day:"2-digit",hour:"2-digit",minute:"2-digit",hour12:false}).format(new Date(eventStartsAt))}</div><div>{msg("星系")}：{draft.solar_system || "—"}</div><div>{msg("建制")}：{draft.doctrine || "—"}</div>{draft.meeting_point && <div>{msg("集结地点")}：{draft.meeting_point}</div>}{draft.commander && <div>{msg("指挥")}：{draft.commander}</div>}{draft.details && <div className="fleet-notice-details">{draft.details}</div>}</div>
    {save.isSuccess && <p role="status">{msg("已保存")}</p>}
    {save.isError && <p role="alert">{save.error.message}</p>}
    </form><div className="attendance-toolbar fleet-notice-sync"><Button variant="outline" disabled={!open || !source || edited!==null || !query.data?.notice || busy} onClick={()=>sync.mutate()}>{sync.isPending ? msg("正在同步") : msg("同步舰队置顶")}</Button>{sync.isError && <span role="alert">{sync.error.message}</span>}</div><FleetMOTDStatus eventID={eventID} source={source} csrf={csrf} user={user} /><FleetDispatch eventID={eventID} user={user} csrf={csrf} revision={draft.revision} ready={edited===null && !!query.data?.notice && !busy} onBusyChange={setDispatchBusy} />
    </>}
    </Modal>}
  </>;
}

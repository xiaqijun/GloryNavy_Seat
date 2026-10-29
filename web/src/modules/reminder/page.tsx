import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useSearchParams } from "react-router-dom";
import { RefreshCw, Save, Mail } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { msg, getLocale } from "@/lib/i18n";
import { useSession } from "@/modules/identity";
import { getAccountAccess } from "@/modules/access/api";
import { getModuleCatalog } from "@/app/catalog";
import * as api from "./api";
import { QQStructure } from "./qq-structure";
import { EmailSettings } from "./email-settings";
import "./reminder.css";

export default function ReminderPage() {
  const current = useSession().data?.session;
  const user = current?.user_id ?? "";
  const modules = useQuery({ queryKey: ["host", "modules"], queryFn: ({ signal }) => getModuleCatalog(signal), enabled: !!user });
  const qqAvailable = modules.data?.some(module => module.id === "community") && modules.data?.some(module => module.id === "structures");
  const access = useQuery({ queryKey: ["access", "me", user], queryFn: ({ signal }) => getAccountAccess(signal), enabled: !!user });
  const config = useQuery({ queryKey: ["reminder", "settings", user], queryFn: ({ signal }) => api.settings(signal), enabled: access.data?.administrator === true });
  return <div className="reminder-page"><div className="page-heading"><h1>{msg("提醒")}</h1></div>
    {access.error ? <p role="alert">{access.error.message}</p> : access.data && !access.data.administrator ? <p role="alert">{msg("仅管理员可访问")}</p> : access.data?.administrator && current ? <>{qqAvailable && <QQStructure user={user} csrf={current.csrf_token} />}<EmailSettings user={user} csrf={current.csrf_token} />{config.error ? <div role="alert"><p>{config.error.message}</p><Button onClick={() => void config.refetch()}>{msg("重试")}</Button></div> : config.data ? <><Editor key={`${user}:${config.data.revision}`} initial={config.data} user={user} csrf={current.csrf_token} /><Records events={config.data.available_events} user={user} csrf={current.csrf_token} /></> : <p role="status">{msg("正在加载")}</p>}</> : <p role="status">{msg("正在加载")}</p>}
  </div>;
}
function Editor({ initial, user, csrf }: { initial: api.Settings; user: string; csrf: string }) {
  const [sender, setSender] = useState(initial.sender_id === "0" ? "" : initial.sender_id);
  const [enabled, setEnabled] = useState(initial.enabled);
  const [events, setEvents] = useState(initial.events);
  const [params, setParams] = useSearchParams();
  const client = useQueryClient();
  const save = useMutation({ mutationFn: () => api.save({ sender_id: sender || "0", enabled, events, revision: initial.revision }, csrf), onSuccess: v => client.setQueryData(["reminder", "settings", user], v) });
  const authorize = useMutation({ mutationFn: () => api.authorize(sender, csrf) });
  const own = initial.characters.find(c => c.id === sender);
  const dirty = [...events].sort().join() !== [...initial.events].sort().join() || enabled !== initial.enabled || (sender || "0") !== initial.sender_id;
  const busy = save.isPending || authorize.isPending;
  const callbackError = params.get("error");
  return <Card><CardContent className="reminder-config">
    <div className="reminder-title"><h2><Mail size={18} aria-hidden="true" />{msg("游戏内邮件")}</h2><span className="muted">{initial.enabled ? msg("已启用") : msg("已停用")}</span></div>
    <div className="reminder-controls"><div className="reminder-sender"><label htmlFor="reminder-sender">{msg("发件角色")}</label><select id="reminder-sender" value={sender} disabled={busy} onChange={e => { setSender(e.target.value); setEnabled(false); }}><option value="">{msg("选择角色")}</option>{initial.sender_id !== "0" && !initial.characters.some(c => c.id === initial.sender_id) && <option value={initial.sender_id}>{initial.sender_name || initial.sender_id}</option>}{initial.characters.map(c => <option value={c.id} key={c.id}>{c.name}</option>)}</select></div>
      <Button variant="outline" disabled={busy || !own} onClick={() => authorize.mutate()}>{msg("授权发信")}</Button>
      <label className="reminder-toggle"><input type="checkbox" checked={enabled} disabled={busy || (!enabled && !own?.ready)} onChange={e => setEnabled(e.target.checked)} />{msg("启用推送")}</label>
      <Button disabled={busy || !dirty || (enabled && (!sender || events.length === 0))} onClick={() => save.mutate()}><Save aria-hidden="true" />{save.isPending ? msg("正在保存") : msg("保存")}</Button>
    </div>
    <fieldset className="reminder-events" disabled={busy}><legend>{msg("提醒类型")}</legend><div className="reminder-event-actions"><Button variant="outline" onClick={() => setEvents(initial.available_events.map(e => e.id))}>{msg("全选")}</Button><Button variant="outline" onClick={() => setEvents([])}>{msg("清空")}</Button></div><div className="reminder-event-grid">{initial.available_events.map(event => <label className="reminder-toggle" key={event.id}><input type="checkbox" checked={events.includes(event.id)} onChange={e => setEvents(v => e.target.checked ? [...v, event.id] : v.filter(id => id !== event.id))} />{msg(event.name)}</label>)}</div></fieldset>
    {sender && (own ? !own.ready : initial.status !== "ready") && <p role="status">{msg("需要授权发信")}</p>}
    {(save.error || authorize.error) && <div role="alert"><p>{(save.error || authorize.error)?.message}</p>{save.error && <Button variant="outline" onClick={() => void client.invalidateQueries({ queryKey: ["reminder", "settings", user] })}>{msg("刷新配置")}</Button>}</div>}
    {callbackError && <div role="alert"><p>{callbackError === "wrong_character" ? msg("授权角色不符，请在 EVE 页面选择 {0}", own?.name || initial.sender_name || sender) : callbackError === "session_expired" ? msg("登录已失效，请重新登录后授权发信") : callbackError === "authorization_required" ? msg("缺少发信权限，请确认 EVE 应用权限后重新授权") : callbackError === "forbidden" ? msg("当前账号无权授权发信") : callbackError === "cancelled" ? msg("授权已取消") : msg("发信授权未完成，请重试")}</p><Button variant="outline" onClick={() => setParams({})}>{msg("关闭")}</Button></div>}
  </CardContent></Card>;
}
const states: Record<string, string> = { sending: "发送中", sent: "已发送", failed: "发送失败", unknown: "结果待确认", retry_requested: "等待重试" };
const reasons: Record<string, string> = { context_changed: "收件人或待办已变化", reauthorize: "需要重新授权", missing_scope: "需要授权发信", identity_changed: "角色授权已变化", credential_unavailable: "角色授权不可用", service_unavailable: "邮件服务不可用", rate_limited: "发送受限", mail_rejected: "邮件被拒绝", confirmation_required: "结果待确认", interrupted: "发送中断" };
function Records({ user, csrf, events }: { user: string; csrf: string; events: api.ReminderEvent[] }) {
  const [pages, setPages] = useState<string[]>([""]);
  const before = pages.at(-1) ?? "";
  const client = useQueryClient();
  const records = useQuery({ queryKey: ["reminder", "deliveries", user, before], queryFn: ({ signal }) => api.deliveries(before, signal), refetchInterval: 30000 });
  const retry = useMutation({ mutationFn: (id: string) => api.retry(id, csrf), onSuccess: () => client.invalidateQueries({ queryKey: ["reminder", "deliveries", user] }) });
  return <Card><CardContent className="reminder-records"><div className="reminder-title"><h2>{msg("发送记录")}</h2><Button variant="outline" disabled={records.isFetching} onClick={() => void records.refetch()} aria-label={msg("刷新发送记录")}><RefreshCw aria-hidden="true" /></Button></div>
    {records.error ? <p role="alert">{records.error.message}</p> : records.isPending ? <p role="status">{msg("正在加载")}</p> : records.data.length === 0 ? <p className="muted">{msg("暂无发送记录")}</p> : <div className="reminder-table-scroll"><table className="reminder-table"><thead><tr><th>{msg("时间")}</th><th>{msg("收件人")}</th><th>{msg("提醒类型")}</th><th>{msg("数量")}</th><th>{msg("状态")}</th><th>{msg("操作")}</th></tr></thead><tbody>{records.data.map(d => <tr key={d.id}><td><time dateTime={d.created_at}>{new Date(d.created_at).toLocaleString(getLocale())}</time></td><td>{d.recipient_name || d.recipient_id}</td><td>{msg(events.find(e => e.id === d.event)?.name ?? "提醒")}</td><td>{d.notice_count}</td><td><span>{msg(states[d.state] ?? "结果待确认")}</span>{d.reason && reasons[d.reason] !== states[d.state] && <span className="reminder-reason">{msg(reasons[d.reason] ?? "邮件服务不可用")}</span>}</td><td>{d.state === "failed" ? <Button variant="outline" disabled={retry.isPending} onClick={() => retry.mutate(d.id)} aria-label={msg("重试发给 {0} 的提醒", d.recipient_name || d.recipient_id)}>{msg("重试")}</Button> : "—"}</td></tr>)}</tbody></table></div>}
    {retry.error && <p role="alert">{retry.error.message}</p>}
    {(pages.length > 1 || records.data?.length === 50) && <div className="reminder-pagination"><Button variant="outline" disabled={pages.length === 1 || records.isFetching} onClick={() => setPages(p => p.slice(0, -1))}>{msg("上一页")}</Button><Button variant="outline" disabled={records.data?.length !== 50 || records.isFetching} onClick={() => setPages(p => [...p, records.data?.at(-1)?.id ?? ""])}>{msg("下一页")}</Button></div>}
  </CardContent></Card>;
}

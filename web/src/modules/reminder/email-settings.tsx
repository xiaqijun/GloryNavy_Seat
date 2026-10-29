import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { getLocale, msg } from "@/lib/i18n";
import * as api from "./email-api";

export function EmailSettings({ user, csrf }: { user: string; csrf: string }) {
  const q = useQuery({ queryKey: ["reminder", "email-settings", user], queryFn: ({ signal }) => api.settings(signal) });
  if (q.error) return <p role="alert">{q.error.message}</p>;
  if (!q.data) return <p role="status">{msg("正在加载")}</p>;
  return <Editor key={`${user}:${q.data.revision}`} initial={q.data} user={user} csrf={csrf} />;
}
function Editor({ initial, user, csrf }: { initial: api.EmailSettings; user: string; csrf: string }) {
  const [host, setHost] = useState(initial.host); const [port, setPort] = useState(initial.port); const [implicit, setImplicit] = useState(initial.implicit_tls);
  const [username, setUsername] = useState(initial.username); const [sender, setSender] = useState(initial.sender); const [secret, setSecret] = useState(""); const [enabled, setEnabled] = useState(initial.enabled); const [events, setEvents] = useState(initial.events);
  const client = useQueryClient(); const save = useMutation({ mutationFn: () => api.saveSettings({ enabled, host: host.trim(), port, implicit_tls: implicit, username: username.trim(), sender: sender.trim(), secret, events, revision: initial.revision }, csrf), onSuccess: v => client.setQueryData(["reminder", "email-settings", user], v) });
  const deliveries = useQuery({ queryKey: ["reminder", "email-deliveries", user], queryFn: ({ signal }) => api.deliveries(signal), refetchInterval: 30000 });
  const retry = useMutation({ mutationFn: (id: string) => api.retry(id, csrf), onSuccess: () => client.invalidateQueries({ queryKey: ["reminder", "email-deliveries", user] }) });
  return <Card><CardContent className="reminder-config"><div className="reminder-title"><h2>{msg("邮箱推送")}</h2><span className="muted">{initial.enabled ? msg("已启用") : msg("已停用")}</span></div>
    <div className="reminder-controls"><div className="reminder-sender"><label htmlFor="smtp-host">{msg("SMTP 服务器")}</label><input id="smtp-host" value={host} onChange={e => setHost(e.target.value)} /></div><div className="reminder-sender"><label htmlFor="smtp-port">{msg("端口")}</label><input id="smtp-port" type="number" min="1" max="65535" value={port} onChange={e => setPort(Number(e.target.value))} /></div><div className="reminder-sender"><label htmlFor="smtp-tls">{msg("加密方式")}</label><select id="smtp-tls" value={implicit ? "tls" : "starttls"} onChange={e => setImplicit(e.target.value === "tls")}><option value="tls">TLS</option><option value="starttls">STARTTLS</option></select></div></div>
    <div className="reminder-controls"><div className="reminder-sender"><label htmlFor="smtp-user">{msg("发件账号")}</label><input id="smtp-user" value={username} autoComplete="off" onChange={e => setUsername(e.target.value)} /></div><div className="reminder-sender"><label htmlFor="smtp-from">{msg("发件邮箱")}</label><input id="smtp-from" type="email" value={sender} onChange={e => setSender(e.target.value)} /></div><div className="reminder-sender"><label htmlFor="smtp-secret">{msg("SMTP 授权码")}</label><input id="smtp-secret" type="password" value={secret} autoComplete="new-password" placeholder={initial.secret_configured ? msg("已配置，留空保持不变") : ""} onChange={e => setSecret(e.target.value)} /></div></div>
    <fieldset className="reminder-events"><legend>{msg("提醒类型")}</legend><div className="reminder-event-actions"><Button variant="outline" onClick={() => setEvents(initial.available_events.map(e => e.id))}>{msg("全选")}</Button><Button variant="outline" onClick={() => setEvents([])}>{msg("清空")}</Button></div><div className="reminder-event-grid">{initial.available_events.map(e => <label className="reminder-toggle" key={e.id}><input type="checkbox" checked={events.includes(e.id)} onChange={ev => setEvents(v => ev.target.checked ? [...v, e.id] : v.filter(id => id !== e.id))} />{msg(e.name)}</label>)}</div></fieldset>
    <div className="reminder-controls"><label className="reminder-toggle"><input type="checkbox" checked={enabled} onChange={e => setEnabled(e.target.checked)} />{msg("启用推送")}</label><Button disabled={save.isPending} onClick={() => save.mutate()}>{msg("保存")}</Button></div>
    {save.error && <p role="alert">{save.error.message}</p>}
    <div className="reminder-title"><h3>{msg("邮箱发送记录")}</h3></div>{deliveries.error ? <p role="alert">{deliveries.error.message}</p> : deliveries.data?.length ? <div className="reminder-table-scroll qq-structure-records"><table className="reminder-table"><thead><tr><th>{msg("时间")}</th><th>{msg("收件人")}</th><th>{msg("提醒类型")}</th><th>{msg("状态")}</th><th>{msg("操作")}</th></tr></thead><tbody>{deliveries.data.map(d => <tr key={d.id}><td data-label={msg("时间")}>{new Date(d.created_at).toLocaleString(getLocale())}</td><td data-label={msg("收件人")}>{d.recipient}</td><td data-label={msg("提醒类型")}>{msg(initial.available_events.find(e => e.id === d.event)?.name ?? "提醒")}</td><td data-label={msg("状态")}>{msg(({ sent:"已发送", failed:"发送失败", sending:"发送中", unknown:"结果待确认", retry_requested:"等待重试" } as Record<string,string>)[d.state] ?? "结果待确认")}</td><td data-label={msg("操作")}>{d.state === "failed" ? <Button variant="outline" disabled={retry.isPending} onClick={() => retry.mutate(d.id)} aria-label={msg("重试发给 {0} 的提醒", d.recipient)}>{msg("重试")}</Button> : "—"}</td></tr>)}</tbody></table></div> : <p className="muted">{msg("暂无发送记录")}</p>}
    {retry.error && <p role="alert">{retry.error.message}</p>}
  </CardContent></Card>;
}

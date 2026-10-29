import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { msg } from "@/lib/i18n";
import * as api from "./email-api";

export function EmailAccount({ user, csrf }: { user: string; csrf: string }) {
  const [address, setAddress] = useState(""); const [code, setCode] = useState(""); const [sent, setSent] = useState(false);
  const client = useQueryClient(); const key = ["reminder", "email-account", user];
  const q = useQuery({ queryKey: key, queryFn: ({ signal }) => api.account(signal) });
  const challenge = useMutation({ mutationFn: () => api.challenge(address.trim(), csrf), onSuccess: () => { setSent(true); void client.invalidateQueries({ queryKey: key }); } });
  const verify = useMutation({ mutationFn: () => api.verify(code.trim(), csrf), onSuccess: v => { client.setQueryData(key, v); setSent(false); setCode(""); setAddress(""); } });
  const toggle = useMutation({ mutationFn: () => api.saveAccount(q.data!.revision, !q.data!.enabled, csrf), onSuccess: v => client.setQueryData(key, v) });
  const busy = challenge.isPending || verify.isPending || toggle.isPending;
  return <Card><CardContent className="reminder-config"><div className="reminder-title"><h2>{msg("邮箱提醒")}</h2><span className="muted">{q.data?.enabled ? msg("已启用") : msg("已停用")}</span></div>
    {q.data?.verified && <p>{q.data.email} · {msg("已验证")}</p>}
    <div className="reminder-controls"><div className="reminder-sender"><label htmlFor="email-address">{msg("收件邮箱")}</label><input id="email-address" type="email" value={address} disabled={busy} onChange={e => setAddress(e.target.value)} /></div><Button variant="outline" disabled={busy || !address.trim()} onClick={() => challenge.mutate()}>{msg("发送验证码")}</Button></div>
    {(sent || q.data?.pending_email) && <div className="reminder-controls"><div className="reminder-sender"><label htmlFor="email-code">{msg("邮箱验证码")}</label><input id="email-code" inputMode="numeric" value={code} maxLength={8} disabled={busy} onChange={e => setCode(e.target.value)} /></div><Button disabled={busy || code.length !== 8} onClick={() => verify.mutate()}>{msg("验证邮箱")}</Button></div>}
    {q.data?.verified && <Button variant="outline" disabled={busy} onClick={() => toggle.mutate()}>{q.data.enabled ? msg("停用邮箱提醒") : msg("启用邮箱提醒")}</Button>}
    {(q.error || challenge.error || verify.error || toggle.error) && <p role="alert">{(q.error || challenge.error || verify.error || toggle.error)?.message}</p>}
  </CardContent></Card>;
}

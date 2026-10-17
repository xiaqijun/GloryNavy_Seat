import { useEffect, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Copy, LoaderCircle, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { IconAction } from "@/components/ui/icon-action";
import { useToast } from "@/components/ui/toast-context";
import { getLocale, msg } from "@/lib/i18n";
import { createQQChallenge, getProfile, type QQChallenge } from "./api";

export function QQBinding({ userID, csrf }: { userID: string; csrf: string }) {
  const toast = useToast();
  const [challenge, setChallenge] = useState<QQChallenge>();
  const [expiredAt, setExpiredAt] = useState("");
  const generate = useMutation({
    mutationFn: () => createQQChallenge(csrf),
    onMutate: () => setChallenge(undefined),
    onSuccess: setChallenge,
  });
  const expires = Date.parse(challenge?.expires_at ?? "");
  const expired = !!challenge && (!Number.isFinite(expires) || expires <= Date.now() || expiredAt === challenge.expires_at);
  const active = !!challenge && !expired;
  useEffect(() => {
    if (!challenge || !Number.isFinite(expires)) return;
    const timeout = window.setTimeout(() => setExpiredAt(challenge.expires_at), Math.max(0, expires - Date.now()));
    return () => window.clearTimeout(timeout);
  }, [challenge, expires]);
  const confirmation = useQuery({
    queryKey: ["community", "profile", userID],
    queryFn: ({ signal }) => getProfile(signal),
    enabled: active,
    refetchInterval: active ? 3000 : false,
  });
  // This is the bot's command syntax, independent of the interface language.
  const command = challenge ? `绑定 ${challenge.code}` : "";
  return (
    <div className="community-qq-binding">
      <Button type="button" variant="outline" disabled={generate.isPending} aria-busy={generate.isPending}
        onClick={() => generate.mutate()}>
        {generate.isPending && <LoaderCircle className="animate-spin" aria-hidden="true" />}
        {challenge ? msg("重新生成") : msg("生成 QQ 绑定码")}
      </Button>
      {active && (
        <>
          <p>{msg("私聊机器人或在群内 @机器人发送：")}</p>
          <div className="community-qq-command">
            <code>{command}</code>
            <IconAction label={msg("复制绑定指令")} onClick={async () => {
              try {
                await navigator.clipboard.writeText(command);
                toast.success(msg("已复制"));
              } catch {
                toast.error(msg("复制失败，请手动复制"));
              }
            }}><Copy aria-hidden="true" /></IconAction>
          </div>
          <p className="muted">{msg("有效至 {0}", new Date(expires).toLocaleTimeString(getLocale(), { hour: "2-digit", minute: "2-digit" }))}</p>
        </>
      )}
      {expired && <p role="status">{msg("绑定码已过期，请重新生成")}</p>}
      {generate.error && <p role="alert" className="login-error">{generate.error.message}</p>}
      {active && confirmation.isError && (
        <div className="community-qq-command" role="alert">
          <p>{msg("确认状态读取失败，请重试")}</p>
          <IconAction label={msg("刷新确认状态")} onClick={() => void confirmation.refetch()}>
            <RefreshCw aria-hidden="true" />
          </IconAction>
        </div>
      )}
    </div>
  );
}

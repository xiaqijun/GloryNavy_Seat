import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { FormDialog } from "@/components/ui/form-dialog";
import { useToast } from "@/components/ui/toast-context";
import { msg } from "@/lib/i18n";
import { useManagementAccess } from "@/modules/access/use-management-access";
import { getConnection, updateConnection, type ConnectionSettings } from "./api";

const connectionQueryKey = ["sentry", "connection"] as const;

export function ConnectionSettingsButton({ csrf }: { csrf: string }) {
  const { access } = useManagementAccess();
  const [open, setOpen] = useState(false);
  const administrator = access.data?.administrator === true;
  const connection = useQuery({
    queryKey: connectionQueryKey,
    queryFn: ({ signal }) => getConnection(signal),
    enabled: open && administrator,
    refetchOnWindowFocus: false,
  });
  const title = msg("Sentry 接入设置");
  if (!administrator) return null;
  return (
    <>
      <Button type="button" variant="outline" onClick={() => setOpen(true)}>
        <Link2 aria-hidden="true" />{title}
      </Button>
      {open && (connection.data ? (
        <ConnectionForm csrf={csrf} settings={connection.data} close={() => setOpen(false)} />
      ) : (
        <FormDialog title={title} close={() => setOpen(false)} showSubmit={false} onSubmit={() => undefined}>
          {connection.isError
            ? <p role="alert">{connection.error.message} <Button type="button" variant="outline" onClick={() => void connection.refetch()}>{msg("重试")}</Button></p>
            : <p role="status">{msg("正在读取")}</p>}
        </FormDialog>
      ))}
    </>
  );
}

function ConnectionForm({ csrf, settings, close }: { csrf: string; settings: ConnectionSettings; close: () => void }) {
  const client = useQueryClient();
  const toast = useToast();
  const [origin, setOrigin] = useState(settings.origin);
  const [revision] = useState(settings.revision);
  const [token, setToken] = useState("");
  const save = useMutation({
    mutationFn: () => updateConnection(csrf, {
      origin: origin.trim(),
      revision,
      ...(token.trim() ? { service_token: token.trim() } : {}),
    }),
    onSuccess: (next) => {
      setToken("");
      client.setQueryData(connectionQueryKey, next);
      void client.invalidateQueries({ queryKey: ["sentry", "keys"] });
      void client.invalidateQueries({ queryKey: ["sentry", "time-pricing"] });
      toast.info(msg("Sentry 鉴权通过，配置已保存"));
      close();
    },
  });
  const retainToken = settings.token_configured && origin.trim().replace(/\/+$/, "") === settings.origin;
  return (
    <FormDialog
      title={msg("Sentry 接入设置")}
      close={close}
      className="sentry-pricing-form"
      busy={save.isPending}
      disabled={!origin.trim() || (!retainToken && !token.trim()) || (!!token.trim() && token.trim().length < 32)}
      submitLabel={msg("测试并保存")}
      onSubmit={() => save.mutate()}
    >
      <label>
        <span>{msg("Sentry 服务地址")}</span>
        <input autoFocus type="url" maxLength={2048} required value={origin} onChange={(event) => setOrigin(event.target.value)} />
      </label>
      <label>
        <span>{msg("服务令牌")}</span>
        <input type="password" autoComplete="new-password" maxLength={512} required={!retainToken} value={token}
          onChange={(event) => setToken(event.target.value)} />
      </label>
      {retainToken && <p>{msg("留空可继续使用已保存的令牌。")}</p>}
      {save.isError && <p role="alert">{save.error.message}</p>}
    </FormDialog>
  );
}

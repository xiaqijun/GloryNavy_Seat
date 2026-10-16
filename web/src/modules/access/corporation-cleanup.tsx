import { useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Settings2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Modal } from "@/components/ui/dialog";
import { useSession } from "@/modules/identity";
import { getLocale, msg } from "@/lib/i18n";
import {
  getCleanup,
  previewCleanup,
  saveCleanup,
  type CleanupSettings,
} from "./corporation-cleanup-api";
import "./corporation-cleanup.css";

export function CorporationCleanupButton({ actor }: { actor: string }) {
  const [open, setOpen] = useState(false);
  const trigger = useRef<HTMLButtonElement>(null);
  const close = () => {
    setOpen(false);
    requestAnimationFrame(() => trigger.current?.focus());
  };
  return (
    <>
      <Button ref={trigger} variant="outline" onClick={() => setOpen(true)}>
        <Settings2 size={16} aria-hidden="true" />
        {msg("清理规则")}
      </Button>
      {open && <CleanupModal actor={actor} close={close} />}
    </>
  );
}
function CleanupModal({ actor, close }: { actor: string; close: () => void }) {
  const query = useQuery({
    queryKey: ["identity", "cleanup", actor],
    queryFn: ({ signal }) => getCleanup(signal),
    staleTime: 0,
  });
  if (!query.data || query.isError)
    return (
      <Modal title={msg("清理规则")} close={close}>
        <p role={query.isError ? "alert" : "status"}>
          {query.error?.message ?? msg("正在读取清理规则")}
        </p>
        {query.isError && (
          <Button variant="outline" onClick={() => void query.refetch()}>
            {msg("重试")}
          </Button>
        )}
      </Modal>
    );
  return (
    <CleanupForm
      key={query.data.version}
      actor={actor}
      initial={query.data}
      close={close}
      reload={() => void query.refetch()}
    />
  );
}
function CleanupForm({
  actor,
  initial,
  close,
  reload,
}: {
  actor: string;
  initial: CleanupSettings;
  close: () => void;
  reload: () => void;
}) {
  const [enabled, setEnabled] = useState(initial.enabled);
  const [raw, setRaw] = useState(initial.corporation_ids.join("\n"));
  const session = useSession();
  const client = useQueryClient();
  const ids = [...new Set(raw.split(/[\s,，]+/).filter(Boolean))];
  const valid =
    ids.length <= 50 &&
    ids.every(
      (id) => /^[1-9]\d{0,18}$/.test(id) && BigInt(id) <= 9223372036854775807n,
    );
  const csrf = session.data?.session?.csrf_token ?? "";
  const preview = useMutation({ mutationFn: () => previewCleanup(ids, csrf) });
  const save = useMutation({
    mutationFn: () =>
      saveCleanup(
        { enabled, corporation_ids: ids, version: initial.version },
        csrf,
      ),
    onSuccess: (data) => {
      client.setQueryData(["identity", "cleanup", actor], data);
      close();
    },
  });
  const busy = save.isPending || preview.isPending;
  const format = (v: string | null) =>
    v ? new Date(v).toLocaleString(getLocale()) : msg("未安排");
  return (
    <Modal
      title={msg("清理规则")}
      close={close}
      busy={busy}
      className="corp-cleanup"
      footer={
        <>
          <Button
            variant="outline"
            disabled={busy || !csrf || !valid || !ids.length}
            onClick={() => preview.mutate()}
          >
            {preview.isPending ? msg("正在预览") : msg("预览清理")}
          </Button>
          <Button
            disabled={busy || !csrf || !valid || (enabled && !ids.length)}
            onClick={() => save.mutate()}
          >
            {save.isPending ? msg("正在保存") : msg("保存")}
          </Button>
        </>
      }
    >
      <label className="corp-cleanup-toggle">
        <input
          type="checkbox"
          checked={enabled}
          disabled={busy}
          onChange={(e) => setEnabled(e.target.checked)}
        />
        <span>{msg("每周自动清理")}</span>
      </label>
      <label className="corp-cleanup-field">
        {msg("保留军团 ID")}
        <textarea
          value={raw}
          rows={3}
          disabled={busy}
          onChange={(e) => {
            setRaw(e.target.value);
            preview.reset();
            save.reset();
          }}
          aria-describedby="cleanup-ids-help"
          aria-invalid={!valid || (enabled && !ids.length)}
        />
      </label>
      <p id="cleanup-ids-help" className="corp-cleanup-note">
        {msg(
          "用逗号或换行分隔，最多 50 个。启用并保存后 7 天执行，之后每周一次。",
        )}
      </p>
      <p className="corp-cleanup-note">
        {msg(
          "名单外角色均解除绑定，包括小号、主角色和最后一个角色。相关授权与登录失效，账号和业务记录保留；军团信息不确定时跳过。",
        )}
      </p>
      {!valid && (
        <p role="alert">{msg("请填写有效的保留军团 ID，启用时名单不能为空")}</p>
      )}
      <dl className="corp-cleanup-stats">
        <div>
          <dt>{msg("下次清理")}</dt>
          <dd>
            {initial.run_started_at
              ? msg("正在清理")
              : format(initial.next_run_at)}
          </dd>
        </div>
        <div>
          <dt>{msg("最近清理")}</dt>
          <dd>
            {initial.last_run_at ? (
              <>
                {format(initial.last_run_at)}
                <br />
                {msg(
                  "已清理 {0} · 跳过 {1}",
                  initial.last_removed_count,
                  initial.last_skipped_count,
                )}
              </>
            ) : (
              msg("尚未执行")
            )}
          </dd>
        </div>
      </dl>
      {(save.error || preview.error) && (
        <div role="alert">
          <p>{save.error?.message ?? preview.error?.message}</p>
          <Button variant="outline" disabled={busy} onClick={reload}>
            {msg("重新读取规则")}
          </Button>
        </div>
      )}
      {preview.data && (
        <section className="corp-cleanup-preview" aria-live="polite">
          <h3>
            {msg(
              "预计清理 {0} · 保留 {1} · 待核实 {2}",
              preview.data.remove_count,
              preview.data.keep_count,
              preview.data.unknown_count,
            )}
          </h3>
          {preview.data.items.length > 0 && (
            <ul>
              {preview.data.items.map((item) => (
                <li key={item.character_id}>
                  <strong>{item.name}</strong>
                  <span>{item.corporation_name || item.corporation_id}</span>
                </li>
              ))}
            </ul>
          )}
          {preview.data.remove_count > 100 && (
            <p className="corp-cleanup-note">{msg("仅展示前 100 个角色")}</p>
          )}
          <p className="corp-cleanup-note">
            {msg("预览不执行清理，运行时重新核对军团信息。")}
          </p>
        </section>
      )}
    </Modal>
  );
}

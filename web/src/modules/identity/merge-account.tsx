import { apiFetch } from "@/lib/http";
import { msg, getLocale } from "@/lib/i18n";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useSearchParams } from "react-router-dom";
import { Dialog } from "radix-ui";
import { ArrowRight, Combine, LoaderCircle } from "lucide-react";
import { Button } from "@/components/ui/button";
import { EveImage } from "@/components/eve-image";
import { APIError, getData } from "@/lib/http";
import "./merge-account.css";

type Character = { id: string; name: string };
type Preview = {
  id: string;
  source_main: Character;
  target_main: Character;
  characters: Character[];
  token: string;
  completed: boolean;
  data: {
    attendance: { points: number; entries: number };
    exchange: {
      coins_minor: number;
      target_coins_minor: number;
      orders: number;
    };
    fittings: { drafts: number };
    welfare?: { cases: number };
  };
};
function isMergePreview(value: unknown): value is Preview {
  if (!value || typeof value !== "object") return false;
  const p = value as Preview;
  const character = (c: Character) =>
    !!c && typeof c.id === "string" && typeof c.name === "string";
  return (
    typeof p.id === "string" &&
    typeof p.token === "string" &&
    typeof p.completed === "boolean" &&
    character(p.source_main) &&
    character(p.target_main) &&
    Array.isArray(p.characters) &&
    p.characters.every(character) &&
    [
      p.data?.attendance?.points,
      p.data?.attendance?.entries,
      p.data?.exchange?.coins_minor,
      p.data?.exchange?.target_coins_minor,
      p.data?.exchange?.orders,
      p.data?.fittings?.drafts,
    ].every(Number.isSafeInteger)
  );
}
async function mutate(
  path: string,
  csrf: string,
  method = "POST",
  body?: unknown,
) {
  const response = await apiFetch(path, {
    method,
    credentials: "same-origin",
    redirect: "error",
    headers: { "X-CSRF-Token": csrf, "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const payload = await response.json().catch(() => null);
  if (!response.ok)
    throw new APIError(
      payload?.error?.message ?? msg("操作失败，请重试"),
      response.status,
    );
  return payload?.data;
}
const coins = (value: number) =>
  (value / 100).toLocaleString(getLocale(), { maximumFractionDigits: 2 });
export function MergeAccount({
  csrf,
  disabled = false,
}: {
  csrf: string;
  disabled?: boolean;
}) {
  const [params, setParams] = useSearchParams();
  const id = params.get("merge") ?? "";
  const [opened, setOpened] = useState(false);
  const cache = useQueryClient();
  const preview = useQuery({
    queryKey: ["identity", "merge", id],
    queryFn: ({ signal }) =>
      getData(
        `/api/v1/identity/merges/${encodeURIComponent(id)}`,
        isMergePreview,
        signal,
      ),
    enabled: !!id,
    retry: false,
    refetchOnWindowFocus: false,
  });
  const start = useMutation({
    mutationFn: async () => {
      const result = await mutate("/api/v1/eve/accounts/merge", csrf);
      if (typeof result?.url !== "string") throw Error(msg("授权地址无效"));
      const url = new URL(result.url);
      if (url.origin !== "https://login.eveonline.com")
        throw Error(msg("授权地址无效"));
      window.location.assign(url.href);
    },
  });
  const clear = () => {
    const next = new URLSearchParams(params);
    next.delete("merge");
    setParams(next, { replace: true });
    setOpened(false);
  };
  const confirm = useMutation({
    mutationFn: async () => {
      const result = await mutate(
        `/api/v1/identity/merges/${encodeURIComponent(id)}`,
        csrf,
        "POST",
        { token: preview.data?.token },
      );
      if (!isMergePreview(result) || !result.completed)
        throw Error(msg("合并结果未确认，请重试"));
      return result;
    },
    onSuccess: async () => {
      await cache.cancelQueries();
      cache.clear();
      window.location.replace("/account?merged=1");
    },
  });
  const cancel = useMutation({
    mutationFn: () =>
      id
        ? mutate(
            `/api/v1/identity/merges/${encodeURIComponent(id)}`,
            csrf,
            "DELETE",
          )
        : Promise.resolve(),
    onSuccess: clear,
  });
  const busy = start.isPending || confirm.isPending || cancel.isPending;
  const p = preview.data;
  return (
    <>
      <Button
        variant="outline"
        disabled={disabled || busy}
        onClick={() => {
          start.reset();
          confirm.reset();
          cancel.reset();
          setOpened(true);
        }}
      >
        <Combine aria-hidden="true" />
        {msg("合并账号")}{" "}
      </Button>
      <Dialog.Root
        open={opened || !!id}
        onOpenChange={(open) => {
          if (!open && !busy) cancel.mutate();
        }}
      >
        <Dialog.Portal>
          <Dialog.Overlay className="ui-dialog-overlay" />
          <Dialog.Content
            className="ui-dialog-surface account-dialog merge-dialog"
            onEscapeKeyDown={(event) => {
              if (busy) event.preventDefault();
            }}
            onPointerDownOutside={(event) => event.preventDefault()}
          >
            <Dialog.Title>{msg("合并账号")}</Dialog.Title>
            <Dialog.Description className="ui-dialog-description">
              {id
                ? msg("将已验证账号合并到当前账号，确认后生效。")
                : msg("通过 EVE 登录验证另一个本站账号。")}
            </Dialog.Description>
            {!id && (
              <p className="merge-policy">
                {msg(
                  "迁入全部角色、PAP、果壳币和订单。验证后可预览，当前账号的主角色与社区资料保留。",
                )}{" "}
              </p>
            )}
            {id && preview.isPending && (
              <p role="status">
                <LoaderCircle className="animate-spin" aria-hidden="true" />
                {msg("正在读取合并预览")}{" "}
              </p>
            )}
            {p && (
              <>
                <div className="merge-direction">
                  {[p.source_main, p.target_main].map((c, i) => (
                    <div key={i} className="merge-account-person">
                      <EveImage
                        id={c.id}
                        kind="character"
                        className="merge-portrait"
                      />
                      <span>
                        <small>{i ? msg("保留账号") : msg("迁入账号")}</small>
                        <strong>{c.name}</strong>
                      </span>
                      {!i && <ArrowRight aria-hidden="true" />}
                    </div>
                  ))}
                </div>
                <div className="merge-characters" aria-label={msg("迁入角色")}>
                  {p.characters.map((c) => (
                    <span key={c.id}>
                      <EveImage
                        id={c.id}
                        kind="character"
                        className="merge-mini-portrait"
                      />
                      {c.name}
                    </span>
                  ))}
                </div>
                <dl className="merge-totals">
                  <div>
                    <dt>{msg("迁入 PAP")}</dt>
                    <dd>{p.data.attendance.points.toLocaleString()}</dd>
                  </div>
                  <div>
                    <dt>{msg("迁入果壳币")}</dt>
                    <dd>{coins(p.data.exchange.coins_minor)}</dd>
                  </div>
                  <div>
                    <dt>{msg("合并后果壳币")}</dt>
                    <dd>
                      {coins(
                        p.data.exchange.coins_minor +
                          p.data.exchange.target_coins_minor,
                      )}
                    </dd>
                  </div>
                  <div>
                    <dt>{msg("兑换订单")}</dt>
                    <dd>{p.data.exchange.orders}</dd>
                  </div>
                  {p.data.welfare && (
                    <div>
                      <dt>{msg("福利记录")}</dt>
                      <dd>{p.data.welfare.cases}</dd>
                    </div>
                  )}
                </dl>
                <p className="merge-policy">
                  {msg(
                    "保留流水与审计。原账号的 QQ/KOOK 和本站权限不迁入，双方其他登录会话将退出。此操作不支持自助撤销。",
                  )}{" "}
                </p>
                {p.completed && <p role="status">{msg("此账号已完成合并")}</p>}
              </>
            )}
            {(preview.error ||
              start.error ||
              confirm.error ||
              cancel.error) && (
              <p role="alert" className="login-error">
                {
                  (
                    cancel.error ??
                    confirm.error ??
                    start.error ??
                    preview.error
                  )?.message
                }
              </p>
            )}
            <div className="account-dialog-actions">
              <Button
                variant="outline"
                disabled={busy}
                onClick={() => cancel.mutate()}
              >
                {p?.completed ? msg("关闭") : msg("取消")}
              </Button>
              {!id ? (
                <Button disabled={busy} onClick={() => start.mutate()}>
                  {start.isPending
                    ? msg("正在前往 EVE")
                    : msg("验证另一个账号")}
                </Button>
              ) : p?.completed ? null : (
                <>
                  <Button
                    variant="outline"
                    disabled={busy || preview.isFetching}
                    onClick={() => {
                      confirm.reset();
                      void preview.refetch();
                    }}
                  >
                    {msg("刷新预览")}{" "}
                  </Button>
                  <Button
                    disabled={
                      busy || preview.isFetching || !p || preview.isError
                    }
                    onClick={() => confirm.mutate()}
                  >
                    {confirm.isPending ? msg("正在合并") : msg("确认合并")}
                  </Button>
                </>
              )}
            </div>
          </Dialog.Content>
        </Dialog.Portal>
      </Dialog.Root>
    </>
  );
}

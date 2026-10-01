import { msg, getLocale } from "@/lib/i18n";
import { useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Coins } from "lucide-react";
import { FormDialog } from "@/components/ui/form-dialog";
import { Button } from "@/components/ui/button";
import { getData } from "@/lib/http";
import { write, type Event } from "./api";
import { isSaved } from "./pap-api";

import { formatPAPUnits, isCoinQuote } from "./coin-api";

export function CoinConversion({
  user,
  csrf,
  event,
}: {
  user: string;
  csrf: string;
  event: Event;
}) {
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState("");
  const [saved, setSaved] = useState(false);
  const trigger = useRef<HTMLButtonElement>(null);
  const attempt = useRef<{
    version: string;
    request_key: string;
    token: string;
    reason: string;
  } | null>(null);
  const client = useQueryClient();
  const q = useQuery({
    queryKey: ["attendance", "conversion", user, event.id, event.version],
    queryFn: ({ signal }) =>
      getData(
        `/api/v1/attendance/events/${event.id}/conversion`,
        isCoinQuote,
        signal,
      ),
    enabled: open,
    refetchOnWindowFocus: false,
    retry: false,
  });
  const m = useMutation({
    mutationFn: () => {
      attempt.current ??= {
        version: event.version,
        request_key: crypto.randomUUID(),
        token: q.data!.token,
        reason: reason.trim(),
      };
      return write(
        `/events/${event.id}/conversion`,
        csrf,
        attempt.current,
        isSaved,
      );
    },
    onSuccess: () => {
      setOpen(false);
      setSaved(true);
      attempt.current = null;
      void client.invalidateQueries({ queryKey: ["attendance"] });
      void client.invalidateQueries({ queryKey: ["exchange"] });
      trigger.current?.focus();
    },
  });
  return (
    <div className="attendance-pap-history">
      <div className="attendance-toolbar">
        <Button
          ref={trigger}
          variant="outline"
          aria-expanded={open}
          disabled={m.isPending}
          onClick={() => {
            setOpen((v) => !v);
            setReason("");
            attempt.current = null;
            setSaved(false);
            m.reset();
          }}
        >
          <Coins aria-hidden="true" />
          {msg("兑换果壳币")}{" "}
        </Button>
        {saved && <span role="status">{msg("已兑换")}</span>}
      </div>
      {open && (
        <FormDialog
          title={msg("兑换果壳币")}
          close={() => setOpen(false)}
          busy={m.isPending}
          showSubmit={
            !q.data || q.isError || q.isFetching || q.data.pending > 0
          }
          disabled={
            q.isError ||
            q.isFetching ||
            !q.data ||
            q.data.pending <= 0 ||
            !reason.trim()
          }
          submitLabel={msg("确认兑换")}
          className="attendance-form attendance-pap-form"
          onSubmit={() => m.mutate()}
          extraActions={
            <Button
              type="button"
              variant="outline"
              disabled={m.isPending || q.isFetching}
              onClick={() => {
                attempt.current = null;
                m.reset();
                void q.refetch();
              }}
            >
              {msg("重新核对")}
            </Button>
          }
        >
          <strong>{event.title}</strong>
          {q.isError ? (
            <div role="alert">
              {q.error.message}
              <Button
                type="button"
                variant="outline"
                onClick={() => void q.refetch()}
              >
                {msg("重试")}{" "}
              </Button>
            </div>
          ) : !q.data || q.isFetching ? (
            <p role="status">{msg("正在核对兑换记录")}</p>
          ) : (
            <>
              <p className="attendance-note">
                {msg("已兑换")} {formatPAPUnits(q.data.converted, q.data.unit_scale, getLocale())} {msg("分 · 待兑换")} {formatPAPUnits(q.data.pending, q.data.unit_scale, getLocale())} {msg("分")}{" "}
              </p>
              {q.data.pending === 0 ? (
                <p role="status">{msg("本次积分已全部兑换")}</p>
              ) : (
                <>
                  <strong>
                    {q.data.characters} {msg("个角色 · 本次发放")}{" "}
                    {(q.data.coins_minor / 100).toLocaleString(getLocale())}{" "}
                    {msg("果壳币")}{" "}
                  </strong>
                  <label>
                    {msg("兑换说明")}{" "}
                    <textarea
                      rows={3}
                      autoFocus
                      required
                      maxLength={200}
                      value={reason}
                      disabled={m.isPending}
                      onChange={(e) => {
                        setReason(e.target.value);
                        attempt.current = null;
                        m.reset();
                      }}
                    />
                  </label>
                  <small>{msg("积分统计保留，已兑换部分不重复发币。")}</small>
                  {m.isError && <p role="alert">{m.error.message}</p>}
                </>
              )}
            </>
          )}
        </FormDialog>
      )}
    </div>
  );
}

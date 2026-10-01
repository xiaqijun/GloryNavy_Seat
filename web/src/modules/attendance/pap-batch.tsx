import { useMemo, useState } from "react";
import { CalendarCheck2, Check, CircleAlert } from "lucide-react";
import { msg, getLocale } from "@/lib/i18n";
import { FormDialog } from "@/components/ui/form-dialog";
import { formatDate, type Event, write } from "./api";
import { isSaved } from "./pap-api";

type Result = { event: Event; error?: string };

export function PAPBatchIssue({
  csrf,
  events,
  close,
  onComplete,
}: {
  csrf: string;
  events: Event[];
  close: () => void;
  onComplete: () => void;
}) {
  const [reason, setReason] = useState("");
  const [points, setPoints] = useState<Record<string, number>>(() =>
    Object.fromEntries(
      events.map((event) => [event.id, Math.max(1, event.pap_points ?? 1)]),
    ),
  );
  const [result, setResult] = useState<Result[] | null>(null);
  const total = useMemo(
    () =>
      events.reduce(
        (sum, event) =>
          sum + event.participants * (points[event.id] ?? 1),
        0,
      ),
    [events, points],
  );
  const validPoints = events.every((event) => {
    const value = points[event.id] ?? 1;
    return Number.isInteger(value) && value >= 1 && value <= 10000;
  });
  // Keep the sequential writes in the dialog so each activity retains its
  // own version, audit row and idempotency key. A failure does not hide which
  // activities were already issued.
  const [busy, setBusy] = useState(false);
  const submit = async () => {
    if (busy || !reason.trim() || events.length === 0) return;
    setBusy(true);
    const results: Result[] = [];
    for (const event of events) {
      try {
        await write(
          `/events/${event.id}/pap`,
          csrf,
          {
            version: event.version,
            request_key: crypto.randomUUID(),
            points: points[event.id] ?? 1,
            reason: reason.trim(),
            revoke: false,
          },
          isSaved,
        );
        results.push({ event });
      } catch (error) {
        results.push({
          event,
          error: error instanceof Error ? error.message : msg("发放失败"),
        });
      }
    }
    setResult(results);
    setBusy(false);
    onComplete();
  };
  const successful = result?.filter((item) => !item.error).length ?? 0;
  return (
    <FormDialog
      title={msg("统一发放军团 PAP")}
      formLabel={msg("统一发放军团 PAP")}
      close={close}
      busy={busy}
      disabled={!reason.trim() || events.length === 0 || !validPoints}
      submitLabel={msg("发放全部")}
      size="form"
      className="attendance-form attendance-pap-batch"
      showSubmit={result === null}
      onSubmit={(event) => {
        event.preventDefault();
        void submit();
      }}
    >
      {result ? (
        <div className="attendance-pap-batch-result" role="status">
          <strong>
            {msg("已处理 {0}/{1} 场活动", successful, result.length)}
          </strong>
          {result.map((item) => (
            <div className="attendance-pap-batch-row" key={item.event.id}>
              {item.error ? (
                <CircleAlert aria-hidden="true" className="is-error" />
              ) : (
                <Check aria-hidden="true" className="is-success" />
              )}
              <span>
                <strong>{item.event.title}</strong>
                {item.error && <small role="alert">{item.error}</small>}
              </span>
            </div>
          ))}
        </div>
      ) : (
        <>
          <p className="attendance-note">
            {msg("以下活动将逐场发放，保留各自的出勤名单和审计记录。")}
          </p>
          <div className="attendance-pap-batch-list">
            {events.map((event) => (
              <div className="attendance-pap-batch-row" key={event.id}>
                <CalendarCheck2 aria-hidden="true" />
                <span className="attendance-event-copy">
                  <strong>{event.title}</strong>
                  <small>
                    {formatDate(event.starts_at)} · {event.participants} {msg("名角色")}
                  </small>
                </span>
                <label>
                  {msg("每角色分值")}
                  <input
                    aria-label={msg("{0} 每角色分值", event.title)}
                    type="number"
                    min={1}
                    max={10000}
                    step={1}
                    value={Number.isFinite(points[event.id]) ? points[event.id] : ""}
                    disabled={busy}
                    onChange={(input) =>
                      setPoints((current) => ({
                        ...current,
                        [event.id]: input.target.valueAsNumber,
                      }))
                    }
                  />
                </label>
              </div>
            ))}
          </div>
          <p className="attendance-note">
            {msg("预计发放 {0} 分", total.toLocaleString(getLocale()))}
          </p>
          <label>
            {msg("发分说明")}
            <textarea
              rows={3}
              autoFocus
              maxLength={200}
              required
              value={reason}
              disabled={busy}
              onChange={(event) => setReason(event.target.value)}
            />
          </label>
        </>
      )}
      {!result && events.length === 0 && (
        <p className="attendance-note">{msg("暂无待发放活动")}</p>
      )}
    </FormDialog>
  );
}

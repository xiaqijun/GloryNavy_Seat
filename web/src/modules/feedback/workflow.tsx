import { useState, type ReactNode } from "react";
import { useMutation } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Modal } from "@/components/ui/dialog";
import { msg } from "@/lib/i18n";
import * as api from "./api";

type Action = { target: api.Status; label: string; required?: boolean };
export function FeedbackWorkflow({
  ticket,
  id,
  user,
  administrator,
  csrf,
  updated,
  close,
  overview,
  children,
  supplementAction,
  supplementForm,
  supplementSubmit,
  busy = false,
}: {
  ticket?: api.Ticket;
  id: string;
  user: string;
  administrator: boolean;
  csrf: string;
  updated: () => Promise<unknown>;
  close: () => void;
  overview: ReactNode;
  children: ReactNode;
  supplementAction?: ReactNode;
  supplementForm?: ReactNode;
  supplementSubmit?: ReactNode;
  busy?: boolean;
}) {
  const owner = ticket?.account_id === user;
  const [action, setAction] = useState<Action | null>(null),
    [note, setNote] = useState("");
  const [key, setKey] = useState(() => crypto.randomUUID());
  const mutation = useMutation({
    mutationFn: () => {
      if (!ticket) throw new Error(msg("正在读取"));
      return api.reply(ticket.id, csrf, {
        request_key: key,
        revision: ticket.revision,
        body: note,
        status: action!.target,
      });
    },
    onSuccess: async () => {
      setAction(null);
      setNote("");
      setKey(crypto.randomUUID());
      await updated();
    },
  });
  let actions: Action[] = [];
  if (owner) {
    if (ticket?.status === "awaiting_confirmation")
      actions = [
        { target: "resolved", label: msg("确认完成") },
        { target: "in_progress", label: msg("退回处理"), required: true },
      ];
    if (ticket?.status === "resolved")
      actions = [
        { target: "in_progress", label: msg("重新打开"), required: true },
      ];
    if (ticket?.status === "closed")
      actions = [
        { target: "submitted", label: msg("重新提交"), required: true },
      ];
  } else if (administrator) {
    if (ticket?.status === "submitted")
      actions = [
        { target: "accepted", label: msg("采纳反馈") },
        { target: "closed", label: msg("不采纳"), required: true },
      ];
    if (ticket?.status === "accepted")
      actions = [
        { target: "in_progress", label: msg("开始处理") },
        { target: "closed", label: msg("关闭反馈"), required: true },
      ];
    if (ticket?.status === "in_progress")
      actions = [
        ...(!ticket.accepted_at
          ? [{ target: "accepted" as const, label: msg("采纳反馈") }]
          : []),
        {
          target: "awaiting_confirmation",
          label: msg("提交完成"),
          required: true,
        },
        { target: "closed", label: msg("关闭反馈"), required: true },
      ];
  }
  const stages: [api.Status, string][] = [
    ["submitted", msg("待处理")],
    ["accepted", msg("已采纳")],
    ["in_progress", msg("处理中")],
    ["awaiting_confirmation", msg("待确认")],
    ["resolved", msg("已完成")],
  ];
  const blocked = busy || mutation.isPending;
  const formID = `feedback-action-${id}`;
  return (
    <Modal
      title={`${msg("反馈详情")} #${id}`}
      close={close}
      busy={blocked}
      className="feedback-modal"
      footer={
        ticket ? (
          <div className="feedback-detail-actions">
            {!action && supplementAction}
            <div className="feedback-workflow-actions">
              {action ? (
                <>
                  <Button
                    type="button"
                    variant="outline"
                    disabled={blocked}
                    onClick={() => setAction(null)}
                  >
                    {msg("取消")}
                  </Button>
                  <Button
                    form={formID}
                    type="submit"
                    disabled={blocked || (action.required && !note.trim())}
                  >
                    {mutation.isPending ? msg("正在提交") : msg("确认操作")}
                  </Button>
                </>
              ) : (
                supplementSubmit ||
                actions.map((a, i) => (
                  <Button
                    key={a.target}
                    disabled={blocked}
                    variant={i === 0 ? "default" : "outline"}
                    onClick={() => {
                      setAction(a);
                      setNote("");
                      setKey(crypto.randomUUID());
                      mutation.reset();
                    }}
                  >
                    {a.label}
                  </Button>
                ))
              )}
            </div>
          </div>
        ) : undefined
      }
    >
      <div className="feedback-thread">
        {overview}
        {ticket && ticket.status !== "closed" && (
          <ol className="feedback-progress" aria-label={msg("办理进度")}>
            {stages.map(([value, label], i) => (
              <li
                key={value}
                aria-current={ticket?.status === value ? "step" : undefined}
              >
                <span aria-hidden="true">{i + 1}</span>
                {label}
              </li>
            ))}
          </ol>
        )}
        {ticket?.status === "awaiting_confirmation" && !owner && (
          <p>{msg("等待提交人确认")}</p>
        )}
        {ticket && action && (
          <form
            id={formID}
            className="feedback-form"
            onSubmit={(e) => {
              e.preventDefault();
              if (!blocked && (!action.required || note.trim()))
                mutation.mutate();
            }}
          >
            <strong>{action.label}</strong>
            <label>
              {msg(action.required ? "处理结果或原因" : "补充说明（选填）")}
              <textarea
                autoFocus
                rows={3}
                maxLength={2000}
                value={note}
                required={action.required}
                disabled={blocked}
                onChange={(e) => {
                  setNote(e.target.value);
                  setKey(crypto.randomUUID());
                  mutation.reset();
                }}
              />
            </label>
            {mutation.isError && <p role="alert">{mutation.error.message}</p>}
          </form>
        )}
        {children}
        {!action && supplementForm}
      </div>
    </Modal>
  );
}
